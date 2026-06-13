package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundlePathForExecutable(t *testing.T) {
	executable := "/Applications/ClamAV Desktop.app/Contents/MacOS/clamav-desktop"
	path, ok := bundlePathForExecutable(executable)
	if !ok {
		t.Fatal("expected executable in app bundle")
	}
	if path != "/Applications/ClamAV Desktop.app" {
		t.Fatalf("unexpected bundle path: %s", path)
	}
}

func TestLoginItemRegisterWritesLaunchAgent(t *testing.T) {
	homeDir := t.TempDir()
	service := &LoginItemService{
		HomeDir:    homeDir,
		BundlePath: "/Applications/ClamAV Desktop.app",
	}

	if err := service.Register(true); err != nil {
		t.Fatalf("register login item: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(homeDir, "Library/LaunchAgents/"+loginAgentLabel+".plist"))
	if err != nil {
		t.Fatalf("read login item plist: %v", err)
	}
	plist := string(content)
	for _, want := range []string{
		"<string>/usr/bin/open</string>",
		"<string>-g</string>",
		"<string>-j</string>",
		"<string>/Applications/ClamAV Desktop.app</string>",
		"<true/>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}

	status := service.Status()
	if !status.Enabled || status.Error != "" {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestLoginItemUnregisterRemovesLaunchAgent(t *testing.T) {
	homeDir := t.TempDir()
	service := &LoginItemService{
		HomeDir:    homeDir,
		BundlePath: "/Applications/ClamAV Desktop.app",
	}
	if err := service.Register(false); err != nil {
		t.Fatalf("register login item: %v", err)
	}
	if err := service.Unregister(); err != nil {
		t.Fatalf("unregister login item: %v", err)
	}
	if service.Status().Enabled {
		t.Fatal("expected login item to be disabled")
	}
}

func TestAppSaveSettingsAppliesLoginItemChange(t *testing.T) {
	homeDir := t.TempDir()
	app := &App{
		settingsStore: SettingsStore{Path: filepath.Join(homeDir, "Library/Application Support/ClamAVDesktop/settings.json")},
		loginItemService: &LoginItemService{
			HomeDir:    homeDir,
			BundlePath: "/Applications/ClamAV Desktop.app",
		},
	}

	settings := defaultSettings()
	settings.Login.LaunchAtLogin = true
	settings.Background.StartHidden = true

	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if !app.GetLoginItemStatus().Enabled {
		t.Fatal("expected login item to be registered")
	}
}
