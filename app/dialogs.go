package main

import (
	"context"
	"fmt"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type filesDialogRunner func(ctx context.Context, options wailsruntime.OpenDialogOptions) ([]string, error)

type folderDialogRunner func(ctx context.Context, options wailsruntime.OpenDialogOptions) (string, error)

// SelectScanFiles opens a native file picker allowing multiple selection and
// returns the chosen paths. Returns an empty slice if the user cancels.
func (a *App) SelectScanFiles() ([]string, error) {
	run := a.selectFilesDialog
	if run == nil {
		run = wailsruntime.OpenMultipleFilesDialog
	}

	paths, err := run(a.context(), wailsruntime.OpenDialogOptions{Title: "選擇要掃描的檔案"})
	if err != nil {
		return nil, fmt.Errorf("選擇檔案失敗: %w", err)
	}
	return paths, nil
}

// SelectScanFolder opens a native folder picker and returns the chosen path.
// Returns an empty string if the user cancels.
func (a *App) SelectScanFolder() (string, error) {
	run := a.selectFolderDialog
	if run == nil {
		run = wailsruntime.OpenDirectoryDialog
	}

	path, err := run(a.context(), wailsruntime.OpenDialogOptions{Title: "選擇要掃描的資料夾"})
	if err != nil {
		return "", fmt.Errorf("選擇資料夾失敗: %w", err)
	}
	return path, nil
}
