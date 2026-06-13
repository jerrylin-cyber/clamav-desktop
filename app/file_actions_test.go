package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenScanResultLocationUsesFinderReveal(t *testing.T) {
	file := writeActionFixture(t, "target.txt", "payload")
	var opened string
	service := testFileActionService(t)
	service.openLocation = func(_ context.Context, path string) error {
		opened = path
		return nil
	}

	if err := service.OpenScanResultLocation(context.Background(), ScanResult{Path: file}); err != nil {
		t.Fatalf("open scan result location: %v", err)
	}
	if opened != file {
		t.Fatalf("unexpected opened path: %s", opened)
	}
}

func TestQuarantineMovesFileAndStoresMetadata(t *testing.T) {
	file := writeActionFixture(t, "eicar.txt", "payload")
	service := testFileActionService(t)

	record, err := service.Quarantine(ScanResult{Path: file, Status: "infected", Signature: "Eicar-Test-Signature"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}

	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("expected original file to be removed, err=%v", err)
	}
	content, err := os.ReadFile(record.QuarantinePath)
	if err != nil {
		t.Fatalf("read quarantine file: %v", err)
	}
	if string(content) == "payload" {
		t.Fatal("expected quarantine file content to be encoded, not plaintext")
	}
	decoded := make([]byte, len(content))
	for i := range content {
		decoded[i] = content[i] ^ quarantineXORKey
	}
	if string(decoded) != "payload" {
		t.Fatalf("unexpected decoded quarantine content %q", string(decoded))
	}
	if record.Signature != "Eicar-Test-Signature" || record.Status != "quarantined" || record.SHA256 == "" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record.Encoding != quarantineEncodingXOR {
		t.Fatalf("expected xor encoding, got %q", record.Encoding)
	}
	// SHA256 必須是原始內容的雜湊，而非編碼後內容。
	originalSum := sha256.Sum256([]byte("payload"))
	if record.SHA256 != hex.EncodeToString(originalSum[:]) {
		t.Fatalf("sha256 should hash original content, got %s", record.SHA256)
	}

	stored, err := service.LoadRecord(record.ID)
	if err != nil {
		t.Fatalf("load record: %v", err)
	}
	if stored.OriginalPath != file || stored.QuarantinePath != record.QuarantinePath {
		t.Fatalf("stored record mismatch: %#v", stored)
	}
}

func TestRestoreMovesQuarantineFileBack(t *testing.T) {
	file := writeActionFixture(t, "restore.txt", "payload")
	service := testFileActionService(t)
	record, err := service.Quarantine(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}

	restored, err := service.Restore(record.ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	if restored.Status != "restored" || restored.RestoredAt == nil {
		t.Fatalf("unexpected restored record: %#v", restored)
	}
	if content, err := os.ReadFile(file); err != nil || string(content) != "payload" {
		t.Fatalf("unexpected restored file content %q err=%v", string(content), err)
	}
	if _, err := os.Stat(record.QuarantinePath); !os.IsNotExist(err) {
		t.Fatalf("expected quarantine file to move away, err=%v", err)
	}
}

func TestRestoreDecodesLegacyPlaintextQuarantine(t *testing.T) {
	service := testFileActionService(t)
	originalPath := filepath.Join(t.TempDir(), "legacy.txt")
	quarantineDir := filepath.Join(service.QuarantinePath, "files")
	if err := os.MkdirAll(quarantineDir, 0700); err != nil {
		t.Fatalf("mkdir quarantine: %v", err)
	}
	quarantinePath := filepath.Join(quarantineDir, "legacy.quarantine")
	if err := os.WriteFile(quarantinePath, []byte("payload"), 0600); err != nil {
		t.Fatalf("write legacy quarantine file: %v", err)
	}
	// Encoding 留空，模擬升級前未編碼的舊隔離檔。
	if err := service.saveRecord(QuarantineRecord{
		ID:             "legacy",
		OriginalPath:   originalPath,
		QuarantinePath: quarantinePath,
		Status:         "quarantined",
		Encoding:       quarantineEncodingNone,
	}); err != nil {
		t.Fatalf("save legacy record: %v", err)
	}

	if _, err := service.Restore("legacy"); err != nil {
		t.Fatalf("restore legacy: %v", err)
	}
	if content, err := os.ReadFile(originalPath); err != nil || string(content) != "payload" {
		t.Fatalf("unexpected restored content %q err=%v", string(content), err)
	}
	if _, err := os.Stat(quarantinePath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy quarantine file to move away, err=%v", err)
	}
}

func TestRestoreRefusesToOverwriteExistingFile(t *testing.T) {
	file := writeActionFixture(t, "conflict.txt", "payload")
	service := testFileActionService(t)
	record, err := service.Quarantine(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if err := os.WriteFile(file, []byte("new"), 0644); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	_, err = service.Restore(record.ID)
	if err == nil {
		t.Fatal("expected restore conflict error")
	}
}

func TestQuarantineRejectsCleanResult(t *testing.T) {
	file := writeActionFixture(t, "clean.txt", "payload")
	service := testFileActionService(t)

	if _, err := service.Quarantine(ScanResult{Path: file, Status: "clean"}); err == nil {
		t.Fatal("expected clean result quarantine rejection")
	}
}

func TestNewFileActionServiceUsesPerUserQuarantinePath(t *testing.T) {
	service := newFileActionService("/Users/jerry")
	expected := "/Users/jerry/Library/Application Support/ClamAVDesktop/quarantine"

	if service.QuarantinePath != expected {
		t.Fatalf("unexpected quarantine path: %s", service.QuarantinePath)
	}
}

func TestAppFileActionMethodsUseSameService(t *testing.T) {
	file := writeActionFixture(t, "app-eicar.txt", "payload")
	service := testFileActionService(t)
	app := &App{fileActions: service, logService: testLogService(t)}

	record, err := app.QuarantineScanResult(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("app quarantine: %v", err)
	}
	restored, err := app.RestoreQuarantineRecord(record.ID)
	if err != nil {
		t.Fatalf("app restore: %v", err)
	}
	if restored.Status != "restored" {
		t.Fatalf("unexpected app restored record: %#v", restored)
	}
}

func TestMoveToTrashInvokesRunnerAndWritesAuditLog(t *testing.T) {
	file := writeActionFixture(t, "trash-me.txt", "payload")
	service := testFileActionService(t)
	var trashed string
	service.moveToTrash = func(_ context.Context, path string) error {
		trashed = path
		return os.Remove(path)
	}

	if err := service.MoveToTrash(context.Background(), file); err != nil {
		t.Fatalf("move to trash: %v", err)
	}
	if trashed != file {
		t.Fatalf("unexpected trashed path: %s", trashed)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, err=%v", err)
	}

	entries := readAuditLog(t, service)
	if len(entries) != 1 || entries[0].Action != "move-to-trash" || entries[0].Path != file {
		t.Fatalf("unexpected audit log entries: %#v", entries)
	}
}

func TestMoveToTrashRejectsMissingPath(t *testing.T) {
	service := testFileActionService(t)
	called := false
	service.moveToTrash = func(_ context.Context, _ string) error {
		called = true
		return nil
	}

	missing := filepath.Join(t.TempDir(), "missing.txt")
	if err := service.MoveToTrash(context.Background(), missing); err == nil {
		t.Fatal("expected error for missing path")
	}
	if called {
		t.Fatal("trash runner must not be invoked for missing path")
	}
}

func TestPermanentlyDeleteRemovesFileAndWritesAuditLog(t *testing.T) {
	file := writeActionFixture(t, "delete-me.txt", "payload")
	service := testFileActionService(t)

	if err := service.PermanentlyDelete(file); err != nil {
		t.Fatalf("permanently delete: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, err=%v", err)
	}

	entries := readAuditLog(t, service)
	if len(entries) != 1 || entries[0].Action != "permanent-delete" || entries[0].Path != file {
		t.Fatalf("unexpected audit log entries: %#v", entries)
	}
}

func TestMoveQuarantineToTrashUpdatesStatus(t *testing.T) {
	file := writeActionFixture(t, "quarantine-trash.txt", "payload")
	service := testFileActionService(t)
	service.moveToTrash = func(_ context.Context, path string) error {
		return os.Remove(path)
	}

	record, err := service.Quarantine(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}

	trashed, err := service.MoveQuarantineToTrash(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("move quarantine to trash: %v", err)
	}
	if trashed.Status != "trashed" {
		t.Fatalf("unexpected status: %s", trashed.Status)
	}
	if _, err := os.Stat(record.QuarantinePath); !os.IsNotExist(err) {
		t.Fatalf("expected quarantine file to be removed, err=%v", err)
	}

	stored, err := service.LoadRecord(record.ID)
	if err != nil {
		t.Fatalf("load record: %v", err)
	}
	if stored.Status != "trashed" {
		t.Fatalf("stored record not updated: %#v", stored)
	}
}

func TestPermanentlyDeleteQuarantineUpdatesStatus(t *testing.T) {
	file := writeActionFixture(t, "quarantine-delete.txt", "payload")
	service := testFileActionService(t)

	record, err := service.Quarantine(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}

	deleted, err := service.PermanentlyDeleteQuarantine(record.ID)
	if err != nil {
		t.Fatalf("permanently delete quarantine: %v", err)
	}
	if deleted.Status != "deleted" {
		t.Fatalf("unexpected status: %s", deleted.Status)
	}
	if _, err := os.Stat(record.QuarantinePath); !os.IsNotExist(err) {
		t.Fatalf("expected quarantine file to be removed, err=%v", err)
	}
}

func TestMoveOrDeleteQuarantineRejectsNonQuarantinedStatus(t *testing.T) {
	file := writeActionFixture(t, "restored.txt", "payload")
	service := testFileActionService(t)
	service.moveToTrash = func(_ context.Context, path string) error {
		return os.Remove(path)
	}

	record, err := service.Quarantine(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if _, err := service.Restore(record.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if _, err := service.MoveQuarantineToTrash(context.Background(), record.ID); err == nil {
		t.Fatal("expected error moving restored record to trash")
	}
	if _, err := service.PermanentlyDeleteQuarantine(record.ID); err == nil {
		t.Fatal("expected error permanently deleting restored record")
	}
}

func TestListQuarantineRecordsReturnsSortedRecords(t *testing.T) {
	service := testFileActionService(t)
	older := QuarantineRecord{ID: "older", OriginalPath: "/tmp/older", QuarantinePath: "/tmp/older.q", Status: "quarantined", DetectedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	newer := QuarantineRecord{ID: "newer", OriginalPath: "/tmp/newer", QuarantinePath: "/tmp/newer.q", Status: "quarantined", DetectedAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)}
	if err := service.saveRecord(older); err != nil {
		t.Fatalf("save older record: %v", err)
	}
	if err := service.saveRecord(newer); err != nil {
		t.Fatalf("save newer record: %v", err)
	}

	records, err := service.ListQuarantineRecords()
	if err != nil {
		t.Fatalf("list quarantine records: %v", err)
	}
	if len(records) != 2 || records[0].ID != "newer" || records[1].ID != "older" {
		t.Fatalf("unexpected record order: %#v", records)
	}
}

func TestListQuarantineRecordsReturnsEmptyWhenMissing(t *testing.T) {
	service := testFileActionService(t)

	records, err := service.ListQuarantineRecords()
	if err != nil {
		t.Fatalf("list quarantine records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no records, got %#v", records)
	}
}

func TestAppQuarantineListAndTrashMethodsUseSameService(t *testing.T) {
	file := writeActionFixture(t, "app-trash.txt", "payload")
	service := testFileActionService(t)
	service.moveToTrash = func(_ context.Context, path string) error {
		return os.Remove(path)
	}
	app := &App{fileActions: service, logService: testLogService(t)}

	record, err := app.QuarantineScanResult(ScanResult{Path: file, Status: "infected", Signature: "Sig"})
	if err != nil {
		t.Fatalf("app quarantine: %v", err)
	}

	records, err := app.ListQuarantineRecords()
	if err != nil {
		t.Fatalf("app list quarantine records: %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("unexpected records: %#v", records)
	}

	trashed, err := app.MoveQuarantineRecordToTrash(record.ID)
	if err != nil {
		t.Fatalf("app move quarantine to trash: %v", err)
	}
	if trashed.Status != "trashed" {
		t.Fatalf("unexpected status: %s", trashed.Status)
	}
}

func TestAppMoveAndDeleteScanResultUseSameService(t *testing.T) {
	trashFile := writeActionFixture(t, "app-scan-trash.txt", "payload")
	deleteFile := writeActionFixture(t, "app-scan-delete.txt", "payload")
	service := testFileActionService(t)
	service.moveToTrash = func(_ context.Context, path string) error {
		return os.Remove(path)
	}
	app := &App{fileActions: service, logService: testLogService(t)}

	if err := app.MoveScanResultToTrash(ScanResult{Path: trashFile}); err != nil {
		t.Fatalf("app move scan result to trash: %v", err)
	}
	if _, err := os.Stat(trashFile); !os.IsNotExist(err) {
		t.Fatalf("expected trashed file to be removed, err=%v", err)
	}

	if err := app.PermanentlyDeleteScanResult(ScanResult{Path: deleteFile}); err != nil {
		t.Fatalf("app permanently delete scan result: %v", err)
	}
	if _, err := os.Stat(deleteFile); !os.IsNotExist(err) {
		t.Fatalf("expected deleted file to be removed, err=%v", err)
	}
}

func readAuditLog(t *testing.T, service *FileActionService) []AuditEntry {
	t.Helper()
	content, err := os.ReadFile(service.auditLogPath())
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}

	var entries []AuditEntry
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse audit log entry: %v", err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func testFileActionService(t *testing.T) *FileActionService {
	t.Helper()
	return &FileActionService{
		QuarantinePath: filepath.Join(t.TempDir(), "quarantine"),
		newID: func() string {
			return "record-test"
		},
		now: func() time.Time {
			return time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
		},
	}
}

func writeActionFixture(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write action fixture: %v", err)
	}
	return path
}
