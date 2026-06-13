package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestSharedRuntimeInstallPlanUsesSystemSharedPaths(t *testing.T) {
	layout := sharedRuntimeLayout("/Library/Application Support/ClamAVDesktop")
	plan, err := SharedRuntimeInstaller{Layout: layout}.BuildInstallPlan("1.5.2", "0.1.0")
	if err != nil {
		t.Fatalf("build install plan: %v", err)
	}

	assertDirectory(t, plan, "/Library/Application Support/ClamAVDesktop/Runtime")
	assertDirectory(t, plan, "/Library/Application Support/ClamAVDesktop/Config")
	assertDirectory(t, plan, "/Library/Application Support/ClamAVDesktop/Database")
	assertDirectory(t, plan, "/Library/Application Support/ClamAVDesktop/Run")

	if !hasFile(plan, "/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.freshclam.plist") {
		t.Fatal("expected freshclam LaunchDaemon plist")
	}
	if !hasFile(plan, "/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.clamd.plist") {
		t.Fatal("expected clamd LaunchDaemon plist")
	}
}

func TestSharedRuntimeConfigsUseUnixSocketAndSharedDatabase(t *testing.T) {
	layout := sharedRuntimeLayout("/Library/Application Support/ClamAVDesktop")

	clamd := clamdConfig(layout)
	if !strings.Contains(clamd, "DatabaseDirectory /Library/Application Support/ClamAVDesktop/Database\n") {
		t.Fatalf("clamd.conf does not use shared database:\n%s", clamd)
	}
	if !strings.Contains(clamd, "LocalSocket /Library/Application Support/ClamAVDesktop/Run/clamd.sock\n") {
		t.Fatalf("clamd.conf does not use app-owned socket:\n%s", clamd)
	}
	if strings.Contains(clamd, "TCPSocket") || strings.Contains(clamd, "/tmp/clamd") {
		t.Fatalf("clamd.conf must not enable TCP or /tmp socket:\n%s", clamd)
	}
	if !strings.Contains(clamd, "AlertEncrypted yes\n") {
		t.Fatalf("clamd.conf does not alert on encrypted files:\n%s", clamd)
	}
	if !strings.Contains(clamd, "AlgorithmicDetection yes\n") {
		t.Fatalf("clamd.conf does not enable algorithmic (heuristic) detection:\n%s", clamd)
	}

	freshclam := freshclamConfig(layout)
	if !strings.Contains(freshclam, "DatabaseDirectory /Library/Application Support/ClamAVDesktop/Database\n") {
		t.Fatalf("freshclam.conf does not use shared database:\n%s", freshclam)
	}
	if !strings.Contains(freshclam, "NotifyClamd /Library/Application Support/ClamAVDesktop/Config/clamd.conf\n") {
		t.Fatalf("freshclam.conf does not notify clamd config:\n%s", freshclam)
	}
}

func TestInstallManifestRecordsOnlyAppManagedPaths(t *testing.T) {
	layout := sharedRuntimeLayout("/Library/Application Support/ClamAVDesktop")
	plan, err := SharedRuntimeInstaller{Layout: layout}.BuildInstallPlan("1.5.2", "0.1.0")
	if err != nil {
		t.Fatalf("build install plan: %v", err)
	}

	manifestFile := fileContent(t, plan, layout.ManifestPath)
	var manifest InstallManifest
	if err := json.Unmarshal([]byte(manifestFile), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}

	if manifest.Mode != "system-shared" {
		t.Fatalf("expected system-shared mode, got %q", manifest.Mode)
	}
	if manifest.RuntimeVersion != "1.5.2" {
		t.Fatalf("expected runtime version, got %q", manifest.RuntimeVersion)
	}
	if manifest.DatabasePath != layout.DatabasePath || manifest.ConfigPath != layout.ConfigPath {
		t.Fatalf("manifest paths do not match layout: %#v", manifest)
	}
	for _, path := range manifest.InstalledPaths {
		if strings.Contains(path, "/opt/homebrew") || strings.Contains(path, "/usr/local/clamav") {
			t.Fatalf("manifest must not record external runtime path: %s", path)
		}
	}
}

func TestLaunchDaemonPlistsUseAbsoluteBinariesAndLaunchdLogs(t *testing.T) {
	layout := sharedRuntimeLayout("/Library/Application Support/ClamAVDesktop")
	plan, err := SharedRuntimeInstaller{Layout: layout}.BuildInstallPlan("1.5.2", "0.1.0")
	if err != nil {
		t.Fatalf("build install plan: %v", err)
	}

	freshclam := fileContent(t, plan, layout.FreshclamLaunchdPath)
	assertContains(t, freshclam, "<string>/Library/Application Support/ClamAVDesktop/Runtime/bin/freshclam</string>")
	assertContains(t, freshclam, "<string>--config-file=/Library/Application Support/ClamAVDesktop/Config/freshclam.conf</string>")
	assertContains(t, freshclam, "<key>StartInterval</key>")
	assertContains(t, freshclam, "<integer>21600</integer>")

	clamd := fileContent(t, plan, layout.ClamdLaunchdPath)
	assertContains(t, clamd, "<string>/Library/Application Support/ClamAVDesktop/Runtime/sbin/clamd</string>")
	assertContains(t, clamd, "<string>--config-file=/Library/Application Support/ClamAVDesktop/Config/clamd.conf</string>")
	assertContains(t, clamd, "<key>KeepAlive</key>")
	assertContains(t, clamd, "<true/>")
}

func TestLaunchDaemonManagerUsesLaunchctlSystemDomain(t *testing.T) {
	var calls [][]string
	manager := LaunchDaemonManager{
		run: func(_ context.Context, args ...string) ([]byte, error) {
			calls = append(calls, append([]string{}, args...))
			return []byte("ok"), nil
		},
	}

	if err := manager.Load(context.Background(), "/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.clamd.plist"); err != nil {
		t.Fatalf("load launch daemon: %v", err)
	}
	if err := manager.Unload(context.Background(), clamdLaunchDaemonLabel); err != nil {
		t.Fatalf("unload launch daemon: %v", err)
	}
	status, err := manager.Status(context.Background(), freshclamLaunchDaemonLabel)
	if err != nil {
		t.Fatalf("status launch daemon: %v", err)
	}
	if status != "ok" {
		t.Fatalf("expected status output, got %q", status)
	}

	assertCall(t, calls, 0, "bootstrap", "system", "/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.clamd.plist")
	assertCall(t, calls, 1, "bootout", "system/"+clamdLaunchDaemonLabel)
	assertCall(t, calls, 2, "print", "system/"+freshclamLaunchDaemonLabel)
}

func assertDirectory(t *testing.T, plan SharedRuntimeInstallPlan, path string) {
	t.Helper()
	for _, directory := range plan.Directories {
		if directory.Path == path {
			return
		}
	}
	t.Fatalf("expected directory %s in install plan", path)
}

func hasFile(plan SharedRuntimeInstallPlan, path string) bool {
	for _, file := range plan.Files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func fileContent(t *testing.T, plan SharedRuntimeInstallPlan, path string) string {
	t.Helper()
	for _, file := range plan.Files {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("expected file %s in install plan", path)
	return ""
}

func assertContains(t *testing.T, content string, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected content to contain %q:\n%s", expected, content)
	}
}

func assertCall(t *testing.T, calls [][]string, index int, expected ...string) {
	t.Helper()
	if len(calls) <= index {
		t.Fatalf("expected call %d, got %d calls", index, len(calls))
	}
	actual := calls[index]
	if len(actual) != len(expected) {
		t.Fatalf("call %d length mismatch: got %#v, expected %#v", index, actual, expected)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("call %d mismatch: got %#v, expected %#v", index, actual, expected)
		}
	}
}

func TestApplyInstallPlanWritesDirectoriesAndFilesWithModes(t *testing.T) {
	tmp := t.TempDir()
	plan := SharedRuntimeInstallPlan{
		Directories: []InstallDirectory{
			{Path: filepath.Join(tmp, "Runtime"), Mode: "0755"},
		},
		Files: []InstallFile{
			{Path: filepath.Join(tmp, "Config", "clamd.conf"), Content: "LogVerbose yes\n", Mode: "0644"},
		},
	}

	if err := ApplyInstallPlan(plan); err != nil {
		t.Fatalf("apply install plan: %v", err)
	}

	dirInfo, err := os.Stat(plan.Directories[0].Path)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("expected %s to be a directory", plan.Directories[0].Path)
	}
	if dirInfo.Mode().Perm() != 0o755 {
		t.Fatalf("expected directory mode 0755, got %o", dirInfo.Mode().Perm())
	}

	content, err := os.ReadFile(plan.Files[0].Path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != plan.Files[0].Content {
		t.Fatalf("file content mismatch: got %q", content)
	}
	fileInfo, err := os.Stat(plan.Files[0].Path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o644 {
		t.Fatalf("expected file mode 0644, got %o", fileInfo.Mode().Perm())
	}
}

func TestExtractRuntimeArchiveExtractsRuntimeTreeAndSkipsOtherEntries(t *testing.T) {
	tmp := t.TempDir()
	archivePath, checksumPath := writeRuntimeArchiveFixture(t, tmp, []archiveEntry{
		{name: "Runtime/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "Runtime/bin/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "Runtime/bin/clamscan", typeflag: tar.TypeReg, mode: 0o755, content: []byte("#!/bin/sh\necho clamscan\n")},
		{name: "Runtime/lib/libclamav.9.dylib", typeflag: tar.TypeReg, mode: 0o644, content: []byte("dylib-bytes")},
		{name: "Runtime/lib/libclamav.dylib", typeflag: tar.TypeSymlink, linkname: "libclamav.9.dylib"},
		{name: "Config/freshclam.conf.sample", typeflag: tar.TypeReg, mode: 0o644, content: []byte("# sample")},
		{name: "Runtime/../escape.txt", typeflag: tar.TypeReg, mode: 0o644, content: []byte("escape")},
	})

	dest := filepath.Join(tmp, "installed", "Runtime")
	if err := ExtractRuntimeArchive(archivePath, checksumPath, dest); err != nil {
		t.Fatalf("extract runtime archive: %v", err)
	}

	clamscanPath := filepath.Join(dest, "bin/clamscan")
	info, err := os.Stat(clamscanPath)
	if err != nil {
		t.Fatalf("stat clamscan: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected clamscan mode 0755, got %o", info.Mode().Perm())
	}
	content, err := os.ReadFile(clamscanPath)
	if err != nil {
		t.Fatalf("read clamscan: %v", err)
	}
	if string(content) != "#!/bin/sh\necho clamscan\n" {
		t.Fatalf("unexpected clamscan content: %q", content)
	}

	link, err := os.Readlink(filepath.Join(dest, "lib/libclamav.dylib"))
	if err != nil {
		t.Fatalf("readlink libclamav.dylib: %v", err)
	}
	if link != "libclamav.9.dylib" {
		t.Fatalf("expected symlink target libclamav.9.dylib, got %s", link)
	}

	if _, err := os.Stat(filepath.Join(tmp, "installed", "Config")); err == nil {
		t.Fatal("Config/ from the artifact must not be extracted alongside Runtime/")
	}
	if _, err := os.Stat(filepath.Join(tmp, "escape.txt")); err == nil {
		t.Fatal("entries outside Runtime/ must not be extracted")
	}
}

func TestExtractRuntimeArchiveRejectsChecksumMismatch(t *testing.T) {
	tmp := t.TempDir()
	archivePath, checksumPath := writeRuntimeArchiveFixture(t, tmp, []archiveEntry{
		{name: "Runtime/bin/clamscan", typeflag: tar.TypeReg, mode: 0o755, content: []byte("clamscan")},
	})

	wrongSum := strings.Repeat("0", 64) + "\n"
	if err := os.WriteFile(checksumPath, []byte(wrongSum), 0o644); err != nil {
		t.Fatalf("overwrite checksum: %v", err)
	}

	dest := filepath.Join(tmp, "Runtime")
	if err := ExtractRuntimeArchive(archivePath, checksumPath, dest); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("Runtime/ must not be created when checksum verification fails")
	}
}

type archiveEntry struct {
	name     string
	typeflag byte
	mode     int64
	content  []byte
	linkname string
}

func writeRuntimeArchiveFixture(t *testing.T, dir string, entries []archiveEntry) (string, string) {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Typeflag: entry.typeflag,
			Mode:     entry.mode,
			Size:     int64(len(entry.content)),
			Linkname: entry.linkname,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %s: %v", entry.name, err)
		}
		if len(entry.content) > 0 {
			if _, err := tw.Write(entry.content); err != nil {
				t.Fatalf("write tar content %s: %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}

	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatalf("create zstd writer: %v", err)
	}
	if _, err := encoder.Write(tarBuf.Bytes()); err != nil {
		t.Fatalf("write zstd content: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}

	archivePath := filepath.Join(dir, "clamav-runtime-darwin-universal.tar.zst")
	if err := os.WriteFile(archivePath, compressed.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	sum := sha256.Sum256(compressed.Bytes())
	checksumPath := filepath.Join(dir, "clamav-runtime-darwin-universal.sha256")
	if err := os.WriteFile(checksumPath, []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
		t.Fatalf("write checksum: %v", err)
	}

	return archivePath, checksumPath
}
