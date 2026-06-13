package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveUserDataRequiresSelection(t *testing.T) {
	_, err := removeUserData(userDataPathsForHome(t.TempDir()), UserDataRemovalOptions{})
	if err == nil {
		t.Fatal("expected error when no user data category is selected")
	}
}

func TestRemoveUserDataRemovesSelectedPathsOnly(t *testing.T) {
	homeDir := t.TempDir()
	paths := userDataPathsForHome(homeDir)
	writeUserDataFixture(t, paths.Settings)
	writeUserDataFixture(t, filepath.Join(paths.Jobs, "job.json"))
	writeUserDataFixture(t, filepath.Join(paths.Results, "job.json"))
	writeUserDataFixture(t, filepath.Join(paths.Quarantine, "records", "record.json"))
	writeUserDataFixture(t, filepath.Join(paths.Logs, "app.log"))

	result, err := removeUserData(paths, UserDataRemovalOptions{
		RemoveSettings:    true,
		RemoveScanJobs:    true,
		RemoveScanResults: true,
	})
	if err != nil {
		t.Fatalf("remove user data: %v", err)
	}
	if len(result.Removed) != 3 {
		t.Fatalf("expected 3 removed paths, got %#v", result)
	}

	assertPathMissing(t, paths.Settings)
	assertPathMissing(t, paths.Jobs)
	assertPathMissing(t, paths.Results)
	assertPathExists(t, paths.Quarantine)
	assertPathExists(t, paths.Logs)
}

func TestRemoveUserDataRemovesQuarantineOnlyWhenExplicit(t *testing.T) {
	homeDir := t.TempDir()
	paths := userDataPathsForHome(homeDir)
	writeUserDataFixture(t, filepath.Join(paths.Quarantine, "records", "record.json"))

	if _, err := removeUserData(paths, UserDataRemovalOptions{RemoveSettings: true}); err != nil {
		t.Fatalf("remove settings: %v", err)
	}
	assertPathExists(t, paths.Quarantine)

	if _, err := removeUserData(paths, UserDataRemovalOptions{RemoveQuarantine: true}); err != nil {
		t.Fatalf("remove quarantine: %v", err)
	}
	assertPathMissing(t, paths.Quarantine)
}

func TestRemoveUserDataResultSerializesEmptySlices(t *testing.T) {
	homeDir := t.TempDir()
	paths := userDataPathsForHome(homeDir)
	writeUserDataFixture(t, paths.Settings)

	result, err := removeUserData(paths, UserDataRemovalOptions{RemoveSettings: true})
	if err != nil {
		t.Fatalf("remove settings: %v", err)
	}

	// 前端會讀 result.removed.length / result.skipped.length；nil slice 序列化為 null 會讓前端噴錯
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(encoded), "null") {
		t.Fatalf("result 不應序列化為 null，實際為 %s", encoded)
	}
}

func writeUserDataFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path to exist %s: %v", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path to be missing %s, got %v", path, err)
	}
}
