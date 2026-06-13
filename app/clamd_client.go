package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const (
	defaultClamdDialTimeout = 2 * time.Second
	defaultClamdIOTimeout   = 30 * time.Second
	defaultStreamChunkSize  = 32 * 1024
	defaultStreamMaxLength  = 100 * 1024 * 1024
)

var errStreamMaxLength = errors.New("INSTREAM 超過 StreamMaxLength")

type clamdDialer func(ctx context.Context, network string, address string) (net.Conn, error)

type ClamDClient struct {
	SocketPath      string
	DialTimeout     time.Duration
	IOTimeout       time.Duration
	StreamChunkSize int
	StreamMaxLength int64
	dial            clamdDialer
}

type FileReadError struct {
	Path   string
	Reason string
	Err    error
}

func (e FileReadError) Error() string {
	return fmt.Sprintf("%s：%s", e.Reason, e.Path)
}

func (e FileReadError) Unwrap() error {
	return e.Err
}

func (c ClamDClient) Ping(ctx context.Context) error {
	reply, err := c.command(ctx, "PING")
	if err != nil {
		return err
	}
	if reply != "PONG" {
		return fmt.Errorf("clamd PING 回應不符預期: %s", reply)
	}
	return nil
}

func (c ClamDClient) VersionCommands(ctx context.Context) (string, error) {
	return c.command(ctx, "VERSIONCOMMANDS")
}

func (c ClamDClient) Scan(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("SCAN path 不可為空")
	}
	return c.command(ctx, "SCAN "+path)
}

func (c ClamDClient) InstreamFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", classifyFileReadError(path, err)
	}
	defer file.Close()

	return c.Instream(ctx, file)
}

func (c ClamDClient) Instream(ctx context.Context, reader io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if reader == nil {
		return "", errors.New("INSTREAM reader 不可為 nil")
	}

	conn, err := c.connect(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := c.setDeadline(conn); err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return "", fmt.Errorf("INSTREAM 指令傳送失敗: %w", err)
	}

	chunkSize := c.streamChunkSize()
	buf := make([]byte, chunkSize)
	var sent int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			sent += int64(n)
			if maxLength := c.streamMaxLength(); maxLength > 0 && sent > maxLength {
				return "", errStreamMaxLength
			}
			if err := c.writeChunk(conn, buf[:n]); err != nil {
				return "", err
			}
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		return "", fmt.Errorf("讀取 INSTREAM 來源失敗: %w", readErr)
	}

	if err := c.writeChunk(conn, nil); err != nil {
		return "", err
	}
	return readClamdReply(conn)
}

func (c ClamDClient) command(ctx context.Context, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	conn, err := c.connect(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := c.setDeadline(conn); err != nil {
		return "", err
	}
	if _, err := conn.Write([]byte("z" + command + "\x00")); err != nil {
		return "", fmt.Errorf("%s 傳送失敗: %w", command, err)
	}
	return readClamdReply(conn)
}

func (c ClamDClient) connect(ctx context.Context) (net.Conn, error) {
	if strings.TrimSpace(c.SocketPath) == "" {
		return nil, errors.New("clamd socket path 不可為空")
	}

	dial := c.dial
	if dial == nil {
		timeout := c.DialTimeout
		if timeout <= 0 {
			timeout = defaultClamdDialTimeout
		}
		dialer := net.Dialer{Timeout: timeout}
		dial = dialer.DialContext
	}

	conn, err := dial(ctx, "unix", c.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("無法連線到 clamd socket %s: %w", c.SocketPath, err)
	}
	return conn, nil
}

func (c ClamDClient) setDeadline(conn net.Conn) error {
	timeout := c.IOTimeout
	if timeout <= 0 {
		timeout = defaultClamdIOTimeout
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("設定 clamd timeout 失敗: %w", err)
	}
	return nil
}

func (c ClamDClient) writeChunk(conn net.Conn, chunk []byte) error {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(chunk)))
	if _, err := conn.Write(size[:]); err != nil {
		return fmt.Errorf("INSTREAM chunk size 傳送失敗: %w", err)
	}
	if len(chunk) == 0 {
		return nil
	}
	if _, err := conn.Write(chunk); err != nil {
		return fmt.Errorf("INSTREAM chunk 傳送失敗: %w", err)
	}
	return nil
}

func (c ClamDClient) streamChunkSize() int {
	if c.StreamChunkSize > 0 {
		return c.StreamChunkSize
	}
	return defaultStreamChunkSize
}

func (c ClamDClient) streamMaxLength() int64 {
	if c.StreamMaxLength > 0 {
		return c.StreamMaxLength
	}
	return defaultStreamMaxLength
}

func readClamdReply(conn net.Conn) (string, error) {
	reply, err := bufio.NewReader(conn).ReadString('\x00')
	if err != nil {
		return "", fmt.Errorf("clamd 沒有回應: %w", err)
	}
	return strings.TrimRight(reply, "\x00\r\n"), nil
}

func classifyFileReadError(path string, err error) error {
	if errors.Is(err, os.ErrPermission) {
		return FileReadError{Path: path, Reason: "權限不足，無法讀取掃描檔案", Err: err}
	}
	if errors.Is(err, os.ErrNotExist) {
		return FileReadError{Path: path, Reason: "掃描檔案不存在", Err: err}
	}
	return FileReadError{Path: path, Reason: "無法讀取掃描檔案", Err: err}
}
