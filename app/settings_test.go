package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsStoreLoadsDefaultsWhenMissing(t *testing.T) {
	store := SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}

	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if settings.SchemaVersion != currentSettingsSchemaVersion {
		t.Fatalf("unexpected schema version: %d", settings.SchemaVersion)
	}
	if !settings.UpdateSchedule.Enabled {
		t.Fatal("expected database update schedule to be enabled by default")
	}
	if !settings.Background.KeepMenuBarIcon {
		t.Fatal("expected menu bar icon to be kept by default")
	}
}

func TestSettingsStoreSavesAndLoadsAtomically(t *testing.T) {
	store := SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}
	settings := defaultSettings()
	settings.ScanSchedule.Enabled = true
	settings.ScanSchedule.TimeOfDay = "22:30"
	settings.Login.LaunchAtLogin = true

	if err := store.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if !loaded.ScanSchedule.Enabled || loaded.ScanSchedule.TimeOfDay != "22:30" {
		t.Fatalf("scan schedule did not round-trip: %#v", loaded.ScanSchedule)
	}
	if !loaded.Login.LaunchAtLogin {
		t.Fatal("login setting did not round-trip")
	}

	entries, err := os.ReadDir(filepath.Dir(store.Path))
	if err != nil {
		t.Fatalf("read settings dir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("atomic save left temp file: %s", entry.Name())
		}
	}
}

func TestSettingsStoreForcesStartHiddenOffWithoutMenuBarIcon(t *testing.T) {
	store := SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}
	settings := defaultSettings()
	settings.Background.StartHidden = true
	settings.Background.KeepMenuBarIcon = false

	if err := store.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if loaded.Background.StartHidden {
		t.Fatal("startHidden 應在未保留狀態列圖示時被強制關閉")
	}
}

func TestSettingsStoreRejectsUnsupportedSchemaVersion(t *testing.T) {
	store := SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}
	content, err := json.Marshal(Settings{SchemaVersion: currentSettingsSchemaVersion + 1})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(store.Path, content, 0600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	_, err = store.Load()

	if !errors.Is(err, errUnsupportedSettingsVersion) {
		t.Fatalf("expected unsupported schema version, got %v", err)
	}
}

func TestSettingsStoreRunsMigrationHook(t *testing.T) {
	store := SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}
	content := []byte(`{"schemaVersion":0,"runtimeMode":"system-shared"}`)
	if err := os.WriteFile(store.Path, content, 0600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	store.Migrate = func(settings *Settings) error {
		settings.SchemaVersion = currentSettingsSchemaVersion
		settings.UpdateSchedule.Enabled = true
		return nil
	}

	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.SchemaVersion != currentSettingsSchemaVersion {
		t.Fatalf("migration did not update schema version: %d", settings.SchemaVersion)
	}
}

func TestUserSettingsPathIsPerUserApplicationSupport(t *testing.T) {
	path := userSettingsPath("/Users/jerry")
	expected := "/Users/jerry/Library/Application Support/ClamAVDesktop/settings.json"

	if path != expected {
		t.Fatalf("unexpected settings path: got %q, expected %q", path, expected)
	}
}

func TestAppSettingsMethodsUseSameStore(t *testing.T) {
	app := &App{settingsStore: SettingsStore{Path: filepath.Join(t.TempDir(), "settings.json")}}
	settings := defaultSettings()
	settings.PowerPolicy.RunOnBattery = true

	saved, err := app.SaveSettings(settings)
	if err != nil {
		t.Fatalf("save app settings: %v", err)
	}
	if !saved.PowerPolicy.RunOnBattery {
		t.Fatal("saved settings did not return updated power policy")
	}

	loaded, err := app.GetSettings()
	if err != nil {
		t.Fatalf("get app settings: %v", err)
	}
	if !loaded.PowerPolicy.RunOnBattery {
		t.Fatal("app settings methods did not use the same store")
	}
}
