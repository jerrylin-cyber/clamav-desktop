package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// ScanResultsPage 為結果頁的一頁資料：當頁項目、套用篩選/搜尋後的總數，以及各狀態的筆數統計。
type ScanResultsPage struct {
	Items  []ScanResult   `json:"items"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
}

// ResultsStore 以 SQLite 保存逐檔掃描結果，支援掃描中逐批寫入與後端分頁查詢，
// 避免一次把數十萬筆結果載入記憶體或跨 Wails bridge 傳給前端。
type ResultsStore struct {
	dbPath    string
	legacyDir string // 舊版以 results/{jobID}.json 保存的結果，供惰性匯入

	mu sync.Mutex
	db *sql.DB
}

func newResultsStore(baseDir string) *ResultsStore {
	return &ResultsStore{
		dbPath:    filepath.Join(baseDir, "results.db"),
		legacyDir: filepath.Join(baseDir, "results"),
	}
}

func (s *ResultsStore) conn() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0700); err != nil {
		return nil, fmt.Errorf("建立資料目錄失敗: %w", err)
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return nil, fmt.Errorf("開啟結果資料庫失敗: %w", err)
	}
	// 序列化所有存取至單一連線，搭配 WAL 與 busy_timeout，避免掃描寫入與前端查詢互相鎖死。
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("設定結果資料庫失敗: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS results (
		job_id    TEXT NOT NULL,
		path      TEXT NOT NULL,
		status    TEXT NOT NULL,
		signature TEXT NOT NULL DEFAULT '',
		engine    TEXT NOT NULL DEFAULT '',
		error     TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("建立結果資料表失敗: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_results_job ON results(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_results_job_status ON results(job_id, status)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("建立結果索引失敗: %w", err)
		}
	}
	s.db = db
	return s.db, nil
}

// AppendResults 將一批掃描結果寫入指定工作（依插入順序，以 rowid 維持穩定排序）。
func (s *ResultsStore) AppendResults(jobID string, items []ScanResult) error {
	if len(items) == 0 {
		return nil
	}
	db, err := s.conn()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("開始結果交易失敗: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO results(job_id, path, status, signature, engine, error) VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("準備結果寫入失敗: %w", err)
	}
	defer stmt.Close()
	for _, item := range items {
		if _, err := stmt.Exec(jobID, item.Path, item.Status, item.Signature, item.Engine, item.Error); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("寫入掃描結果失敗: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交結果交易失敗: %w", err)
	}
	return nil
}

// QueryPage 依工作 ID 取得一頁結果（套用狀態篩選與路徑/簽章搜尋），並回傳篩選後總數與各狀態筆數。
func (s *ResultsStore) QueryPage(jobID, status, query string, offset, limit int) (ScanResultsPage, error) {
	if err := s.ensureImported(jobID); err != nil {
		return ScanResultsPage{}, err
	}
	db, err := s.conn()
	if err != nil {
		return ScanResultsPage{}, err
	}

	page := ScanResultsPage{Items: []ScanResult{}, Counts: map[string]int{}}

	countRows, err := db.Query(`SELECT status, COUNT(*) FROM results WHERE job_id = ? GROUP BY status`, jobID)
	if err != nil {
		return ScanResultsPage{}, fmt.Errorf("統計掃描結果失敗: %w", err)
	}
	all := 0
	for countRows.Next() {
		var st string
		var n int
		if err := countRows.Scan(&st, &n); err != nil {
			countRows.Close()
			return ScanResultsPage{}, fmt.Errorf("讀取結果統計失敗: %w", err)
		}
		page.Counts[st] = n
		all += n
	}
	if err := countRows.Err(); err != nil {
		countRows.Close()
		return ScanResultsPage{}, fmt.Errorf("讀取結果統計失敗: %w", err)
	}
	countRows.Close()
	page.Counts["all"] = all

	where := "job_id = ?"
	args := []any{jobID}
	if status != "" && status != "all" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if query != "" {
		where += " AND (path LIKE ? OR signature LIKE ?)"
		like := "%" + query + "%"
		args = append(args, like, like)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM results WHERE `+where, args...).Scan(&page.Total); err != nil {
		return ScanResultsPage{}, fmt.Errorf("計算結果總數失敗: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := db.Query(`SELECT path, status, signature, engine, error FROM results WHERE `+where+` ORDER BY rowid LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return ScanResultsPage{}, fmt.Errorf("查詢掃描結果失敗: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ScanResult
		if err := rows.Scan(&item.Path, &item.Status, &item.Signature, &item.Engine, &item.Error); err != nil {
			return ScanResultsPage{}, fmt.Errorf("讀取掃描結果失敗: %w", err)
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ScanResultsPage{}, fmt.Errorf("讀取掃描結果失敗: %w", err)
	}
	return page, nil
}

// LoadAll 讀取指定工作的全部結果，依插入順序回傳；供向後相容與測試使用。
func (s *ResultsStore) LoadAll(jobID string) ([]ScanResult, error) {
	if err := s.ensureImported(jobID); err != nil {
		return nil, err
	}
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT path, status, signature, engine, error FROM results WHERE job_id = ? ORDER BY rowid`, jobID)
	if err != nil {
		return nil, fmt.Errorf("查詢掃描結果失敗: %w", err)
	}
	defer rows.Close()
	results := []ScanResult{}
	for rows.Next() {
		var item ScanResult
		if err := rows.Scan(&item.Path, &item.Status, &item.Signature, &item.Engine, &item.Error); err != nil {
			return nil, fmt.Errorf("讀取掃描結果失敗: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// UpdateStatus 更新某工作中指定路徑結果的狀態（例如隔離、移到垃圾桶、永久刪除後）。
func (s *ResultsStore) UpdateStatus(jobID, path, status string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE results SET status = ? WHERE job_id = ? AND path = ?`, status, jobID, path); err != nil {
		return fmt.Errorf("更新掃描結果狀態失敗: %w", err)
	}
	return nil
}

// DeleteJob 移除單一工作的所有結果。
func (s *ResultsStore) DeleteJob(jobID string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM results WHERE job_id = ?`, jobID); err != nil {
		return fmt.Errorf("刪除掃描結果失敗: %w", err)
	}
	return nil
}

// DeleteAll 清空所有掃描結果。
func (s *ResultsStore) DeleteAll() error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM results`); err != nil {
		return fmt.Errorf("清除掃描結果失敗: %w", err)
	}
	return nil
}

// ensureImported 若工作在資料庫尚無任何結果、但存在舊版 JSON 結果檔，則匯入一次（保留原 JSON 檔不刪）。
func (s *ResultsStore) ensureImported(jobID string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM results WHERE job_id = ? LIMIT 1`, jobID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("檢查結果是否存在失敗: %w", err)
	}

	content, err := os.ReadFile(filepath.Join(s.legacyDir, jobID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("讀取舊版掃描結果失敗: %w", err)
	}
	var legacy []ScanResult
	if err := json.Unmarshal(content, &legacy); err != nil {
		return fmt.Errorf("解析舊版掃描結果失敗: %w", err)
	}
	return s.AppendResults(jobID, legacy)
}
