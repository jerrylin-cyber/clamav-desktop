package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ScanJob 代表一次掃描工作的中繼資料與彙總統計，持久化後可在「結果」與「紀錄」頁查看。
type ScanJob struct {
	ID           string      `json:"id"`
	Paths        []string    `json:"paths"`
	Options      ScanOptions `json:"options"`
	Status       string      `json:"status"`
	StartedAt    time.Time   `json:"startedAt"`
	EndedAt      *time.Time  `json:"endedAt"`
	ScannedFiles int         `json:"scannedFiles"`
	Detections   int         `json:"detections"`
	Errors       int         `json:"errors"`
}

// ScanOptions 為掃描選項，例如是否遞迴掃描子目錄。
type ScanOptions struct {
	Recursive bool `json:"recursive"`
	AllMatch  bool `json:"allMatch"`
}

// ScanProgressEvent 為掃描進行中的即時進度事件，透過 Wails 事件推播給前端更新畫面。
type ScanProgressEvent struct {
	JobID        string `json:"jobId"`
	Status       string `json:"status"`
	CurrentPath  string `json:"currentPath"`
	ScannedFiles int    `json:"scannedFiles"`
	Detections   int    `json:"detections"`
	Errors       int    `json:"errors"`
}

// ScanResult 為單一檔案的掃描結果（乾淨、感染、錯誤等），感染時附上病毒簽章名稱。
type ScanResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Signature string `json:"signature"`
	Engine    string `json:"engine"`
	Error     string `json:"error"`
}

type scanFileFunc func(ctx context.Context, path string) (string, error)

// scanResultBatchSize 為掃描中逐批寫入結果庫的批次大小，平衡寫入次數與崩潰時的結果遺失量。
const scanResultBatchSize = 200

// ScanJobManager 建立、執行、查詢與取消掃描工作，並負責工作與結果的持久化。
type ScanJobManager struct {
	JobsPath    string
	ResultsPath string
	results     *ResultsStore
	scanFile    scanFileFunc
	now         func() time.Time
	newID       func() string

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func newScanJobManager(homeDir string, client ClamDClient) *ScanJobManager {
	base := filepath.Join(homeDir, "Library/Application Support/ClamAVDesktop")
	return &ScanJobManager{
		JobsPath:    filepath.Join(base, "jobs"),
		ResultsPath: filepath.Join(base, "results"),
		results:     newResultsStore(base),
		scanFile: func(ctx context.Context, path string) (string, error) {
			reply, err := client.InstreamFile(ctx, path)
			if errors.Is(err, errStreamMaxLength) {
				// 超過 INSTREAM 串流上限的大檔，改用 clamd SCAN 依路徑掃描（clamd 直接讀磁碟，無串流上限）
				return client.Scan(ctx, path)
			}
			return reply, err
		},
	}
}

// CreateScanJob 依路徑與選項建立並持久化一筆新的掃描工作（初始狀態），但不立即執行。
func (m *ScanJobManager) CreateScanJob(paths []string, options ScanOptions) (ScanJob, error) {
	normalizedPaths, err := normalizeScanPaths(paths)
	if err != nil {
		return ScanJob{}, err
	}

	job := ScanJob{
		ID:        m.nextID(),
		Paths:     normalizedPaths,
		Options:   options,
		Status:    "queued",
		StartedAt: m.timeNow(),
	}
	if err := m.saveJob(job); err != nil {
		return ScanJob{}, err
	}
	return job, nil
}

// RunScan 走訪指定路徑逐檔掃描，過程透過 emit 推播進度，結束後保存工作統計與結果並回傳；支援透過 context 取消。
func (m *ScanJobManager) RunScan(ctx context.Context, paths []string, options ScanOptions, emit func(ScanProgressEvent)) (ScanJob, []ScanResult, error) {
	job, err := m.CreateScanJob(paths, options)
	if err != nil {
		return ScanJob{}, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	m.registerCancel(job.ID, cancel)
	defer m.unregisterCancel(job.ID)
	defer cancel()

	job.Status = "scanning"
	if err := m.saveJob(job); err != nil {
		return job, nil, err
	}

	files, preResults := m.collectFiles(job.Paths, options)
	results := append([]ScanResult{}, preResults...)
	progress := ScanProgressEvent{JobID: job.ID, Status: job.Status}
	for _, result := range preResults {
		if result.Status == "infected" {
			progress.Detections++
		}
		if result.Status == "error" || result.Status == "skipped" {
			progress.Errors++
		}
	}

	// 逐批 durable 寫入結果庫：掃描中即時持久化，app 被強制結束時已掃部分不會遺失。
	pending := append([]ScanResult{}, preResults...)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		_ = m.results.AppendResults(job.ID, pending)
		pending = pending[:0]
	}

	for _, path := range files {
		if err := ctx.Err(); err != nil {
			flush()
			job.Status = "canceled"
			ended := m.timeNow()
			job.EndedAt = &ended
			job.ScannedFiles = progress.ScannedFiles
			job.Detections = progress.Detections
			job.Errors = progress.Errors
			_ = m.saveJob(job)
			progress.Status = job.Status
			emitScanProgress(emit, progress)
			return job, results, err
		}

		progress.CurrentPath = path
		emitScanProgress(emit, progress)

		reply, scanErr := m.scanner()(ctx, path)
		if errors.Is(scanErr, context.Canceled) {
			flush()
			job.Status = "canceled"
			ended := m.timeNow()
			job.EndedAt = &ended
			job.ScannedFiles = progress.ScannedFiles
			job.Detections = progress.Detections
			job.Errors = progress.Errors
			_ = m.saveJob(job)
			progress.Status = job.Status
			emitScanProgress(emit, progress)
			return job, results, scanErr
		}
		result := scanResultFromReply(path, reply, scanErr)
		results = append(results, result)
		pending = append(pending, result)
		if len(pending) >= scanResultBatchSize {
			flush()
		}
		progress.ScannedFiles++
		if result.Status == "infected" {
			progress.Detections++
		}
		if result.Status == "error" || result.Status == "skipped" {
			progress.Errors++
		}
		emitScanProgress(emit, progress)
	}

	flush()
	ended := m.timeNow()
	job.EndedAt = &ended
	job.ScannedFiles = progress.ScannedFiles
	job.Detections = progress.Detections
	job.Errors = progress.Errors
	if progress.Errors > 0 {
		job.Status = "completed-with-warnings"
	} else {
		job.Status = "completed"
	}
	progress.Status = job.Status
	if err := m.saveJob(job); err != nil {
		return job, results, err
	}
	emitScanProgress(emit, progress)
	return job, results, nil
}

// GetScanJob 依 ID 讀取已保存的掃描工作中繼資料。
func (m *ScanJobManager) GetScanJob(id string) (ScanJob, error) {
	content, err := os.ReadFile(m.jobPath(id))
	if err != nil {
		return ScanJob{}, fmt.Errorf("讀取 scan job 失敗: %w", err)
	}
	var job ScanJob
	if err := json.Unmarshal(content, &job); err != nil {
		return ScanJob{}, fmt.Errorf("解析 scan job 失敗: %w", err)
	}
	return job, nil
}

// LoadResults 依工作 ID 讀取該次掃描保存的逐檔結果清單（來源為結果庫，必要時自舊版 JSON 惰性匯入）。
func (m *ScanJobManager) LoadResults(jobID string) ([]ScanResult, error) {
	return m.results.LoadAll(jobID)
}

// LoadResultsPage 依工作 ID 取得一頁結果，套用狀態篩選與路徑/簽章搜尋，並回傳篩選後總數與各狀態筆數。
func (m *ScanJobManager) LoadResultsPage(jobID, status, query string, offset, limit int) (ScanResultsPage, error) {
	return m.results.QueryPage(jobID, status, query, offset, limit)
}

// ListScanJobs returns all scan jobs for the current user, most recently
// started first.
func (m *ScanJobManager) ListScanJobs() ([]ScanJob, error) {
	entries, err := os.ReadDir(m.JobsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []ScanJob{}, nil
		}
		return nil, fmt.Errorf("讀取 scan jobs 失敗: %w", err)
	}

	jobs := make([]ScanJob, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		job, err := m.GetScanJob(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].StartedAt.After(jobs[j].StartedAt)
	})
	return jobs, nil
}

// HasRunningJobs 回報目前是否有執行中的掃描工作（供關閉視窗時決定是否保留背景 process）。
func (m *ScanJobManager) HasRunningJobs() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running) > 0
}

// MarkInterruptedJobs 在 app 啟動時，將仍停留在 queued/scanning 但已無對應執行中程序的工作標記為 interrupted，
// 避免狀態永久卡住，並讓前端能提供「重新掃描」。回傳被標記的工作數。
func (m *ScanJobManager) MarkInterruptedJobs() (int, error) {
	jobs, err := m.ListScanJobs()
	if err != nil {
		return 0, err
	}
	marked := 0
	for _, job := range jobs {
		if job.Status != "queued" && job.Status != "scanning" {
			continue
		}
		job.Status = "interrupted"
		if job.EndedAt == nil {
			ended := m.timeNow()
			job.EndedAt = &ended
		}
		if err := m.saveJob(job); err != nil {
			return marked, err
		}
		marked++
	}
	return marked, nil
}

// CancelScanJob 取消執行中的掃描工作，回傳是否確實有對應的執行中工作被取消。
func (m *ScanJobManager) CancelScanJob(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cancel, ok := m.running[id]
	if !ok {
		return false
	}
	cancel()
	return true
}

func (m *ScanJobManager) collectFiles(paths []string, options ScanOptions) ([]string, []ScanResult) {
	var files []string
	var results []ScanResult
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			results = append(results, scanResultFromReply(path, "", err))
			continue
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		if !options.Recursive {
			results = append(results, ScanResult{Path: path, Status: "skipped", Error: "目錄掃描需要啟用 recursive"})
			continue
		}
		err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				results = append(results, scanResultFromReply(current, "", walkErr))
				return nil
			}
			if entry.Type().IsRegular() {
				files = append(files, current)
			}
			return nil
		})
		if err != nil {
			results = append(results, scanResultFromReply(path, "", err))
		}
	}
	return files, results
}

func (m *ScanJobManager) saveJob(job ScanJob) error {
	return writeJSONFile(m.jobPath(job.ID), job)
}

func (m *ScanJobManager) jobPath(id string) string {
	return filepath.Join(m.JobsPath, id+".json")
}

func (m *ScanJobManager) scanner() scanFileFunc {
	if m.scanFile != nil {
		return m.scanFile
	}
	return func(context.Context, string) (string, error) {
		return "", errors.New("scan file function 未設定")
	}
}

func (m *ScanJobManager) timeNow() time.Time {
	if m.now != nil {
		return m.now().UTC()
	}
	return time.Now().UTC()
}

func (m *ScanJobManager) nextID() string {
	if m.newID != nil {
		return m.newID()
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("scan_%d", time.Now().UnixNano())
	}
	return "scan_" + hex.EncodeToString(random[:])
}

func (m *ScanJobManager) registerCancel(id string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == nil {
		m.running = map[string]context.CancelFunc{}
	}
	m.running[id] = cancel
}

func (m *ScanJobManager) unregisterCancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, id)
}

func normalizeScanPaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		normalized = append(normalized, path)
	}
	if len(normalized) == 0 {
		return nil, errors.New("scan paths 不可為空")
	}
	return normalized, nil
}

func scanResultFromReply(path string, reply string, err error) ScanResult {
	if err != nil {
		var fileErr FileReadError
		if errors.As(err, &fileErr) {
			return ScanResult{Path: path, Status: "skipped", Error: fileErr.Reason}
		}
		return ScanResult{Path: path, Status: "error", Error: err.Error()}
	}
	reply = strings.TrimSpace(reply)
	if strings.HasSuffix(reply, " FOUND") {
		signature := strings.TrimSuffix(reply, " FOUND")
		if colon := strings.LastIndex(signature, ":"); colon >= 0 {
			signature = strings.TrimSpace(signature[colon+1:])
		}
		return ScanResult{Path: path, Status: "infected", Signature: signature, Engine: "clamd"}
	}
	if strings.HasSuffix(reply, " OK") || reply == "stream: OK" {
		return ScanResult{Path: path, Status: "clean", Engine: "clamd"}
	}
	if strings.Contains(reply, "ERROR") {
		return ScanResult{Path: path, Status: "error", Engine: "clamd", Error: reply}
	}
	return ScanResult{Path: path, Status: "error", Engine: "clamd", Error: "無法解析 clamd 掃描結果: " + reply}
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("建立資料目錄失敗: %w", err)
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失敗: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0600); err != nil {
		return fmt.Errorf("寫入 JSON 失敗: %w", err)
	}
	return nil
}

func emitScanProgress(emit func(ScanProgressEvent), event ScanProgressEvent) {
	if emit != nil {
		emit(event)
	}
}
