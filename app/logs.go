package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const sharedLogPath = "/Library/Logs/ClamAVDesktop"

// LogEntry is one line of the per-user app log.
type LogEntry struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// LogService reads and writes the per-user app log and exposes the shared
// freshclam/clamd logs written by the LaunchDaemon-managed services.
type LogService struct {
	LogPath       string
	SharedLogPath string
	openLocation  openLocationRunner
	now           func() time.Time
}

func newLogService(homeDir string) *LogService {
	return &LogService{
		LogPath:       filepath.Join(homeDir, "Library/Logs/ClamAVDesktop"),
		SharedLogPath: sharedLogPath,
	}
}

func (s *LogService) appLogPath() string {
	return filepath.Join(s.LogPath, "app.log")
}

// WriteAppLog appends one entry to the per-user app log.
func (s *LogService) WriteAppLog(level, message string) error {
	entry := LogEntry{At: s.timeNow(), Level: level, Message: message}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化 app log 失敗: %w", err)
	}
	line = append(line, '\n')

	if err := os.MkdirAll(s.LogPath, 0700); err != nil {
		return fmt.Errorf("建立 log 目錄失敗: %w", err)
	}
	file, err := os.OpenFile(s.appLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("開啟 app log 失敗: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("寫入 app log 失敗: %w", err)
	}
	return nil
}

// ListAppLogEntries returns the most recent app log entries, newest first.
// limit <= 0 returns all entries.
func (s *LogService) ListAppLogEntries(limit int) ([]LogEntry, error) {
	content, err := os.ReadFile(s.appLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, fmt.Errorf("讀取 app log 失敗: %w", err)
	}

	entries := make([]LogEntry, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].At.After(entries[j].At)
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// ReadSharedLog returns the most recent lines of a shared system log written
// by the LaunchDaemon-managed freshclam/clamd services. Returns an empty
// slice if the shared runtime is not installed or the log is unreadable.
// limit <= 0 returns all lines.
func (s *LogService) ReadSharedLog(name string, limit int) []string {
	base := s.SharedLogPath
	if base == "" {
		base = sharedLogPath
	}
	content, err := os.ReadFile(filepath.Join(base, name))
	if err != nil {
		return []string{}
	}

	trimmed := strings.TrimRight(string(content), "\n")
	if trimmed == "" {
		return []string{}
	}
	lines := strings.Split(trimmed, "\n")
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

// ExportDiagnostics writes the given report content to a timestamped file
// under the per-user log directory and reveals it in Finder.
func (s *LogService) ExportDiagnostics(ctx context.Context, content string) (string, error) {
	if err := os.MkdirAll(s.LogPath, 0700); err != nil {
		return "", fmt.Errorf("建立 log 目錄失敗: %w", err)
	}
	path := filepath.Join(s.LogPath, fmt.Sprintf("diagnostics-%s.txt", s.timeNow().Format("20060102-150405")))
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("寫入診斷報告失敗: %w", err)
	}

	open := s.openLocation
	if open == nil {
		open = openLocationInFinder
	}
	if err := open(ctx, path); err != nil {
		return "", fmt.Errorf("開啟診斷報告位置失敗: %w", err)
	}
	return path, nil
}

func (s *LogService) timeNow() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
