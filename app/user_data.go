package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UserDataRemovalOptions 指定要移除哪幾類使用者資料；各旗標獨立，可組合勾選。
type UserDataRemovalOptions struct {
	RemoveSettings    bool `json:"removeSettings"`
	RemoveScanJobs    bool `json:"removeScanJobs"`
	RemoveScanResults bool `json:"removeScanResults"`
	RemoveQuarantine  bool `json:"removeQuarantine"`
	RemoveLogs        bool `json:"removeLogs"`
}

// UserDataRemovalResult 回報實際移除與略過（不存在）的路徑，供前端顯示結果。
type UserDataRemovalResult struct {
	Removed []string `json:"removed"`
	Skipped []string `json:"skipped"`
}

type userDataPaths struct {
	Base       string
	Settings   string
	Jobs       string
	Results    string
	Quarantine string
	Logs       string
}

// RemoveUserData 依選項移除目前使用者的 ClamAV Desktop 資料（設定、掃描工作、結果、隔離區、紀錄）。
// 作為 Wails binding 供前端呼叫；移除前會驗證目標路徑確實位於 App 的資料目錄內，避免誤刪其他位置。
func (a *App) RemoveUserData(options UserDataRemovalOptions) (UserDataRemovalResult, error) {
	homeDir, _ := os.UserHomeDir()
	return removeUserData(userDataPathsForHome(homeDir), options)
}

func userDataPathsForHome(homeDir string) userDataPaths {
	base := filepath.Join(homeDir, "Library/Application Support/ClamAVDesktop")
	return userDataPaths{
		Base:       base,
		Settings:   filepath.Join(base, "settings.json"),
		Jobs:       filepath.Join(base, "jobs"),
		Results:    filepath.Join(base, "results"),
		Quarantine: filepath.Join(base, "quarantine"),
		Logs:       filepath.Join(homeDir, "Library/Logs/ClamAVDesktop"),
	}
}

func removeUserData(paths userDataPaths, options UserDataRemovalOptions) (UserDataRemovalResult, error) {
	targets := selectedUserDataTargets(paths, options)
	if len(targets) == 0 {
		return UserDataRemovalResult{}, errors.New("至少選擇一種要移除的使用者資料")
	}

	result := UserDataRemovalResult{Removed: []string{}, Skipped: []string{}}
	for _, target := range targets {
		if err := ensureSafeUserDataTarget(paths, target); err != nil {
			return result, err
		}
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			result.Skipped = append(result.Skipped, target)
			continue
		} else if err != nil {
			return result, fmt.Errorf("檢查使用者資料失敗: %w", err)
		}

		if err := os.RemoveAll(target); err != nil {
			return result, fmt.Errorf("移除使用者資料失敗: %w", err)
		}
		result.Removed = append(result.Removed, target)
	}
	return result, nil
}

func selectedUserDataTargets(paths userDataPaths, options UserDataRemovalOptions) []string {
	var targets []string
	if options.RemoveSettings {
		targets = append(targets, paths.Settings)
	}
	if options.RemoveScanJobs {
		targets = append(targets, paths.Jobs)
	}
	if options.RemoveScanResults {
		targets = append(targets, paths.Results)
	}
	if options.RemoveQuarantine {
		targets = append(targets, paths.Quarantine)
	}
	if options.RemoveLogs {
		targets = append(targets, paths.Logs)
	}
	return targets
}

func ensureSafeUserDataTarget(paths userDataPaths, target string) error {
	cleanTarget := filepath.Clean(target)
	cleanBase := filepath.Clean(paths.Base)
	cleanLogs := filepath.Clean(paths.Logs)

	if cleanTarget == cleanLogs {
		return nil
	}
	if cleanTarget == cleanBase || !strings.HasPrefix(cleanTarget, cleanBase+string(os.PathSeparator)) {
		return fmt.Errorf("拒絕移除非 ClamAV Desktop 使用者資料路徑: %s", target)
	}
	return nil
}
