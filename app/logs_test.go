package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAppLogAndListAppLogEntries(t *testing.T) {
	service := testLogService(t)
	times := []time.Time{
		time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC),
	}
	index := 0
	service.now = func() time.Time {
		at := times[index]
		index++
		return at
	}

	if err := service.WriteAppLog("info", "first"); err != nil {
		t.Fatalf("write app log: %v", err)
	}
	if err := service.WriteAppLog("info", "second"); err != nil {
		t.Fatalf("write app log: %v", err)
	}
	if err := service.WriteAppLog("error", "third"); err != nil {
		t.Fatalf("write app log: %v", err)
	}

	entries, err := service.ListAppLogEntries(0)
	if err != nil {
		t.Fatalf("list app log entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %#v", entries)
	}
	if entries[0].Message != "third" || entries[0].Level != "error" {
		t.Fatalf("expected newest entry first, got %#v", entries[0])
	}
	if entries[2].Message != "first" {
		t.Fatalf("expected oldest entry last, got %#v", entries[2])
	}

	info, err := os.Stat(service.appLogPath())
	if err != nil {
		t.Fatalf("stat app log: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 perms, got %v", info.Mode().Perm())
	}
}

func TestListAppLogEntriesAppliesLimit(t *testing.T) {
	service := testLogService(t)
	for i := 0; i < 3; i++ {
		if err := service.WriteAppLog("info", "entry"); err != nil {
			t.Fatalf("write app log: %v", err)
		}
	}

	entries, err := service.ListAppLogEntries(2)
	if err != nil {
		t.Fatalf("list app log entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected limit to apply, got %d entries", len(entries))
	}
}

func TestListAppLogEntriesReturnsEmptyWhenMissing(t *testing.T) {
	service := testLogService(t)

	entries, err := service.ListAppLogEntries(0)
	if err != nil {
		t.Fatalf("list app log entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty entries, got %#v", entries)
	}
}

func TestReadSharedLogReturnsLinesWithinLimit(t *testing.T) {
	service := testLogService(t)
	if err := os.MkdirAll(service.SharedLogPath, 0755); err != nil {
		t.Fatalf("mkdir shared log path: %v", err)
	}
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(filepath.Join(service.SharedLogPath, "freshclam.log"), []byte(content), 0644); err != nil {
		t.Fatalf("write shared log: %v", err)
	}

	lines := service.ReadSharedLog("freshclam.log", 2)
	if len(lines) != 2 || lines[0] != "line2" || lines[1] != "line3" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestReadSharedLogReturnsEmptyWhenMissing(t *testing.T) {
	service := testLogService(t)

	lines := service.ReadSharedLog("clamd.log", 10)
	if len(lines) != 0 {
		t.Fatalf("expected empty lines, got %#v", lines)
	}
}

func TestExportDiagnosticsWritesReportAndRevealsLocation(t *testing.T) {
	service := testLogService(t)
	service.now = func() time.Time {
		return time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
	}
	var revealed string
	service.openLocation = func(_ context.Context, path string) error {
		revealed = path
		return nil
	}

	path, err := service.ExportDiagnostics(context.Background(), "report content")
	if err != nil {
		t.Fatalf("export diagnostics: %v", err)
	}
	if revealed != path {
		t.Fatalf("expected reveal of %s, got %s", path, revealed)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read diagnostics file: %v", err)
	}
	if string(content) != "report content" {
		t.Fatalf("unexpected content: %q", content)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat diagnostics file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 perms, got %v", info.Mode().Perm())
	}
}

func testLogService(t *testing.T) *LogService {
	t.Helper()
	base := t.TempDir()
	return &LogService{
		LogPath:       filepath.Join(base, "logs"),
		SharedLogPath: filepath.Join(base, "shared-logs"),
	}
}
