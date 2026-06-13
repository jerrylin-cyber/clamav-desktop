package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	fullDiskAccessSettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
	notificationSettingsURL   = "x-apple.systempreferences:com.apple.Notifications-Settings.extension"
)

type systemSettingsRunner func(context.Context, string) error
type systemPermissionChecker func(string, bool) error

// SystemPermissionCheck 為單一 macOS 權限的檢測結果（是否授權、狀態碼與說明）。
type SystemPermissionCheck struct {
	Authorized bool   `json:"authorized"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

// SystemPermissionStatus 彙整 App 關注的 macOS 權限狀態，序列化給前端顯示。
type SystemPermissionStatus struct {
	FullDiskAccess SystemPermissionCheck `json:"fullDiskAccess"`
}

// SystemSettingsService 負責開啟 macOS 系統設定的特定面板，並檢測權限狀態；欄位可在測試中替換以注入假行為。
type SystemSettingsService struct {
	OpenURL   systemSettingsRunner
	HomeDir   string
	CheckPath systemPermissionChecker
}

func newSystemSettingsService() *SystemSettingsService {
	return &SystemSettingsService{OpenURL: openSystemSettingsURL}
}

// OpenFullDiskAccess 開啟「系統設定 → 隱私權與安全性 → 完整磁碟取用權限」面板。
func (s *SystemSettingsService) OpenFullDiskAccess(ctx context.Context) error {
	return s.open(ctx, fullDiskAccessSettingsURL)
}

// OpenNotifications 開啟「系統設定 → 通知」面板。
func (s *SystemSettingsService) OpenNotifications(ctx context.Context) error {
	return s.open(ctx, notificationSettingsURL)
}

// PermissionStatus 回傳目前 App 關注的 macOS 權限檢測結果。
func (s *SystemSettingsService) PermissionStatus() SystemPermissionStatus {
	return SystemPermissionStatus{
		FullDiskAccess: s.fullDiskAccessStatus(),
	}
}

func (s *SystemSettingsService) open(ctx context.Context, url string) error {
	runner := s.OpenURL
	if runner == nil {
		runner = openSystemSettingsURL
	}
	if err := runner(ctx, url); err != nil {
		return fmt.Errorf("開啟 System Settings 失敗: %w", err)
	}
	return nil
}

func openSystemSettingsURL(ctx context.Context, url string) error {
	return exec.CommandContext(ctx, "/usr/bin/open", url).Run()
}

func (s *SystemSettingsService) fullDiskAccessStatus() SystemPermissionCheck {
	homeDir := s.HomeDir
	if strings.TrimSpace(homeDir) == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil || strings.TrimSpace(homeDir) == "" {
			return SystemPermissionCheck{Status: "unknown", Message: "無法取得使用者 home 目錄"}
		}
	}

	checkPath := s.CheckPath
	if checkPath == nil {
		checkPath = checkSystemPermissionPath
	}

	checkedExistingPath := false
	for _, probe := range fullDiskAccessProbes(homeDir) {
		if err := checkPath(probe.path, probe.directory); err == nil {
			return SystemPermissionCheck{Authorized: true, Status: "authorized", Message: "可讀取 macOS 受保護資料"}
		} else if errors.Is(err, os.ErrNotExist) {
			continue
		} else if isPermissionDenied(err) {
			return SystemPermissionCheck{Status: "denied", Message: "macOS 拒絕讀取受保護資料"}
		} else {
			checkedExistingPath = true
		}
	}

	if checkedExistingPath {
		return SystemPermissionCheck{Status: "unknown", Message: "受保護資料存在，但目前無法判定權限狀態"}
	}
	return SystemPermissionCheck{Status: "unknown", Message: "找不到可用來檢查的受保護資料"}
}

type fullDiskAccessProbe struct {
	path      string
	directory bool
}

func fullDiskAccessProbes(homeDir string) []fullDiskAccessProbe {
	return []fullDiskAccessProbe{
		{path: filepath.Join(homeDir, "Library", "Safari", "History.db")},
		{path: filepath.Join(homeDir, "Library", "Safari", "CloudTabs.db")},
		{path: filepath.Join(homeDir, "Library", "Messages", "chat.db")},
		{path: filepath.Join(homeDir, "Library", "Mail"), directory: true},
	}
}

func checkSystemPermissionPath(path string, directory bool) error {
	if directory {
		_, err := os.ReadDir(path)
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var oneByte [1]byte
	if _, err := file.Read(oneByte[:]); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func isPermissionDenied(err error) bool {
	if os.IsPermission(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "operation not permitted") || strings.Contains(message, "permission denied")
}
