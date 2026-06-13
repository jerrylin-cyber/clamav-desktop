package main

import (
	"context"
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
