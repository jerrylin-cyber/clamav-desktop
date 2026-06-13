package main

import (
	"context"
	"fmt"
	"os/exec"
)

const (
	fullDiskAccessSettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
	notificationSettingsURL   = "x-apple.systempreferences:com.apple.Notifications-Settings.extension"
)

type systemSettingsRunner func(context.Context, string) error

type SystemSettingsService struct {
	OpenURL systemSettingsRunner
}

func newSystemSettingsService() *SystemSettingsService {
	return &SystemSettingsService{OpenURL: openSystemSettingsURL}
}

func (s *SystemSettingsService) OpenFullDiskAccess(ctx context.Context) error {
	return s.open(ctx, fullDiskAccessSettingsURL)
}

func (s *SystemSettingsService) OpenNotifications(ctx context.Context) error {
	return s.open(ctx, notificationSettingsURL)
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
