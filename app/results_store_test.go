package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTestResultsStore(t *testing.T) *ResultsStore {
	t.Helper()
	return newResultsStore(t.TempDir())
}

func sampleResults() []ScanResult {
	return []ScanResult{
		{Path: "/a/clean1.txt", Status: "clean", Engine: "clamd"},
		{Path: "/a/eicar.txt", Status: "infected", Signature: "Eicar-Test-Signature", Engine: "clamd"},
		{Path: "/a/clean2.txt", Status: "clean", Engine: "clamd"},
		{Path: "/a/denied.bin", Status: "skipped", Error: "權限不足"},
		{Path: "/a/broken.dat", Status: "error", Error: "讀取失敗"},
	}
}

func TestResultsStoreQueryPagePaginatesAndCounts(t *testing.T) {
	store := newTestResultsStore(t)
	if err := store.AppendResults("job1", sampleResults()); err != nil {
		t.Fatalf("append results: %v", err)
	}

	page, err := store.QueryPage("job1", "all", "", 0, 2)
	if err != nil {
		t.Fatalf("query page: %v", err)
	}
	if page.Total != 5 {
		t.Fatalf("expected total 5, got %d", page.Total)
	}
	if len(page.Items) != 2 || page.Items[0].Path != "/a/clean1.txt" || page.Items[1].Path != "/a/eicar.txt" {
		t.Fatalf("unexpected page 1 items: %#v", page.Items)
	}
	if page.Counts["all"] != 5 || page.Counts["clean"] != 2 || page.Counts["infected"] != 1 || page.Counts["skipped"] != 1 || page.Counts["error"] != 1 {
		t.Fatalf("unexpected counts: %#v", page.Counts)
	}

	second, err := store.QueryPage("job1", "all", "", 2, 2)
	if err != nil {
		t.Fatalf("query page 2: %v", err)
	}
	if len(second.Items) != 2 || second.Items[0].Path != "/a/clean2.txt" {
		t.Fatalf("unexpected page 2 items: %#v", second.Items)
	}
}

func TestResultsStoreQueryPageFiltersByStatus(t *testing.T) {
	store := newTestResultsStore(t)
	if err := store.AppendResults("job1", sampleResults()); err != nil {
		t.Fatalf("append results: %v", err)
	}

	page, err := store.QueryPage("job1", "infected", "", 0, 25)
	if err != nil {
		t.Fatalf("query infected: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Status != "infected" {
		t.Fatalf("unexpected infected page: total=%d items=%#v", page.Total, page.Items)
	}
	// Counts 仍反映全部狀態，供篩選列顯示數字。
	if page.Counts["clean"] != 2 {
		t.Fatalf("expected clean count 2 in counts, got %#v", page.Counts)
	}
}

func TestResultsStoreQueryPageSearchesPathAndSignature(t *testing.T) {
	store := newTestResultsStore(t)
	if err := store.AppendResults("job1", sampleResults()); err != nil {
		t.Fatalf("append results: %v", err)
	}

	bySignature, err := store.QueryPage("job1", "all", "eicar-test", 0, 25)
	if err != nil {
		t.Fatalf("query signature: %v", err)
	}
	if bySignature.Total != 1 || bySignature.Items[0].Signature != "Eicar-Test-Signature" {
		t.Fatalf("unexpected signature search: total=%d items=%#v", bySignature.Total, bySignature.Items)
	}

	byPath, err := store.QueryPage("job1", "all", "clean", 0, 25)
	if err != nil {
		t.Fatalf("query path: %v", err)
	}
	if byPath.Total != 2 {
		t.Fatalf("expected 2 path matches, got %d", byPath.Total)
	}
}

func TestResultsStoreUpdateStatus(t *testing.T) {
	store := newTestResultsStore(t)
	if err := store.AppendResults("job1", sampleResults()); err != nil {
		t.Fatalf("append results: %v", err)
	}
	if err := store.UpdateStatus("job1", "/a/eicar.txt", "quarantined"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	page, err := store.QueryPage("job1", "all", "eicar", 0, 25)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if page.Items[0].Status != "quarantined" {
		t.Fatalf("expected quarantined status, got %s", page.Items[0].Status)
	}
}

func TestResultsStoreDeleteAll(t *testing.T) {
	store := newTestResultsStore(t)
	if err := store.AppendResults("job1", sampleResults()); err != nil {
		t.Fatalf("append results: %v", err)
	}
	if err := store.DeleteAll(); err != nil {
		t.Fatalf("delete all: %v", err)
	}
	page, err := store.QueryPage("job1", "all", "", 0, 25)
	if err != nil {
		t.Fatalf("query after delete: %v", err)
	}
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("expected empty after delete, got total=%d", page.Total)
	}
}

func TestResultsStoreLazyImportsLegacyJSON(t *testing.T) {
	base := t.TempDir()
	store := newResultsStore(base)

	legacyDir := filepath.Join(base, "results")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	content, err := json.Marshal(sampleResults())
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy1.json"), content, 0600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	page, err := store.QueryPage("legacy1", "all", "", 0, 25)
	if err != nil {
		t.Fatalf("query legacy: %v", err)
	}
	if page.Total != 5 {
		t.Fatalf("expected 5 imported rows, got %d", page.Total)
	}
}
