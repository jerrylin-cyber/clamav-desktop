package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// quarantineXORKey 對隔離檔內容做單一位元組 XOR，使惡意 payload 不以原始形態
// 留存在磁碟上，降低被其他程式直接讀取或誤觸發的風險。XOR 為對稱運算，
// 還原時套用同一把 key 即可解碼。
const quarantineXORKey byte = 0xFF

// 隔離檔的編碼標記。空字串代表舊版未編碼的明文隔離檔，向後相容。
const (
	quarantineEncodingNone = ""
	quarantineEncodingXOR  = "xor"
)

type QuarantineRecord struct {
	ID             string     `json:"id"`
	OriginalPath   string     `json:"originalPath"`
	QuarantinePath string     `json:"quarantinePath"`
	Signature      string     `json:"signature"`
	DetectedAt     time.Time  `json:"detectedAt"`
	SHA256         string     `json:"sha256"`
	Status         string     `json:"status"`
	Encoding       string     `json:"encoding"`
	RestoredAt     *time.Time `json:"restoredAt"`
}

// xorWriter 在寫入底層 writer 前對每個位元組套用 XOR key。
type xorWriter struct {
	w   io.Writer
	key byte
}

func (x *xorWriter) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	for i := range p {
		buf[i] = p[i] ^ x.key
	}
	return x.w.Write(buf)
}

type openLocationRunner func(ctx context.Context, path string) error

type trashRunner func(ctx context.Context, path string) error

type AuditEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Path   string    `json:"path"`
}

type FileActionService struct {
	QuarantinePath string
	openLocation   openLocationRunner
	moveToTrash    trashRunner
	now            func() time.Time
	newID          func() string
}

func newFileActionService(homeDir string) *FileActionService {
	return &FileActionService{
		QuarantinePath: filepath.Join(homeDir, "Library/Application Support/ClamAVDesktop/quarantine"),
	}
}

func (s *FileActionService) OpenScanResultLocation(ctx context.Context, result ScanResult) error {
	return s.openAllowedLocation(ctx, result.Path)
}

func (s *FileActionService) OpenQuarantineLocation(ctx context.Context, record QuarantineRecord) error {
	if record.Status == "restored" {
		return s.openAllowedLocation(ctx, record.OriginalPath)
	}
	return s.openAllowedLocation(ctx, record.QuarantinePath)
}

func (s *FileActionService) Quarantine(result ScanResult) (QuarantineRecord, error) {
	if strings.TrimSpace(result.Path) == "" {
		return QuarantineRecord{}, errors.New("scan result path 不可為空")
	}
	if result.Status != "infected" {
		return QuarantineRecord{}, errors.New("只有 infected scan result 可隔離")
	}

	source, err := os.Open(result.Path)
	if err != nil {
		return QuarantineRecord{}, fmt.Errorf("開啟隔離來源失敗: %w", err)
	}
	defer source.Close()

	id := s.nextID()
	filesDir := filepath.Join(s.QuarantinePath, "files")
	if err := os.MkdirAll(filesDir, 0700); err != nil {
		return QuarantineRecord{}, fmt.Errorf("建立 quarantine 目錄失敗: %w", err)
	}

	targetPath := filepath.Join(filesDir, id+".quarantine")
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return QuarantineRecord{}, fmt.Errorf("建立 quarantine 檔案失敗: %w", err)
	}

	// SHA256 取原始內容的雜湊（可比對威脅情報）；磁碟上以 XOR 編碼寫入。
	hasher := sha256.New()
	encoded := &xorWriter{w: target, key: quarantineXORKey}
	if _, err := io.Copy(io.MultiWriter(hasher, encoded), source); err != nil {
		_ = target.Close()
		_ = os.Remove(targetPath)
		return QuarantineRecord{}, fmt.Errorf("寫入 quarantine 檔案失敗: %w", err)
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(targetPath)
		return QuarantineRecord{}, fmt.Errorf("關閉 quarantine 檔案失敗: %w", err)
	}

	if err := os.Remove(result.Path); err != nil {
		_ = os.Remove(targetPath)
		return QuarantineRecord{}, fmt.Errorf("移除原始檔案失敗: %w", err)
	}

	record := QuarantineRecord{
		ID:             id,
		OriginalPath:   result.Path,
		QuarantinePath: targetPath,
		Signature:      result.Signature,
		DetectedAt:     s.timeNow(),
		SHA256:         hex.EncodeToString(hasher.Sum(nil)),
		Status:         "quarantined",
		Encoding:       quarantineEncodingXOR,
	}
	if err := s.saveRecord(record); err != nil {
		return QuarantineRecord{}, err
	}
	return record, nil
}

func (s *FileActionService) Restore(recordID string) (QuarantineRecord, error) {
	record, err := s.LoadRecord(recordID)
	if err != nil {
		return QuarantineRecord{}, err
	}
	if record.Status != "quarantined" {
		return QuarantineRecord{}, errors.New("quarantine record 不在 quarantined 狀態")
	}
	if _, err := os.Stat(record.OriginalPath); err == nil {
		return QuarantineRecord{}, errors.New("原位置已有檔案，請改名或取消 restore")
	} else if !errors.Is(err, os.ErrNotExist) {
		return QuarantineRecord{}, fmt.Errorf("檢查原位置失敗: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(record.OriginalPath), 0700); err != nil {
		return QuarantineRecord{}, fmt.Errorf("建立 restore 目錄失敗: %w", err)
	}
	if err := restoreQuarantineFile(record); err != nil {
		return QuarantineRecord{}, err
	}
	restoredAt := s.timeNow()
	record.RestoredAt = &restoredAt
	record.Status = "restored"
	if err := s.saveRecord(record); err != nil {
		return QuarantineRecord{}, err
	}
	return record, nil
}

// restoreQuarantineFile 將隔離檔還原回原位置。XOR 編碼的隔離檔需解碼後寫回；
// 舊版未編碼（明文）的隔離檔則沿用 rename，避免無謂的複製。
func restoreQuarantineFile(record QuarantineRecord) error {
	if record.Encoding == quarantineEncodingNone {
		if err := os.Rename(record.QuarantinePath, record.OriginalPath); err != nil {
			return fmt.Errorf("restore 檔案失敗: %w", err)
		}
		return nil
	}

	source, err := os.Open(record.QuarantinePath)
	if err != nil {
		return fmt.Errorf("開啟 quarantine 檔案失敗: %w", err)
	}
	defer source.Close()

	target, err := os.OpenFile(record.OriginalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("建立 restore 檔案失敗: %w", err)
	}

	decoded := &xorWriter{w: target, key: quarantineXORKey}
	if _, err := io.Copy(decoded, source); err != nil {
		_ = target.Close()
		_ = os.Remove(record.OriginalPath)
		return fmt.Errorf("解碼 quarantine 檔案失敗: %w", err)
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(record.OriginalPath)
		return fmt.Errorf("關閉 restore 檔案失敗: %w", err)
	}
	if err := os.Remove(record.QuarantinePath); err != nil {
		return fmt.Errorf("移除 quarantine 檔案失敗: %w", err)
	}
	return nil
}

func (s *FileActionService) LoadRecord(id string) (QuarantineRecord, error) {
	content, err := os.ReadFile(s.recordPath(id))
	if err != nil {
		return QuarantineRecord{}, fmt.Errorf("讀取 quarantine metadata 失敗: %w", err)
	}
	var record QuarantineRecord
	if err := json.Unmarshal(content, &record); err != nil {
		return QuarantineRecord{}, fmt.Errorf("解析 quarantine metadata 失敗: %w", err)
	}
	return record, nil
}

// ListQuarantineRecords returns all quarantine records for the current user,
// most recently detected first.
func (s *FileActionService) ListQuarantineRecords() ([]QuarantineRecord, error) {
	dir := filepath.Join(s.QuarantinePath, "records")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []QuarantineRecord{}, nil
		}
		return nil, fmt.Errorf("讀取 quarantine records 失敗: %w", err)
	}

	records := make([]QuarantineRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.LoadRecord(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].DetectedAt.After(records[j].DetectedAt)
	})
	return records, nil
}

// MoveToTrash moves the file at path to the macOS Trash via Finder, so it can
// be recovered by the user, and records the action in the per-user audit log.
func (s *FileActionService) MoveToTrash(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path 不可為空")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("目標檔案不存在或無法讀取: %w", err)
	}
	trash := s.moveToTrash
	if trash == nil {
		trash = moveToTrashViaFinder
	}
	if err := trash(ctx, path); err != nil {
		return fmt.Errorf("移到垃圾桶失敗: %w", err)
	}
	return s.appendAuditLog(AuditEntry{At: s.timeNow(), Action: "move-to-trash", Path: path})
}

// PermanentlyDelete removes the file at path immediately and records the
// action in the per-user audit log. Callers must obtain user confirmation
// before calling this, since the action is irreversible.
func (s *FileActionService) PermanentlyDelete(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path 不可為空")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("目標檔案不存在或無法讀取: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("永久刪除失敗: %w", err)
	}
	return s.appendAuditLog(AuditEntry{At: s.timeNow(), Action: "permanent-delete", Path: path})
}

// MoveQuarantineToTrash moves a quarantined file's payload to the Trash and
// marks the record as trashed.
func (s *FileActionService) MoveQuarantineToTrash(ctx context.Context, id string) (QuarantineRecord, error) {
	record, err := s.LoadRecord(id)
	if err != nil {
		return QuarantineRecord{}, err
	}
	if record.Status != "quarantined" {
		return QuarantineRecord{}, errors.New("quarantine record 不在 quarantined 狀態")
	}
	if err := s.MoveToTrash(ctx, record.QuarantinePath); err != nil {
		return QuarantineRecord{}, err
	}
	record.Status = "trashed"
	if err := s.saveRecord(record); err != nil {
		return QuarantineRecord{}, err
	}
	return record, nil
}

// PermanentlyDeleteQuarantine permanently deletes a quarantined file's
// payload and marks the record as deleted. Callers must obtain user
// confirmation before calling this, since the action is irreversible.
func (s *FileActionService) PermanentlyDeleteQuarantine(id string) (QuarantineRecord, error) {
	record, err := s.LoadRecord(id)
	if err != nil {
		return QuarantineRecord{}, err
	}
	if record.Status != "quarantined" {
		return QuarantineRecord{}, errors.New("quarantine record 不在 quarantined 狀態")
	}
	if err := s.PermanentlyDelete(record.QuarantinePath); err != nil {
		return QuarantineRecord{}, err
	}
	record.Status = "deleted"
	if err := s.saveRecord(record); err != nil {
		return QuarantineRecord{}, err
	}
	return record, nil
}

func (s *FileActionService) openAllowedLocation(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("open location path 不可為空")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("目標檔案不存在或無法讀取: %w", err)
	}
	open := s.openLocation
	if open == nil {
		open = openLocationInFinder
	}
	return open(ctx, path)
}

func (s *FileActionService) saveRecord(record QuarantineRecord) error {
	return writeJSONFile(s.recordPath(record.ID), record)
}

func (s *FileActionService) recordPath(id string) string {
	return filepath.Join(s.QuarantinePath, "records", id+".json")
}

func (s *FileActionService) auditLogPath() string {
	return filepath.Join(filepath.Dir(s.QuarantinePath), "audit.log")
}

func (s *FileActionService) appendAuditLog(entry AuditEntry) error {
	path := s.auditLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("建立 audit log 目錄失敗: %w", err)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化 audit log 失敗: %w", err)
	}
	line = append(line, '\n')

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("開啟 audit log 失敗: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("寫入 audit log 失敗: %w", err)
	}
	return nil
}

func (s *FileActionService) timeNow() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *FileActionService) nextID() string {
	if s.newID != nil {
		return s.newID()
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("quarantine_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(random[:])
}

func openLocationInFinder(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "/usr/bin/open", "-R", path).Run()
}

// moveToTrashViaFinder asks Finder to move path to the Trash, which is the
// macOS-native Trash behaviour (recoverable, appears in Finder's Trash).
func moveToTrashViaFinder(ctx context.Context, path string) error {
	script := fmt.Sprintf(`tell application "Finder" to delete POSIX file %s`, appleScriptQuote(path))
	return exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).Run()
}

func appleScriptQuote(path string) string {
	escaped := strings.ReplaceAll(path, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
