package main

import (
	"context"
	"path/filepath"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestBeforeCloseCancelPreventsClose(t *testing.T) {
	app := appWithCloseSettings(t, false)
	app.messageDialog = func(_ context.Context, options wailsruntime.MessageDialogOptions) (string, error) {
		if options.Title != "是否關閉" {
			t.Fatalf("unexpected dialog title: %q", options.Title)
		}
		return "取消", nil
	}

	if !app.beforeClose(context.Background()) {
		t.Fatal("cancel should prevent close")
	}
}

func TestBeforeCloseConfirmWithoutBackgroundAllowsQuit(t *testing.T) {
	app := appWithCloseSettings(t, false)
	app.messageDialog = func(context.Context, wailsruntime.MessageDialogOptions) (string, error) {
		return "關閉", nil
	}

	if app.beforeClose(context.Background()) {
		t.Fatal("confirmed close without menu bar icon should quit")
	}
}

func TestBeforeCloseConfirmWithBackgroundShowsNoticeAndHidesWindow(t *testing.T) {
	app := appWithCloseSettings(t, true)
	var titles []string
	var statusItemStarted bool
	var hidden bool
	app.messageDialog = func(_ context.Context, options wailsruntime.MessageDialogOptions) (string, error) {
		titles = append(titles, options.Title)
		if len(titles) == 1 {
			return "關閉", nil
		}
		return "知道了", nil
	}
	app.startStatusItemRun = func() {
		statusItemStarted = true
	}
	app.hideWindow = func(context.Context) {
		hidden = true
	}

	if !app.beforeClose(context.Background()) {
		t.Fatal("background close should keep app running")
	}
	if !statusItemStarted {
		t.Fatal("status item should be started before hiding")
	}
	if !hidden {
		t.Fatal("window should be hidden")
	}
	if len(titles) != 2 || titles[0] != "是否關閉" || titles[1] != "仍然會在背景運作" {
		t.Fatalf("unexpected dialogs: %#v", titles)
	}
}

func appWithCloseSettings(t *testing.T, keepMenuBarIcon bool) *App {
	t.Helper()
	store := SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}
	settings := defaultSettings()
	settings.Background.KeepMenuBarIcon = keepMenuBarIcon
	settings.Background.StartHidden = false
	if err := store.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	return &App{settingsStore: store}
}
