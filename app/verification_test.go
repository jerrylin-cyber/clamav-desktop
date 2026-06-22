package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestVerifyPaginationScalesToLargeDataset 實證問題 1/2 的核心：結果庫存入大量資料後，
// 取單一分頁的延遲與資料量無關（只回傳 25 筆），不會把整批資料載入前端。
func TestVerifyPaginationScalesToLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("skip large-data verification in -short mode")
	}
	store := newTestResultsStore(t)

	const total = 700000
	const batch = 5000
	buf := make([]ScanResult, 0, batch)
	insertStart := time.Now()
	for i := 0; i < total; i++ {
		status := "clean"
		sig := ""
		if i%10000 == 0 {
			status = "infected"
			sig = fmt.Sprintf("Test-Sig-%d", i)
		}
		buf = append(buf, ScanResult{
			Path:      fmt.Sprintf("/Users/test/Dropbox/file_%07d.dat", i),
			Status:    status,
			Signature: sig,
			Engine:    "clamd",
		})
		if len(buf) == batch {
			if err := store.AppendResults("bigjob", buf); err != nil {
				t.Fatalf("append batch: %v", err)
			}
			buf = buf[:0]
		}
	}
	if err := store.AppendResults("bigjob", buf); err != nil {
		t.Fatalf("append tail: %v", err)
	}
	t.Logf("插入 %d 筆耗時 %s", total, time.Since(insertStart))

	// 取中段的一頁，計時。延遲應遠小於把 70 萬筆載入記憶體的成本。
	queryStart := time.Now()
	page, err := store.QueryPage("bigjob", "all", "", 350000, 25)
	if err != nil {
		t.Fatalf("query mid page: %v", err)
	}
	elapsed := time.Since(queryStart)
	t.Logf("查詢第 350000 筆起的 25 筆 + 計數耗時 %s", elapsed)

	if len(page.Items) != 25 {
		t.Fatalf("expected 25 items per page, got %d", len(page.Items))
	}
	if page.Total != total {
		t.Fatalf("expected total %d, got %d", total, page.Total)
	}
	if page.Counts["all"] != total || page.Counts["infected"] != total/10000 {
		t.Fatalf("unexpected counts: %#v", page.Counts)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("single page query too slow (%s) for %d rows — pagination not effective", elapsed, total)
	}

	// 篩選 + 搜尋同樣只回傳當頁，不需把全部資料拉到前端。
	infectedPage, err := store.QueryPage("bigjob", "infected", "Test-Sig-0", 0, 25)
	if err != nil {
		t.Fatalf("query infected page: %v", err)
	}
	if infectedPage.Total == 0 || len(infectedPage.Items) == 0 {
		t.Fatalf("expected infected search hits, got total=%d", infectedPage.Total)
	}
}

// TestVerifyScanInterruptionPersistsPartialResults 實證問題 3：掃描中途被取消（等同 app 關閉時 context 取消）時，
// 已掃描的部分結果已 durable 寫入結果庫、不會遺失，且工作可被標記為可重掃狀態。
func TestVerifyScanInterruptionPersistsPartialResults(t *testing.T) {
	root := t.TempDir()
	const fileCount = 600
	for i := 0; i < fileCount; i++ {
		name := filepath.Join(root, fmt.Sprintf("f_%04d.txt", i))
		if err := os.WriteFile(name, []byte("payload"), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	scanned := make(chan struct{}, fileCount)
	manager := testScanJobManager(t, func(ctx context.Context, _ string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		scanned <- struct{}{}
		return "stream: OK", nil
	})

	done := make(chan ScanJob, 1)
	go func() {
		job, _, _ := manager.RunScan(context.Background(), []string{root}, ScanOptions{Recursive: true}, nil)
		done <- job
	}()

	// 等掃了一些檔案（超過一個寫入批次 200 筆）後再取消，模擬 app 在掃描中途關閉。
	for i := 0; i < scanResultBatchSize+50; i++ {
		<-scanned
	}
	if !manager.CancelScanJob("scan_test") {
		t.Fatal("expected to cancel running scan")
	}
	job := <-done

	if job.Status != "canceled" {
		t.Fatalf("expected canceled status, got %s", job.Status)
	}

	page, err := manager.LoadResultsPage(job.ID, "all", "", 0, 25)
	if err != nil {
		t.Fatalf("load page after cancel: %v", err)
	}
	// 取消前已 flush 的批次（至少一批 200 筆）應已持久化，證明結果不會整批遺失。
	if page.Total < scanResultBatchSize {
		t.Fatalf("expected at least %d persisted partial results after interruption, got %d", scanResultBatchSize, page.Total)
	}
	t.Logf("中斷後已持久化 %d 筆部分結果（未遺失）", page.Total)
}
