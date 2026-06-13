package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errFreshclamUpdateInProgress = errors.New("freshclam update already running")

// DatabaseStatus 描述病毒碼資料庫的位置、最後更新時間、版本與簽章數量，序列化給前端顯示。
type DatabaseStatus struct {
	Path        string    `json:"path"`
	LastUpdated time.Time `json:"lastUpdated"`
	Version     string    `json:"version"`
	Signatures  int       `json:"signatures"`
	Error       string    `json:"error"`
}

// FreshclamEvent 為更新過程中的一行輸出事件（stdout/stderr），即時推播給前端顯示進度。
type FreshclamEvent struct {
	Stream  string    `json:"stream"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// FreshclamError 為分類過的更新錯誤（如 config、network、permission、lock），方便前端對應提示與處理。
type FreshclamError struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Err      error  `json:"-"`
}

// Error 回傳錯誤訊息；訊息為空時退回分類字串。
func (e FreshclamError) Error() string {
	if e.Message == "" {
		return e.Category
	}
	return e.Message
}

// Unwrap 回傳底層錯誤，支援 errors.Is / errors.As。
func (e FreshclamError) Unwrap() error {
	return e.Err
}

type freshclamRunner func(ctx context.Context, command string, args []string, stdout io.Writer, stderr io.Writer) error

// FreshclamService 封裝 freshclam 病毒碼更新：載入狀態、執行更新，並以 mutex 防止重複執行。
type FreshclamService struct {
	Profile               RuntimeProfile
	StatusPath            string
	GeneratedConfigPath   string
	GeneratedDatabasePath string
	GeneratedLogPath      string
	run                   freshclamRunner
	now                   func() time.Time

	mu      sync.Mutex
	running bool
}

func newFreshclamService(profile RuntimeProfile) *FreshclamService {
	return &FreshclamService{Profile: profile}
}

// LoadStatus 讀取已保存的病毒碼狀態；狀態檔不存在時回傳僅含路徑的空狀態。
func (s *FreshclamService) LoadStatus() (DatabaseStatus, error) {
	path := s.statusPath()
	status := DatabaseStatus{Path: s.databasePath()}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, fmt.Errorf("讀取 database status 失敗: %w", err)
	}
	if err := json.Unmarshal(content, &status); err != nil {
		return DatabaseStatus{Path: s.databasePath(), Error: err.Error()}, fmt.Errorf("解析 database status 失敗: %w", err)
	}
	if status.Path == "" {
		status.Path = s.databasePath()
	}
	return status, nil
}

// UpdateDatabase 執行 freshclam 更新病毒碼，過程逐行透過 emit 推播事件；同時間僅允許一次更新（否則回傳 lock 錯誤）。
func (s *FreshclamService) UpdateDatabase(ctx context.Context, emit func(FreshclamEvent)) (DatabaseStatus, error) {
	if !s.startUpdate() {
		err := FreshclamError{
			Category: "lock",
			Message:  "已有 freshclam 更新正在執行",
			Err:      errFreshclamUpdateInProgress,
		}
		status := DatabaseStatus{Path: s.databasePath(), Error: err.Message}
		_ = s.saveStatus(status)
		return status, err
	}
	defer s.finishUpdate()

	if strings.TrimSpace(s.Profile.FreshclamPath) == "" {
		err := FreshclamError{Category: "config", Message: "freshclam path 未設定"}
		status := DatabaseStatus{Path: s.databasePath(), Error: err.Message}
		_ = s.saveStatus(status)
		return status, err
	}

	configPath, err := s.ensureFreshclamConfig()
	if err != nil {
		classified := FreshclamError{Category: "config", Message: "病毒碼更新設定錯誤：" + err.Error(), Err: err}
		status := DatabaseStatus{Path: s.databasePath(), Error: classified.Message}
		_ = s.saveStatus(status)
		return status, classified
	}

	args := []string{"--foreground", "--config-file=" + configPath}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	stdoutEvents := newFreshclamEventWriter("stdout", emit, s.timeNow)
	stderrEvents := newFreshclamEventWriter("stderr", emit, s.timeNow)
	err = s.runner()(ctx, s.Profile.FreshclamPath, args, io.MultiWriter(&stdoutBuf, stdoutEvents), io.MultiWriter(&stderrBuf, stderrEvents))
	stdoutEvents.Flush()
	stderrEvents.Flush()

	combinedOutput := strings.TrimSpace(stdoutBuf.String() + "\n" + stderrBuf.String())
	if err != nil {
		classified := classifyFreshclamError(combinedOutput, err)
		status := DatabaseStatus{Path: s.databasePath(), Error: classified.Message}
		_ = s.saveStatus(status)
		return status, classified
	}

	status := parseFreshclamStatus(s.databasePath(), combinedOutput, s.timeNow())
	if err := s.saveStatus(status); err != nil {
		return status, err
	}
	return status, nil
}

func (s *FreshclamService) startUpdate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *FreshclamService) finishUpdate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

func (s *FreshclamService) runner() freshclamRunner {
	if s.run != nil {
		return s.run
	}
	return runFreshclam
}

func (s *FreshclamService) timeNow() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *FreshclamService) databasePath() string {
	if s.shouldUseGeneratedConfig() {
		return s.generatedDatabasePath()
	}
	if strings.TrimSpace(s.Profile.DatabasePath) != "" {
		return s.Profile.DatabasePath
	}
	return filepath.Join(systemRuntimeBase, "Database")
}

func (s *FreshclamService) statusPath() string {
	if strings.TrimSpace(s.StatusPath) != "" {
		return s.StatusPath
	}
	return filepath.Join(s.databasePath(), "status.json")
}

func (s *FreshclamService) saveStatus(status DatabaseStatus) error {
	if status.Path == "" {
		status.Path = s.databasePath()
	}
	content, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 database status 失敗: %w", err)
	}
	content = append(content, '\n')

	path := s.statusPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("建立 database status 目錄失敗: %w", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("寫入 database status 失敗: %w", err)
	}
	return nil
}

func (s *FreshclamService) ensureFreshclamConfig() (string, error) {
	if path := s.configuredFreshclamConfigPath(); fileExists(path) {
		return path, nil
	}

	configPath := s.generatedFreshclamConfigPath()
	databasePath := s.generatedDatabasePath()
	logPath := s.generatedLogPath()

	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return "", fmt.Errorf("建立 freshclam config 目錄失敗: %w", err)
	}
	if err := os.MkdirAll(databasePath, 0700); err != nil {
		return "", fmt.Errorf("建立病毒碼資料庫目錄失敗: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return "", fmt.Errorf("建立 freshclam log 目錄失敗: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(generatedFreshclamConfig(databasePath, logPath)), 0600); err != nil {
		return "", fmt.Errorf("寫入 freshclam config 失敗: %w", err)
	}
	return configPath, nil
}

func (s *FreshclamService) shouldUseGeneratedConfig() bool {
	return !fileExists(s.configuredFreshclamConfigPath())
}

func (s *FreshclamService) configuredFreshclamConfigPath() string {
	if strings.TrimSpace(s.Profile.ConfigPath) == "" {
		return ""
	}
	return filepath.Join(s.Profile.ConfigPath, "freshclam.conf")
}

func (s *FreshclamService) generatedFreshclamConfigPath() string {
	if strings.TrimSpace(s.GeneratedConfigPath) != "" {
		return s.GeneratedConfigPath
	}
	return filepath.Join(defaultUserDataBase(), "Config/freshclam.conf")
}

func (s *FreshclamService) generatedDatabasePath() string {
	if strings.TrimSpace(s.GeneratedDatabasePath) != "" {
		return s.GeneratedDatabasePath
	}
	return filepath.Join(defaultUserDataBase(), "Database")
}

func (s *FreshclamService) generatedLogPath() string {
	if strings.TrimSpace(s.GeneratedLogPath) != "" {
		return s.GeneratedLogPath
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "Library/Logs/ClamAVDesktop/freshclam.log")
}

func generatedFreshclamConfig(databasePath string, logPath string) string {
	return strings.Join([]string{
		"DatabaseDirectory " + databasePath,
		"UpdateLogFile " + logPath,
		"LogTime yes",
		"DatabaseMirror database.clamav.net",
		"",
	}, "\n")
}

func defaultUserDataBase() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "Library/Application Support/ClamAVDesktop")
}

func runFreshclam(ctx context.Context, command string, args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func parseFreshclamStatus(databasePath string, output string, updatedAt time.Time) DatabaseStatus {
	status := DatabaseStatus{Path: databasePath, LastUpdated: updatedAt.UTC()}
	for _, line := range strings.Split(output, "\n") {
		if version := parseFreshclamVersion(line); version != "" {
			status.Version = version
		}
		if signatures := parseFreshclamSignatures(line); signatures > 0 {
			status.Signatures = signatures
		}
	}
	return status
}

func parseFreshclamVersion(line string) string {
	const marker = "version:"
	index := strings.Index(line, marker)
	if index == -1 {
		return ""
	}
	value := strings.TrimSpace(line[index+len(marker):])
	value = strings.TrimRight(value, ")")
	if comma := strings.Index(value, ","); comma >= 0 {
		value = strings.TrimSpace(value[:comma])
	}
	return value
}

func parseFreshclamSignatures(line string) int {
	const marker = "sigs:"
	index := strings.Index(line, marker)
	if index == -1 {
		return 0
	}
	value := strings.TrimSpace(line[index+len(marker):])
	value = strings.TrimRight(value, ")")
	if comma := strings.Index(value, ","); comma >= 0 {
		value = strings.TrimSpace(value[:comma])
	}
	var signatures int
	_, _ = fmt.Sscanf(value, "%d", &signatures)
	return signatures
}

func classifyFreshclamError(output string, err error) FreshclamError {
	lower := strings.ToLower(output + "\n" + err.Error())
	category := "unknown"
	switch {
	case strings.Contains(lower, "lock") || strings.Contains(lower, "already running"):
		category = "lock"
	case strings.Contains(lower, "permission denied") || strings.Contains(lower, "operation not permitted"):
		category = "permission"
	case strings.Contains(lower, "config") || strings.Contains(lower, "parse"):
		category = "config"
	case strings.Contains(lower, "download") || strings.Contains(lower, "network") || strings.Contains(lower, "connection") || strings.Contains(lower, "resolve") || strings.Contains(lower, "dns") || strings.Contains(lower, "http get failed") || strings.Contains(lower, "couldn't resolve host") || strings.Contains(lower, "failed to get"):
		category = "network"
	}
	message := freshclamErrorMessage(category)
	if output = strings.TrimSpace(output); output != "" {
		message += "：" + firstOutputLine(output)
	}
	return FreshclamError{Category: category, Message: message, Err: err}
}

func freshclamErrorMessage(category string) string {
	switch category {
	case "lock":
		return "病毒碼更新已在執行"
	case "permission":
		return "病毒碼更新權限不足"
	case "config":
		return "病毒碼更新設定錯誤"
	case "network":
		return "病毒碼更新網路錯誤"
	default:
		return "病毒碼更新失敗"
	}
}

func firstOutputLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

type freshclamEventWriter struct {
	stream string
	emit   func(FreshclamEvent)
	now    func() time.Time
	buf    []byte
}

func newFreshclamEventWriter(stream string, emit func(FreshclamEvent), now func() time.Time) *freshclamEventWriter {
	return &freshclamEventWriter{stream: stream, emit: emit, now: now}
}

// Write 實作 io.Writer，依換行切分緩衝內容並逐行發出 FreshclamEvent。
func (w *freshclamEventWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			w.emitLine()
			continue
		}
		w.buf = append(w.buf, b)
	}
	return len(p), nil
}

// Flush 將緩衝中尚未換行的殘餘內容作為最後一行事件發出。
func (w *freshclamEventWriter) Flush() {
	w.emitLine()
}

func (w *freshclamEventWriter) emitLine() {
	if w.emit == nil || len(w.buf) == 0 {
		w.buf = w.buf[:0]
		return
	}
	message := strings.TrimRight(string(w.buf), "\r")
	w.buf = w.buf[:0]
	if strings.TrimSpace(message) == "" {
		return
	}
	w.emit(FreshclamEvent{Stream: w.stream, Message: message, At: w.now()})
}
