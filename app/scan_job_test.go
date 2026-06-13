package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanJobManagerRunsScanAndStoresResults(t *testing.T) {
	file := writeScanFixture(t, "clean.txt", "clean")
	manager := testScanJobManager(t, func(_ context.Context, path string) (string, error) {
		if path != file {
			t.Fatalf("unexpected scan path: %s", path)
		}
		return "stream: OK", nil
	})

	var events []ScanProgressEvent
	job, results, err := manager.RunScan(context.Background(), []string{file}, ScanOptions{}, func(event ScanProgressEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("run scan: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("unexpected job status: %s", job.Status)
	}
	if len(results) != 1 || results[0].Status != "clean" {
		t.Fatalf("unexpected results: %#v", results)
	}
	if len(events) == 0 || events[len(events)-1].Status != "completed" {
		t.Fatalf("expected completed progress event, got %#v", events)
	}

	storedJob, err := manager.GetScanJob(job.ID)
	if err != nil {
		t.Fatalf("get scan job: %v", err)
	}
	if storedJob.ID != job.ID || storedJob.Status != "completed" {
		t.Fatalf("stored job mismatch: %#v", storedJob)
	}
	storedResults, err := manager.LoadResults(job.ID)
	if err != nil {
		t.Fatalf("load results: %v", err)
	}
	if len(storedResults) != 1 || storedResults[0].Path != file {
		t.Fatalf("stored results mismatch: %#v", storedResults)
	}
	if job.ScannedFiles != 1 || job.Detections != 0 || job.Errors != 0 {
		t.Fatalf("unexpected job summary counts: %#v", job)
	}
}

func TestScanJobManagerParsesInfectedAndErrorResults(t *testing.T) {
	infected := writeScanFixture(t, "eicar.txt", "infected")
	denied := writeScanFixture(t, "denied.txt", "denied")
	manager := testScanJobManager(t, func(_ context.Context, path string) (string, error) {
		if path == infected {
			return "stream: Eicar-Test-Signature FOUND", nil
		}
		return "", FileReadError{Path: path, Reason: "權限不足，無法讀取掃描檔案", Err: os.ErrPermission}
	})

	job, results, err := manager.RunScan(context.Background(), []string{infected, denied}, ScanOptions{}, nil)
	if err != nil {
		t.Fatalf("run scan: %v", err)
	}
	if job.Status != "completed-with-warnings" {
		t.Fatalf("unexpected job status: %s", job.Status)
	}
	if results[0].Status != "infected" || results[0].Signature != "Eicar-Test-Signature" {
		t.Fatalf("unexpected infected result: %#v", results[0])
	}
	if results[1].Status != "skipped" {
		t.Fatalf("unexpected denied result: %#v", results[1])
	}
	if job.ScannedFiles != 2 || job.Detections != 1 || job.Errors != 1 {
		t.Fatalf("unexpected job summary counts: %#v", job)
	}
}

func TestScanJobManagerScansRecursiveDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	file := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(file, []byte("payload"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	var scanned []string
	manager := testScanJobManager(t, func(_ context.Context, path string) (string, error) {
		scanned = append(scanned, path)
		return "stream: OK", nil
	})

	_, results, err := manager.RunScan(context.Background(), []string{root}, ScanOptions{Recursive: true}, nil)
	if err != nil {
		t.Fatalf("run recursive scan: %v", err)
	}
	if len(scanned) != 1 || scanned[0] != file {
		t.Fatalf("unexpected scanned files: %#v", scanned)
	}
	if len(results) != 1 || results[0].Path != file {
		t.Fatalf("unexpected recursive results: %#v", results)
	}
}

func TestScanJobManagerCanCancelRunningScan(t *testing.T) {
	file := writeScanFixture(t, "slow.txt", "slow")
	started := make(chan string, 1)
	release := make(chan struct{})
	manager := testScanJobManager(t, func(ctx context.Context, path string) (string, error) {
		started <- path
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return "stream: OK", nil
		}
	})

	done := make(chan ScanJob, 1)
	go func() {
		job, _, _ := manager.RunScan(context.Background(), []string{file}, ScanOptions{}, nil)
		done <- job
	}()
	<-started
	if !manager.CancelScanJob("scan_test") {
		t.Fatal("expected cancel to find running job")
	}
	job := <-done
	close(release)

	if job.Status != "canceled" {
		t.Fatalf("unexpected canceled job status: %s", job.Status)
	}
}

func TestScanJobManagerKeepsUserStoresSeparate(t *testing.T) {
	homeA := t.TempDir()
	homeB := t.TempDir()
	managerA := newScanJobManager(homeA, ClamDClient{})
	managerB := newScanJobManager(homeB, ClamDClient{})

	if managerA.JobsPath == managerB.JobsPath || managerA.ResultsPath == managerB.ResultsPath {
		t.Fatalf("expected per-user stores, got %s and %s", managerA.JobsPath, managerB.JobsPath)
	}
}

func TestListScanJobsReturnsSortedJobs(t *testing.T) {
	manager := testScanJobManager(t, nil)

	older := ScanJob{ID: "scan_old", Status: "completed", StartedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	newer := ScanJob{ID: "scan_new", Status: "completed", StartedAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)}
	if err := manager.saveJob(older); err != nil {
		t.Fatalf("save older job: %v", err)
	}
	if err := manager.saveJob(newer); err != nil {
		t.Fatalf("save newer job: %v", err)
	}

	jobs, err := manager.ListScanJobs()
	if err != nil {
		t.Fatalf("list scan jobs: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ID != "scan_new" || jobs[1].ID != "scan_old" {
		t.Fatalf("expected newest first, got %#v", jobs)
	}
}

func TestListScanJobsReturnsEmptyWhenMissing(t *testing.T) {
	manager := testScanJobManager(t, nil)

	jobs, err := manager.ListScanJobs()
	if err != nil {
		t.Fatalf("list scan jobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected empty, got %#v", jobs)
	}
}

func TestAppScanMethodsUseSameManager(t *testing.T) {
	file := writeScanFixture(t, "app-scan.txt", "clean")
	manager := testScanJobManager(t, func(_ context.Context, _ string) (string, error) {
		return "stream: OK", nil
	})
	app := &App{scanJobManager: manager, logService: testLogService(t)}

	job, err := app.StartScan([]string{file}, ScanOptions{})
	if err != nil {
		t.Fatalf("start scan: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("unexpected app scan status: %s", job.Status)
	}

	results, err := app.LoadScanResults(job.ID)
	if err != nil {
		t.Fatalf("load app scan results: %v", err)
	}
	if len(results) != 1 || results[0].Status != "clean" {
		t.Fatalf("unexpected app scan results: %#v", results)
	}
	if app.CancelScanJob(job.ID) {
		t.Fatal("completed scan should not be cancelable")
	}

	jobs, err := app.ListScanJobs()
	if err != nil {
		t.Fatalf("app list scan jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatalf("unexpected app scan jobs: %#v", jobs)
	}

	entries, err := app.ListAppLogEntries(0)
	if err != nil {
		t.Fatalf("app list log entries: %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Message, "掃描完成") {
		t.Fatalf("expected scan completion log entry, got %#v", entries)
	}
}

func TestScanResultFromReplyParsesClamdReplies(t *testing.T) {
	cases := []struct {
		reply     string
		err       error
		status    string
		signature string
	}{
		{reply: "stream: OK", status: "clean"},
		{reply: "/tmp/eicar.txt: Eicar-Test-Signature FOUND", status: "infected", signature: "Eicar-Test-Signature"},
		{reply: "/tmp/a.txt: ERROR: Permission denied", status: "error"},
		{err: errors.New("boom"), status: "error"},
	}

	for _, tc := range cases {
		result := scanResultFromReply("/tmp/a.txt", tc.reply, tc.err)
		if result.Status != tc.status {
			t.Fatalf("reply %q expected %s, got %#v", tc.reply, tc.status, result)
		}
		if tc.signature != "" && result.Signature != tc.signature {
			t.Fatalf("expected signature %q, got %q", tc.signature, result.Signature)
		}
	}
}

func testScanJobManager(t *testing.T, scan scanFileFunc) *ScanJobManager {
	t.Helper()
	base := t.TempDir()
	return &ScanJobManager{
		JobsPath:    filepath.Join(base, "jobs"),
		ResultsPath: filepath.Join(base, "results"),
		scanFile:    scan,
		newID: func() string {
			return "scan_test"
		},
		now: func() time.Time {
			return time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
		},
	}
}

func writeScanFixture(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write scan fixture: %v", err)
	}
	return path
}
