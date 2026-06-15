package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFreshclamServiceUpdatesDatabaseAndStoresStatus(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
	configPath := ""
	service := testFreshclamService(t, func(_ context.Context, command string, args []string, stdout io.Writer, stderr io.Writer) error {
		if command != "/Runtime/bin/freshclam" {
			t.Fatalf("unexpected command: %s", command)
		}
		if len(args) != 2 || args[0] != "--foreground" || args[1] != "--config-file="+configPath {
			t.Fatalf("unexpected args: %#v", args)
		}
		_, _ = io.WriteString(stdout, "daily.cvd database is up-to-date (version: 27123, sigs: 2040312)\n")
		_, _ = io.WriteString(stderr, "WARNING: test warning\n")
		return nil
	})
	configPath = filepath.Join(service.Profile.ConfigPath, "freshclam.conf")
	service.now = func() time.Time { return now }

	var events []FreshclamEvent
	status, err := service.UpdateDatabase(context.Background(), func(event FreshclamEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("update database: %v", err)
	}

	if status.Path != service.Profile.DatabasePath {
		t.Fatalf("unexpected status path: %s", status.Path)
	}
	if status.Version != "27123" {
		t.Fatalf("unexpected version: %s", status.Version)
	}
	if status.Signatures != 2040312 {
		t.Fatalf("unexpected signatures: %d", status.Signatures)
	}
	if !status.LastUpdated.Equal(now) {
		t.Fatalf("unexpected updated time: %s", status.LastUpdated)
	}
	if len(events) != 2 {
		t.Fatalf("expected stdout and stderr events, got %#v", events)
	}

	loaded, err := service.LoadStatus()
	if err != nil {
		t.Fatalf("load status: %v", err)
	}
	if loaded.Version != status.Version || loaded.Signatures != status.Signatures {
		t.Fatalf("stored status mismatch: %#v", loaded)
	}
}

func TestFreshclamServiceGeneratesConfigWhenRuntimeConfigIsMissing(t *testing.T) {
	dir := t.TempDir()
	generatedConfig := filepath.Join(dir, "Config/freshclam.conf")
	generatedDatabase := filepath.Join(dir, "Database")
	generatedLog := filepath.Join(dir, "Logs/freshclam.log")

	service := &FreshclamService{
		Profile: RuntimeProfile{
			Mode:          "external",
			FreshclamPath: "/Runtime/bin/freshclam",
			ConfigPath:    filepath.Join(dir, "MissingConfig"),
			DatabasePath:  "/ExternalDatabase",
		},
		StatusPath:            filepath.Join(dir, "status.json"),
		GeneratedConfigPath:   generatedConfig,
		GeneratedDatabasePath: generatedDatabase,
		GeneratedLogPath:      generatedLog,
		run: func(_ context.Context, _ string, args []string, stdout io.Writer, _ io.Writer) error {
			if len(args) != 2 || args[1] != "--config-file="+generatedConfig {
				t.Fatalf("unexpected args: %#v", args)
			}
			_, _ = io.WriteString(stdout, "daily.cvd database is up-to-date (version: 99, sigs: 123)\n")
			return nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
		},
	}

	status, err := service.UpdateDatabase(context.Background(), nil)
	if err != nil {
		t.Fatalf("update database: %v", err)
	}
	if status.Path != generatedDatabase {
		t.Fatalf("expected generated database path, got %s", status.Path)
	}

	content, err := os.ReadFile(generatedConfig)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	config := string(content)
	for _, want := range []string{
		"DatabaseDirectory " + generatedDatabase,
		"UpdateLogFile " + generatedLog,
		"DatabaseMirror database.clamav.net",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("generated config missing %q:\n%s", want, config)
		}
	}
}

func TestFreshclamServiceUsesGeneratedConfigForExternalRuntime(t *testing.T) {
	dir := t.TempDir()
	runtimeConfigDir := filepath.Join(dir, "RuntimeConfig")
	generatedConfig := filepath.Join(dir, "Config/freshclam.conf")
	generatedDatabase := filepath.Join(dir, "Database")
	generatedLog := filepath.Join(dir, "Logs/freshclam.log")

	if err := os.MkdirAll(runtimeConfigDir, 0700); err != nil {
		t.Fatalf("create runtime config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeConfigDir, "freshclam.conf"), []byte("DatabaseMirror database.clamav.net\n"), 0600); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	service := &FreshclamService{
		Profile: RuntimeProfile{
			Mode:          "external",
			FreshclamPath: "/Runtime/bin/freshclam",
			ConfigPath:    runtimeConfigDir,
			DatabasePath:  "/ExternalDatabase",
		},
		StatusPath:            filepath.Join(dir, "status.json"),
		GeneratedConfigPath:   generatedConfig,
		GeneratedDatabasePath: generatedDatabase,
		GeneratedLogPath:      generatedLog,
		run: func(_ context.Context, _ string, args []string, stdout io.Writer, _ io.Writer) error {
			if len(args) != 2 || args[1] != "--config-file="+generatedConfig {
				t.Fatalf("unexpected args: %#v", args)
			}
			_, _ = io.WriteString(stdout, "daily.cvd database is up-to-date (version: 99, sigs: 123)\n")
			return nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
		},
	}

	if service.activeFreshclamConfigPath() != generatedConfig {
		t.Fatalf("expected generated config path, got %s", service.activeFreshclamConfigPath())
	}

	status, err := service.UpdateDatabase(context.Background(), nil)
	if err != nil {
		t.Fatalf("update database: %v", err)
	}
	if status.Path != generatedDatabase {
		t.Fatalf("expected generated database path, got %s", status.Path)
	}
}

func TestFreshclamServiceClassifiesNetworkError(t *testing.T) {
	service := testFreshclamService(t, func(_ context.Context, _ string, _ []string, _ io.Writer, stderr io.Writer) error {
		_, _ = io.WriteString(stderr, "ERROR: Can't download daily.cvd from database.clamav.net\n")
		return errors.New("exit status 1")
	})

	status, err := service.UpdateDatabase(context.Background(), nil)

	var freshclamErr FreshclamError
	if !errors.As(err, &freshclamErr) {
		t.Fatalf("expected FreshclamError, got %T", err)
	}
	if freshclamErr.Category != "network" {
		t.Fatalf("expected network category, got %q", freshclamErr.Category)
	}
	if !strings.Contains(status.Error, "網路錯誤") {
		t.Fatalf("expected user-facing network error, got %q", status.Error)
	}
}

func TestFreshclamServiceClassifiesPermissionConfigAndLockErrors(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		expected string
	}{
		{name: "permission", output: "ERROR: Permission denied", expected: "permission"},
		{name: "config", output: "ERROR: Can't parse config file", expected: "config"},
		{name: "lock", output: "ERROR: Database update process is already running and locked", expected: "lock"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := testFreshclamService(t, func(_ context.Context, _ string, _ []string, _ io.Writer, stderr io.Writer) error {
				_, _ = io.WriteString(stderr, tc.output)
				return errors.New("exit status 1")
			})

			_, err := service.UpdateDatabase(context.Background(), nil)

			var freshclamErr FreshclamError
			if !errors.As(err, &freshclamErr) {
				t.Fatalf("expected FreshclamError, got %T", err)
			}
			if freshclamErr.Category != tc.expected {
				t.Fatalf("expected %s category, got %q", tc.expected, freshclamErr.Category)
			}
		})
	}
}

func TestFreshclamServicePreventsConcurrentUpdates(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := testFreshclamService(t, func(ctx context.Context, _ string, _ []string, stdout io.Writer, _ io.Writer) error {
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
		}
		_, _ = io.WriteString(stdout, "daily.cvd database is up-to-date (version: 1, sigs: 2)\n")
		return nil
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = service.UpdateDatabase(context.Background(), nil)
	}()
	<-started

	_, err := service.UpdateDatabase(context.Background(), nil)
	if !errors.Is(err, errFreshclamUpdateInProgress) {
		t.Fatalf("expected update lock error, got %v", err)
	}

	close(release)
	wg.Wait()
}

func TestFreshclamServiceLoadStatusReturnsEmptyStatusWhenMissing(t *testing.T) {
	service := testFreshclamService(t, nil)

	status, err := service.LoadStatus()
	if err != nil {
		t.Fatalf("load missing status: %v", err)
	}
	if status.Path != service.Profile.DatabasePath {
		t.Fatalf("unexpected status path: %s", status.Path)
	}
	if !status.LastUpdated.IsZero() {
		t.Fatalf("expected zero last updated, got %s", status.LastUpdated)
	}
}

func TestAppDatabaseMethodsUseFreshclamService(t *testing.T) {
	service := testFreshclamService(t, func(_ context.Context, _ string, _ []string, stdout io.Writer, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "daily.cvd database is up-to-date (version: 42, sigs: 100)\n")
		return nil
	})
	app := &App{freshclamService: service, logService: testLogService(t)}

	updated, err := app.UpdateDatabase()
	if err != nil {
		t.Fatalf("update database: %v", err)
	}
	if updated.Version != "42" {
		t.Fatalf("unexpected updated version: %s", updated.Version)
	}

	status, err := app.GetDatabaseStatus()
	if err != nil {
		t.Fatalf("get database status: %v", err)
	}
	if status.Version != "42" || status.Signatures != 100 {
		t.Fatalf("database methods did not share status: %#v", status)
	}
}

func TestParseFreshclamStatusUsesLatestVersionLine(t *testing.T) {
	status := parseFreshclamStatus("/Database", strings.Join([]string{
		"main.cvd database is up-to-date (version: 63, sigs: 7060360)",
		"daily.cvd database is up-to-date (version: 27123, sigs: 2040312)",
	}, "\n"), time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC))

	if status.Version != "27123" {
		t.Fatalf("unexpected parsed version: %s", status.Version)
	}
	if status.Signatures != 2040312 {
		t.Fatalf("unexpected parsed signatures: %d", status.Signatures)
	}
}

func testFreshclamService(t *testing.T, run freshclamRunner) *FreshclamService {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "Config")
	databasePath := filepath.Join(dir, "Database")
	if err := os.MkdirAll(configPath, 0700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.MkdirAll(databasePath, 0700); err != nil {
		t.Fatalf("create database dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configPath, "freshclam.conf"), []byte("DatabaseMirror database.clamav.net\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return &FreshclamService{
		Profile: RuntimeProfile{
			FreshclamPath: "/Runtime/bin/freshclam",
			ConfigPath:    configPath,
			DatabasePath:  databasePath,
		},
		StatusPath: filepath.Join(dir, "status.json"),
		run:        run,
		now: func() time.Time {
			return time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
		},
	}
}
