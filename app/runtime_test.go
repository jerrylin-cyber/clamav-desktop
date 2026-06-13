package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeResolverDetectsSystemSharedRuntime(t *testing.T) {
	base := t.TempDir()
	makeExecutable(t, filepath.Join(base, "Runtime/bin/clamscan"))
	makeExecutable(t, filepath.Join(base, "Runtime/bin/freshclam"))
	makeExecutable(t, filepath.Join(base, "Runtime/sbin/clamd"))

	profile := RuntimeResolver{systemBase: base, externalCandidates: []RuntimeProfile{}}.Resolve()

	if profile.Mode != "system-shared" {
		t.Fatalf("expected system-shared runtime, got %q", profile.Mode)
	}
	if len(profile.Warnings) == 0 {
		t.Fatal("expected legacy app-managed warning")
	}
}

func TestRuntimeResolverPrefersHomebrewRuntime(t *testing.T) {
	systemBase := t.TempDir()
	makeExecutable(t, filepath.Join(systemBase, "Runtime/bin/clamscan"))
	makeExecutable(t, filepath.Join(systemBase, "Runtime/bin/freshclam"))
	makeExecutable(t, filepath.Join(systemBase, "Runtime/sbin/clamd"))

	homebrewBase := t.TempDir()
	makeExecutable(t, filepath.Join(homebrewBase, "bin/clamscan"))
	profile := RuntimeResolver{
		systemBase: systemBase,
		externalCandidates: []RuntimeProfile{
			externalRuntimeProfile("homebrew-arm64", homebrewBase),
		},
	}.Resolve()

	if profile.Mode != "external" || profile.Source != "homebrew-arm64" {
		t.Fatalf("expected homebrew external runtime, got mode=%q source=%q", profile.Mode, profile.Source)
	}
}

func TestRuntimeResolverDoesNotTreatPartialSystemRuntimeAsInstalled(t *testing.T) {
	base := t.TempDir()
	makeExecutable(t, filepath.Join(base, "Runtime/bin/freshclam"))

	profile := RuntimeResolver{systemBase: base, externalCandidates: []RuntimeProfile{}}.Resolve()

	if profile.Mode != "missing" {
		t.Fatalf("expected missing runtime, got %q", profile.Mode)
	}
	if len(profile.Warnings) == 0 {
		t.Fatal("expected missing runtime warning")
	}
}

func TestRuntimeResolverReportsPerUserRuntimeAsUnsupported(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	makeExecutable(t, filepath.Join(home, "Library/Application Support/ClamAVDesktop/Runtime/sbin/clamd"))

	profile := RuntimeResolver{systemBase: base, homeDir: home, externalCandidates: []RuntimeProfile{}}.Resolve()

	if profile.Mode != "per-user" {
		t.Fatalf("expected per-user runtime, got %q", profile.Mode)
	}
	if profile.Source != "per-user-runtime" {
		t.Fatalf("expected per-user source, got %q", profile.Source)
	}
}

func TestRuntimeResolverDetectsManualRuntime(t *testing.T) {
	base := t.TempDir()
	makeExecutable(t, filepath.Join(base, "bin/clamscan"))

	profile := RuntimeResolver{systemBase: t.TempDir(), manualBase: base, externalCandidates: []RuntimeProfile{}}.Resolve()

	if profile.Mode != "manual" {
		t.Fatalf("expected manual runtime, got %q", profile.Mode)
	}
	if profile.Source != "manual" {
		t.Fatalf("expected manual source, got %q", profile.Source)
	}
	if len(profile.Warnings) == 0 {
		t.Fatal("expected manual runtime warning")
	}
}

func TestRuntimeHealthRequiresSocketPing(t *testing.T) {
	base := t.TempDir()
	makeExecutable(t, filepath.Join(base, "Runtime/bin/clamscan"))
	makeExecutable(t, filepath.Join(base, "Runtime/bin/freshclam"))
	makeExecutable(t, filepath.Join(base, "Runtime/sbin/clamd"))
	mkdir(t, filepath.Join(base, "Config"))
	mkdir(t, filepath.Join(base, "Database"))

	profile := appManagedProfile(base)
	profile.Mode = "system-shared"

	health := runtimeHealth(profile)

	if health.Status != "repair-required" {
		t.Fatalf("expected repair-required health, got %q", health.Status)
	}
}

func TestRuntimeHealthAcceptsClamdPing(t *testing.T) {
	restore := stubClamdReply("PONG\x00")
	defer restore()

	check := clamdCommandCheck("clamd ping", "/tmp/clamd.sock", "PING", func(reply string) bool {
		return reply == "PONG"
	})

	if check.Status != "ok" {
		t.Fatalf("expected ok ping check, got %#v", check)
	}
}

func TestClamdVersionCommandsCheckAcceptsCommandList(t *testing.T) {
	restore := stubClamdReply("ClamAV 1.5.2/COMMANDS: SCAN INSTREAM\x00")
	defer restore()

	check := clamdCommandCheck("clamd commands", "/tmp/clamd.sock", "VERSIONCOMMANDS", func(reply string) bool {
		return reply == "ClamAV 1.5.2/COMMANDS: SCAN INSTREAM"
	})

	if check.Status != "ok" {
		t.Fatalf("expected ok version commands check, got %#v", check)
	}
}

func makeExecutable(t *testing.T, path string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func stubClamdReply(reply string) func() {
	originalDial := dialUnix
	dialUnix = func(network string, address string, timeout time.Duration) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			buf := make([]byte, 64)
			_, _ = server.Read(buf)
			_, _ = server.Write([]byte(reply))
		}()
		return client, nil
	}
	return func() {
		dialUnix = originalDial
	}
}
