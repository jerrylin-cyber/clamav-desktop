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

type ScanOptions struct {
	Recursive bool `json:"recursive"`
	AllMatch  bool `json:"allMatch"`
}

type ScanProgressEvent struct {
	JobID        string `json:"jobId"`
	Status       string `json:"status"`
	CurrentPath  string `json:"currentPath"`
	ScannedFiles int    `json:"scannedFiles"`
	Detections   int    `json:"detections"`
	Errors       int    `json:"errors"`
}

type ScanResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Signature string `json:"signature"`
	Engine    string `json:"engine"`
	Error     string `json:"error"`
}

type scanFileFunc func(ctx context.Context, path string) (string, error)

type ScanJobManager struct {
	JobsPath    string
	ResultsPath string
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

	for _, path := range files {
		if err := ctx.Err(); err != nil {
			job.Status = "canceled"
			ended := m.timeNow()
			job.EndedAt = &ended
			job.ScannedFiles = progress.ScannedFiles
			job.Detections = progress.Detections
			job.Errors = progress.Errors
			_ = m.saveJob(job)
			_ = m.saveResults(job.ID, results)
			progress.Status = job.Status
			emitScanProgress(emit, progress)
			return job, results, err
		}

		progress.CurrentPath = path
		emitScanProgress(emit, progress)

		reply, scanErr := m.scanner()(ctx, path)
		if errors.Is(scanErr, context.Canceled) {
			job.Status = "canceled"
			ended := m.timeNow()
			job.EndedAt = &ended
			job.ScannedFiles = progress.ScannedFiles
			job.Detections = progress.Detections
			job.Errors = progress.Errors
			_ = m.saveJob(job)
			_ = m.saveResults(job.ID, results)
			progress.Status = job.Status
			emitScanProgress(emit, progress)
			return job, results, scanErr
		}
		result := scanResultFromReply(path, reply, scanErr)
		results = append(results, result)
		progress.ScannedFiles++
		if result.Status == "infected" {
			progress.Detections++
		}
		if result.Status == "error" || result.Status == "skipped" {
			progress.Errors++
		}
		emitScanProgress(emit, progress)
	}

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
	if err := m.saveResults(job.ID, results); err != nil {
		job.Status = "failed"
		progress.Status = job.Status
		progress.Errors++
		_ = m.saveJob(job)
		emitScanProgress(emit, progress)
		return job, results, err
	}
	if err := m.saveJob(job); err != nil {
		return job, results, err
	}
	emitScanProgress(emit, progress)
	return job, results, nil
}

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

func (m *ScanJobManager) LoadResults(jobID string) ([]ScanResult, error) {
	content, err := os.ReadFile(m.resultsPath(jobID))
	if err != nil {
		return nil, fmt.Errorf("讀取 scan results 失敗: %w", err)
	}
	var results []ScanResult
	if err := json.Unmarshal(content, &results); err != nil {
		return nil, fmt.Errorf("解析 scan results 失敗: %w", err)
	}
	return results, nil
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

func (m *ScanJobManager) saveResults(jobID string, results []ScanResult) error {
	return writeJSONFile(m.resultsPath(jobID), results)
}

func (m *ScanJobManager) jobPath(id string) string {
	return filepath.Join(m.JobsPath, id+".json")
}

func (m *ScanJobManager) resultsPath(id string) string {
	return filepath.Join(m.ResultsPath, id+".json")
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
