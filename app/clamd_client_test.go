package main

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClamDClientPing(t *testing.T) {
	client := stubClamDClient(t, func(t *testing.T, conn net.Conn) {
		assertCommand(t, conn, "zPING\x00")
		writeReply(t, conn, "PONG\x00")
	})

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping clamd: %v", err)
	}
}

func TestClamDClientVersionCommands(t *testing.T) {
	client := stubClamDClient(t, func(t *testing.T, conn net.Conn) {
		assertCommand(t, conn, "zVERSIONCOMMANDS\x00")
		writeReply(t, conn, "ClamAV 1.5.2/COMMANDS: SCAN INSTREAM\x00")
	})

	reply, err := client.VersionCommands(context.Background())
	if err != nil {
		t.Fatalf("version commands: %v", err)
	}
	if reply != "ClamAV 1.5.2/COMMANDS: SCAN INSTREAM" {
		t.Fatalf("unexpected reply: %q", reply)
	}
}

func TestClamDClientScan(t *testing.T) {
	client := stubClamDClient(t, func(t *testing.T, conn net.Conn) {
		assertCommand(t, conn, "zSCAN /Users/jerry/Downloads/a.txt\x00")
		writeReply(t, conn, "/Users/jerry/Downloads/a.txt: OK\x00")
	})

	reply, err := client.Scan(context.Background(), "/Users/jerry/Downloads/a.txt")
	if err != nil {
		t.Fatalf("scan path: %v", err)
	}
	if reply != "/Users/jerry/Downloads/a.txt: OK" {
		t.Fatalf("unexpected scan reply: %q", reply)
	}
}

func TestClamDClientInstreamSendsChunks(t *testing.T) {
	client := stubClamDClient(t, func(t *testing.T, conn net.Conn) {
		assertCommand(t, conn, "zINSTREAM\x00")
		assertChunk(t, conn, "abc")
		assertChunk(t, conn, "def")
		assertChunk(t, conn, "")
		writeReply(t, conn, "stream: OK\x00")
	})
	client.StreamChunkSize = 3

	reply, err := client.Instream(context.Background(), strings.NewReader("abcdef"))
	if err != nil {
		t.Fatalf("instream: %v", err)
	}
	if reply != "stream: OK" {
		t.Fatalf("unexpected instream reply: %q", reply)
	}
}

func TestClamDClientInstreamHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := ClamDClient{SocketPath: "/tmp/clamd.sock"}
	_, err := client.Instream(ctx, strings.NewReader("abc"))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestClamDClientInstreamReportsStreamMaxLength(t *testing.T) {
	client := stubClamDClient(t, func(t *testing.T, conn net.Conn) {
		assertCommand(t, conn, "zINSTREAM\x00")
	})
	client.StreamChunkSize = 4
	client.StreamMaxLength = 3

	_, err := client.Instream(context.Background(), strings.NewReader("abcd"))

	if !errors.Is(err, errStreamMaxLength) {
		t.Fatalf("expected stream max length error, got %v", err)
	}
}

func TestClamDClientInstreamFileReadsUserFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "scan-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := file.WriteString("payload"); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	client := stubClamDClient(t, func(t *testing.T, conn net.Conn) {
		assertCommand(t, conn, "zINSTREAM\x00")
		assertChunk(t, conn, "payload")
		assertChunk(t, conn, "")
		writeReply(t, conn, "stream: OK\x00")
	})

	reply, err := client.InstreamFile(context.Background(), file.Name())
	if err != nil {
		t.Fatalf("instream file: %v", err)
	}
	if reply != "stream: OK" {
		t.Fatalf("unexpected file scan reply: %q", reply)
	}
}

func TestClamDClientSetsReadWriteDeadline(t *testing.T) {
	var wrapped *deadlineConn
	client := ClamDClient{
		SocketPath: "/tmp/clamd.sock",
		IOTimeout: 10 * time.Second,
		dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			client, server := net.Pipe()
			wrapped = &deadlineConn{Conn: client}
			go func() {
				defer server.Close()
				assertCommand(t, server, "zPING\x00")
				writeReply(t, server, "PONG\x00")
			}()
			return wrapped, nil
		},
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping clamd: %v", err)
	}
	if wrapped == nil || wrapped.deadline.IsZero() {
		t.Fatal("expected client to set read/write deadline")
	}
}

func TestClassifyFileReadErrorReportsAccessDenied(t *testing.T) {
	err := classifyFileReadError("/private/file.txt", os.ErrPermission)

	var fileErr FileReadError
	if !errors.As(err, &fileErr) {
		t.Fatalf("expected FileReadError, got %T", err)
	}
	if fileErr.Reason != "權限不足，無法讀取掃描檔案" {
		t.Fatalf("unexpected reason: %q", fileErr.Reason)
	}
}

func stubClamDClient(t *testing.T, handle func(t *testing.T, conn net.Conn)) ClamDClient {
	t.Helper()
	return ClamDClient{
		SocketPath: "/tmp/clamd.sock",
		dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				handle(t, server)
			}()
			return client, nil
		},
	}
}

func assertCommand(t *testing.T, conn net.Conn, expected string) {
	t.Helper()
	buf := make([]byte, len(expected))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read command: %v", err)
	}
	if string(buf) != expected {
		t.Fatalf("unexpected command: got %q, expected %q", string(buf), expected)
	}
}

func assertChunk(t *testing.T, conn net.Conn, expected string) {
	t.Helper()
	var size [4]byte
	if _, err := io.ReadFull(conn, size[:]); err != nil {
		t.Fatalf("read chunk size: %v", err)
	}
	chunkSize := binary.BigEndian.Uint32(size[:])
	if chunkSize != uint32(len(expected)) {
		t.Fatalf("unexpected chunk size: got %d, expected %d", chunkSize, len(expected))
	}
	if chunkSize == 0 {
		return
	}
	buf := make([]byte, chunkSize)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if string(buf) != expected {
		t.Fatalf("unexpected chunk: got %q, expected %q", string(buf), expected)
	}
}

func writeReply(t *testing.T, conn net.Conn, reply string) {
	t.Helper()
	if _, err := conn.Write([]byte(reply)); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

type deadlineConn struct {
	net.Conn
	deadline time.Time
}

func (c *deadlineConn) SetDeadline(deadline time.Time) error {
	c.deadline = deadline
	return c.Conn.SetDeadline(deadline)
}
