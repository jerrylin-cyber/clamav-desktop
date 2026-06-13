package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const currentSettingsSchemaVersion = 1

var errUnsupportedSettingsVersion = errors.New("settings schema version 不支援")

type Settings struct {
	SchemaVersion  int            `json:"schemaVersion"`
	RuntimeMode    string         `json:"runtimeMode"`
	ScanSchedule   ScanSchedule   `json:"scanSchedule"`
	UpdateSchedule UpdateSchedule `json:"updateSchedule"`
	PowerPolicy    PowerPolicy    `json:"powerPolicy"`
	Background     Background     `json:"background"`
	Login          LoginSettings  `json:"login"`
	Actions        ActionSettings `json:"actions"`
}

type ScanSchedule struct {
	Enabled   bool     `json:"enabled"`
	Frequency string   `json:"frequency"`
	TimeOfDay string   `json:"timeOfDay"`
	Weekday   int      `json:"weekday"`
	Paths     []string `json:"paths"`
}

type UpdateSchedule struct {
	Enabled   bool   `json:"enabled"`
	Frequency string `json:"frequency"`
	TimeOfDay string `json:"timeOfDay"`
}

type PowerPolicy struct {
	RunOnBattery       bool `json:"runOnBattery"`
	RunInLowPowerMode  bool `json:"runInLowPowerMode"`
	DeferUntilCharging bool `json:"deferUntilCharging"`
}

type Background struct {
	Enabled         bool `json:"enabled"`
	StartHidden     bool `json:"startHidden"`
	KeepMenuBarIcon bool `json:"keepMenuBarIcon"`
}

type LoginSettings struct {
	LaunchAtLogin bool `json:"launchAtLogin"`
}

type ActionSettings struct {
	ConfirmPermanentDelete bool `json:"confirmPermanentDelete"`
}

type SettingsStore struct {
	Path    string
	Migrate func(settings *Settings) error
}

func defaultSettingsStore() SettingsStore {
	homeDir, _ := os.UserHomeDir()
	return SettingsStore{Path: userSettingsPath(homeDir)}
}

func userSettingsPath(homeDir string) string {
	return filepath.Join(homeDir, "Library/Application Support/ClamAVDesktop/settings.json")
}

func defaultSettings() Settings {
	return Settings{
		SchemaVersion: currentSettingsSchemaVersion,
		RuntimeMode:   "system-shared",
		ScanSchedule: ScanSchedule{
			Enabled:   false,
			Frequency: "daily",
			TimeOfDay: "12:00",
			Weekday:   1,
			Paths:     []string{},
		},
		UpdateSchedule: UpdateSchedule{
			Enabled:   true,
			Frequency: "daily",
			TimeOfDay: "03:00",
		},
		PowerPolicy: PowerPolicy{
			RunOnBattery:       false,
			RunInLowPowerMode:  false,
			DeferUntilCharging: true,
		},
		Background: Background{
			Enabled:         true,
			StartHidden:     false,
			KeepMenuBarIcon: true,
		},
		Login: LoginSettings{
			LaunchAtLogin: false,
		},
		Actions: ActionSettings{
			ConfirmPermanentDelete: true,
		},
	}
}

func (s SettingsStore) Load() (Settings, error) {
	if s.Path == "" {
		return Settings{}, errors.New("settings path 不可為空")
	}

	content, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultSettings(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("讀取 settings 失敗: %w", err)
	}

	settings := defaultSettings()
	if err := json.Unmarshal(content, &settings); err != nil {
		return Settings{}, fmt.Errorf("解析 settings 失敗: %w", err)
	}
	if err := s.normalize(&settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s SettingsStore) Save(settings Settings) error {
	if s.Path == "" {
		return errors.New("settings path 不可為空")
	}
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = currentSettingsSchemaVersion
	}
	if err := s.normalize(&settings); err != nil {
		return err
	}

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 settings 失敗: %w", err)
	}
	content = append(content, '\n')

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("建立 settings 目錄失敗: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("建立 settings 暫存檔失敗: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("寫入 settings 暫存檔失敗: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("同步 settings 暫存檔失敗: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("關閉 settings 暫存檔失敗: %w", err)
	}
	if err := os.Chmod(tempPath, 0600); err != nil {
		return fmt.Errorf("設定 settings 權限失敗: %w", err)
	}
	if err := os.Rename(tempPath, s.Path); err != nil {
		return fmt.Errorf("替換 settings 失敗: %w", err)
	}
	cleanup = false
	return nil
}

func (s SettingsStore) normalize(settings *Settings) error {
	if settings.SchemaVersion > currentSettingsSchemaVersion {
		return fmt.Errorf("%w: %d", errUnsupportedSettingsVersion, settings.SchemaVersion)
	}
	if settings.SchemaVersion < currentSettingsSchemaVersion {
		if s.Migrate == nil {
			return fmt.Errorf("%w: %d", errUnsupportedSettingsVersion, settings.SchemaVersion)
		}
		if err := s.Migrate(settings); err != nil {
			return fmt.Errorf("migrate settings 失敗: %w", err)
		}
	}

	defaults := defaultSettings()
	if settings.RuntimeMode == "" {
		settings.RuntimeMode = defaults.RuntimeMode
	}
	if settings.ScanSchedule.Frequency == "" {
		settings.ScanSchedule.Frequency = defaults.ScanSchedule.Frequency
	}
	if settings.ScanSchedule.TimeOfDay == "" {
		settings.ScanSchedule.TimeOfDay = defaults.ScanSchedule.TimeOfDay
	}
	if settings.ScanSchedule.Paths == nil {
		settings.ScanSchedule.Paths = []string{}
	}
	if settings.UpdateSchedule.Frequency == "" {
		settings.UpdateSchedule.Frequency = defaults.UpdateSchedule.Frequency
	}
	if settings.UpdateSchedule.TimeOfDay == "" {
		settings.UpdateSchedule.TimeOfDay = defaults.UpdateSchedule.TimeOfDay
	}
	// 「啟動時隱藏視窗」必須搭配「保留狀態列圖示」，否則啟動後沒有任何入口可開啟視窗
	if !settings.Background.KeepMenuBarIcon {
		settings.Background.StartHidden = false
	}
	return nil
}
