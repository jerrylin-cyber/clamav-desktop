package main

import (
	"context"
	"reflect"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestSelectScanFilesReturnsChosenPaths(t *testing.T) {
	app := &App{
		selectFilesDialog: func(_ context.Context, _ wailsruntime.OpenDialogOptions) ([]string, error) {
			return []string{"/Users/test/a.txt", "/Users/test/b.txt"}, nil
		},
	}

	paths, err := app.SelectScanFiles()
	if err != nil {
		t.Fatalf("select scan files: %v", err)
	}
	if !reflect.DeepEqual(paths, []string{"/Users/test/a.txt", "/Users/test/b.txt"}) {
		t.Fatalf("unexpected paths: %#v", paths)
	}
}

func TestSelectScanFilesReturnsEmptyWhenCanceled(t *testing.T) {
	app := &App{
		selectFilesDialog: func(_ context.Context, _ wailsruntime.OpenDialogOptions) ([]string, error) {
			return nil, nil
		},
	}

	paths, err := app.SelectScanFiles()
	if err != nil {
		t.Fatalf("select scan files: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no paths, got %#v", paths)
	}
}

func TestSelectScanFolderReturnsChosenPath(t *testing.T) {
	app := &App{
		selectFolderDialog: func(_ context.Context, _ wailsruntime.OpenDialogOptions) (string, error) {
			return "/Users/test/Documents", nil
		},
	}

	path, err := app.SelectScanFolder()
	if err != nil {
		t.Fatalf("select scan folder: %v", err)
	}
	if path != "/Users/test/Documents" {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestSelectScanFolderReturnsEmptyWhenCanceled(t *testing.T) {
	app := &App{
		selectFolderDialog: func(_ context.Context, _ wailsruntime.OpenDialogOptions) (string, error) {
			return "", nil
		},
	}

	path, err := app.SelectScanFolder()
	if err != nil {
		t.Fatalf("select scan folder: %v", err)
	}
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
}
