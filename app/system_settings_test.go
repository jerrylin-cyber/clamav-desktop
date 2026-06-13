package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemSettingsServiceOpensFullDiskAccess(t *testing.T) {
	var opened string
	service := &SystemSettingsService{
		OpenURL: func(ctx context.Context, url string) error {
			opened = url
			return nil
		},
	}

	if err := service.OpenFullDiskAccess(context.Background()); err != nil {
		t.Fatalf("open full disk access settings: %v", err)
	}
	if opened != fullDiskAccessSettingsURL {
		t.Fatalf("unexpected settings URL: %s", opened)
	}
}

func TestSystemSettingsServiceOpensNotifications(t *testing.T) {
	var opened string
	service := &SystemSettingsService{
		OpenURL: func(ctx context.Context, url string) error {
			opened = url
			return nil
		},
	}

	if err := service.OpenNotifications(context.Background()); err != nil {
		t.Fatalf("open notification settings: %v", err)
	}
	if opened != notificationSettingsURL {
		t.Fatalf("unexpected settings URL: %s", opened)
	}
}

func TestSystemSettingsServiceReportsFullDiskAccessAuthorized(t *testing.T) {
	homeDir := t.TempDir()
	service := &SystemSettingsService{
		HomeDir: homeDir,
		CheckPath: func(path string, directory bool) error {
			if path != filepath.Join(homeDir, "Library", "Safari", "History.db") {
				t.Fatalf("unexpected probe path: %s", path)
			}
			if directory {
				t.Fatal("first probe should be a file")
			}
			return nil
		},
	}

	status := service.PermissionStatus().FullDiskAccess
	if !status.Authorized || status.Status != "authorized" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestSystemSettingsServiceReportsFullDiskAccessDenied(t *testing.T) {
	service := &SystemSettingsService{
		HomeDir: t.TempDir(),
		CheckPath: func(path string, directory bool) error {
			return os.ErrPermission
		},
	}

	status := service.PermissionStatus().FullDiskAccess
	if status.Authorized || status.Status != "denied" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestSystemSettingsServiceReportsFullDiskAccessUnknownWhenNoProbeExists(t *testing.T) {
	service := &SystemSettingsService{
		HomeDir: t.TempDir(),
		CheckPath: func(path string, directory bool) error {
			return os.ErrNotExist
		},
	}

	status := service.PermissionStatus().FullDiskAccess
	if status.Authorized || status.Status != "unknown" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestIsPermissionDeniedRecognizesOperationNotPermitted(t *testing.T) {
	if !isPermissionDenied(errors.New("open file: operation not permitted")) {
		t.Fatal("expected operation not permitted to be denied")
	}
}
