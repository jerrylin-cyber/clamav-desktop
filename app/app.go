package main

import (
	"context"
	"os"
	"path/filepath"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type RuntimeProfile struct {
	Mode          string   `json:"mode"`
	ClamScanPath  string   `json:"clamScanPath"`
	FreshclamPath string   `json:"freshclamPath"`
	ClamdPath     string   `json:"clamdPath"`
	ClamdSocket   string   `json:"clamdSocket"`
	DatabasePath  string   `json:"databasePath"`
	ConfigPath    string   `json:"configPath"`
	Source        string   `json:"source"`
	Warnings      []string `json:"warnings"`
}

type AppStatus struct {
	Runtime RuntimeProfile `json:"runtime"`
	Pages   []string       `json:"pages"`
}

func (a *App) GetAppStatus() AppStatus {
	return AppStatus{
		Runtime: runtimeProfile(),
		Pages: []string{
			"儀表板",
			"掃描",
			"結果",
			"排程",
			"隔離區",
			"設定",
			"紀錄",
			"關於",
		},
	}
}

func runtimeProfile() RuntimeProfile {
	base := "/Library/Application Support/ClamAVDesktop"
	profile := RuntimeProfile{
		Mode:          "missing",
		ClamScanPath:  filepath.Join(base, "Runtime/bin/clamscan"),
		FreshclamPath: filepath.Join(base, "Runtime/bin/freshclam"),
		ClamdPath:     filepath.Join(base, "Runtime/sbin/clamd"),
		ClamdSocket:   filepath.Join(base, "Run/clamd.sock"),
		DatabasePath:  filepath.Join(base, "Database"),
		ConfigPath:    filepath.Join(base, "Config"),
		Source:        "app-managed-system-shared",
		Warnings:      []string{},
	}

	if fileExists(profile.ClamdPath) && fileExists(profile.FreshclamPath) {
		profile.Mode = "system-shared"
		return profile
	}

	profile.Warnings = append(profile.Warnings, "尚未安裝 App-managed ClamAV runtime。")
	return profile
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
