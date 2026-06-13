package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	freshclamLaunchDaemonLabel = "com.lazyjerry.clamavdesktop.freshclam"
	clamdLaunchDaemonLabel     = "com.lazyjerry.clamavdesktop.clamd"
	systemLogBase              = "/Library/Logs/ClamAVDesktop"
)

type SharedRuntimeLayout struct {
	BasePath               string
	RuntimePath            string
	ConfigPath             string
	DatabasePath           string
	RunPath                string
	LogPath                string
	ManifestPath           string
	FreshclamConfigPath    string
	ClamdConfigPath        string
	ClamdSocketPath        string
	FreshclamPath          string
	ClamdPath              string
	FreshclamLaunchdPath   string
	ClamdLaunchdPath       string
	FreshclamLogPath       string
	ClamdLogPath           string
	FreshclamLaunchdLog    string
	ClamdLaunchdLog        string
	FreshclamLaunchdErrLog string
	ClamdLaunchdErrLog     string
}

type InstallManifest struct {
	Mode                  string   `json:"mode"`
	RuntimeVersion        string   `json:"runtimeVersion"`
	InstalledPaths        []string `json:"installedPaths"`
	LaunchdLabels         []string `json:"launchdLabels"`
	DatabasePath          string   `json:"databasePath"`
	ConfigPath            string   `json:"configPath"`
	InstalledByAppVersion string   `json:"installedByAppVersion"`
}

type InstallDirectory struct {
	Path string
	Mode string
}

type InstallFile struct {
	Path    string
	Content string
	Mode    string
}

type SharedRuntimeInstallPlan struct {
	Directories []InstallDirectory
	Files       []InstallFile
	Manifest    InstallManifest
}

type SharedRuntimeInstaller struct {
	Layout SharedRuntimeLayout
}

type launchctlRunner func(ctx context.Context, args ...string) ([]byte, error)

type LaunchDaemonManager struct {
	Domain string
	run    launchctlRunner
}

func defaultSharedRuntimeLayout() SharedRuntimeLayout {
	return sharedRuntimeLayout(systemRuntimeBase)
}

func sharedRuntimeLayout(base string) SharedRuntimeLayout {
	layout := SharedRuntimeLayout{
		BasePath:               base,
		RuntimePath:            filepath.Join(base, "Runtime"),
		ConfigPath:             filepath.Join(base, "Config"),
		DatabasePath:           filepath.Join(base, "Database"),
		RunPath:                filepath.Join(base, "Run"),
		LogPath:                systemLogBase,
		ManifestPath:           filepath.Join(base, "install-manifest.json"),
		FreshclamLaunchdPath:   "/Library/LaunchDaemons/" + freshclamLaunchDaemonLabel + ".plist",
		ClamdLaunchdPath:       "/Library/LaunchDaemons/" + clamdLaunchDaemonLabel + ".plist",
		FreshclamLaunchdLog:    filepath.Join(systemLogBase, "freshclam.launchd.log"),
		ClamdLaunchdLog:        filepath.Join(systemLogBase, "clamd.launchd.log"),
		FreshclamLaunchdErrLog: filepath.Join(systemLogBase, "freshclam.launchd.err.log"),
		ClamdLaunchdErrLog:     filepath.Join(systemLogBase, "clamd.launchd.err.log"),
		FreshclamLogPath:       filepath.Join(systemLogBase, "freshclam.log"),
		ClamdLogPath:           filepath.Join(systemLogBase, "clamd.log"),
	}
	layout.FreshclamConfigPath = filepath.Join(layout.ConfigPath, "freshclam.conf")
	layout.ClamdConfigPath = filepath.Join(layout.ConfigPath, "clamd.conf")
	layout.ClamdSocketPath = filepath.Join(layout.RunPath, "clamd.sock")
	layout.FreshclamPath = filepath.Join(layout.RuntimePath, "bin/freshclam")
	layout.ClamdPath = filepath.Join(layout.RuntimePath, "sbin/clamd")
	return layout
}

func (i SharedRuntimeInstaller) BuildInstallPlan(runtimeVersion string, appVersion string) (SharedRuntimeInstallPlan, error) {
	layout := i.Layout
	if layout.BasePath == "" {
		layout = defaultSharedRuntimeLayout()
	}

	manifest := installManifest(layout, runtimeVersion, appVersion)
	manifestContent, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return SharedRuntimeInstallPlan{}, err
	}

	return SharedRuntimeInstallPlan{
		Directories: []InstallDirectory{
			{Path: layout.BasePath, Mode: "0755"},
			{Path: layout.RuntimePath, Mode: "0755"},
			{Path: layout.ConfigPath, Mode: "0755"},
			{Path: layout.DatabasePath, Mode: "0755"},
			{Path: layout.RunPath, Mode: "0755"},
			{Path: layout.LogPath, Mode: "0755"},
		},
		Files: []InstallFile{
			{Path: layout.FreshclamConfigPath, Content: freshclamConfig(layout), Mode: "0644"},
			{Path: layout.ClamdConfigPath, Content: clamdConfig(layout), Mode: "0644"},
			{Path: layout.ManifestPath, Content: string(manifestContent) + "\n", Mode: "0644"},
			{Path: layout.FreshclamLaunchdPath, Content: freshclamLaunchDaemonPlist(layout), Mode: "0644"},
			{Path: layout.ClamdLaunchdPath, Content: clamdLaunchDaemonPlist(layout), Mode: "0644"},
		},
		Manifest: manifest,
	}, nil
}

// ApplyInstallPlan creates the directories and writes the files described by
// plan, applying the recorded permission modes. Writing under
// systemRuntimeBase requires the caller to already hold the necessary
// filesystem privileges.
func ApplyInstallPlan(plan SharedRuntimeInstallPlan) error {
	for _, dir := range plan.Directories {
		mode, err := parseFileMode(dir.Mode)
		if err != nil {
			return fmt.Errorf("directory %s: %w", dir.Path, err)
		}
		if err := os.MkdirAll(dir.Path, mode); err != nil {
			return fmt.Errorf("create directory %s: %w", dir.Path, err)
		}
		if err := os.Chmod(dir.Path, mode); err != nil {
			return fmt.Errorf("chmod directory %s: %w", dir.Path, err)
		}
	}

	for _, file := range plan.Files {
		mode, err := parseFileMode(file.Mode)
		if err != nil {
			return fmt.Errorf("file %s: %w", file.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return fmt.Errorf("create parent of %s: %w", file.Path, err)
		}
		if err := os.WriteFile(file.Path, []byte(file.Content), mode); err != nil {
			return fmt.Errorf("write file %s: %w", file.Path, err)
		}
		if err := os.Chmod(file.Path, mode); err != nil {
			return fmt.Errorf("chmod file %s: %w", file.Path, err)
		}
	}

	return nil
}

func parseFileMode(mode string) (os.FileMode, error) {
	value, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid file mode %q: %w", mode, err)
	}
	return os.FileMode(value), nil
}

// ExtractRuntimeArchive verifies archivePath against the sha256 checksum
// stored in checksumPath, then extracts the "Runtime/" subtree of the
// tar+zstd artifact into destRuntimePath. Entries outside "Runtime/" (such as
// the artifact's bundled Config/Database placeholders) are ignored, since the
// shared install plan generates those files itself.
func ExtractRuntimeArchive(archivePath string, checksumPath string, destRuntimePath string) error {
	if err := verifyArtifactChecksum(archivePath, checksumPath); err != nil {
		return err
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer archive.Close()

	decoder, err := zstd.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open zstd stream: %w", err)
	}
	defer decoder.Close()

	reader := tar.NewReader(decoder)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read artifact entry: %w", err)
		}

		relPath, ok := runtimeRelativePath(header.Name)
		if !ok {
			continue
		}

		target := filepath.Join(destRuntimePath, relPath)
		if err := extractArtifactEntry(reader, header, target); err != nil {
			return fmt.Errorf("extract %s: %w", header.Name, err)
		}
	}
}

func verifyArtifactChecksum(archivePath string, checksumPath string) error {
	expected, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer archive.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, archive); err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	expectedHex := strings.TrimSpace(string(expected))
	if actual != expectedHex {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHex, actual)
	}
	return nil
}

// runtimeRelativePath reports the path of name relative to the archive's
// "Runtime/" directory. Entries outside that directory, or that would escape
// destRuntimePath via "..", are rejected.
func runtimeRelativePath(name string) (string, bool) {
	const prefix = "Runtime/"

	cleaned := filepath.Clean(name)
	if !strings.HasPrefix(cleaned, prefix) {
		return "", false
	}

	rel := strings.TrimPrefix(cleaned, prefix)
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func extractArtifactEntry(reader *tar.Reader, header *tar.Header, target string) error {
	mode := os.FileMode(header.Mode) & 0o7777

	switch header.Typeflag {
	case tar.TypeDir:
		if mode == 0 {
			mode = 0o755
		}
		return os.MkdirAll(target, mode)
	case tar.TypeReg:
		if mode == 0 {
			mode = 0o644
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, reader); err != nil {
			return err
		}
		return out.Chmod(mode)
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(header.Linkname, target)
	default:
		return nil
	}
}

func installManifest(layout SharedRuntimeLayout, runtimeVersion string, appVersion string) InstallManifest {
	return InstallManifest{
		Mode:           "system-shared",
		RuntimeVersion: runtimeVersion,
		InstalledPaths: []string{
			layout.BasePath,
			layout.RuntimePath,
			layout.ConfigPath,
			layout.DatabasePath,
			layout.RunPath,
			layout.LogPath,
			layout.ManifestPath,
			layout.FreshclamConfigPath,
			layout.ClamdConfigPath,
			layout.FreshclamLaunchdPath,
			layout.ClamdLaunchdPath,
		},
		LaunchdLabels: []string{
			freshclamLaunchDaemonLabel,
			clamdLaunchDaemonLabel,
		},
		DatabasePath:          layout.DatabasePath,
		ConfigPath:            layout.ConfigPath,
		InstalledByAppVersion: appVersion,
	}
}

func freshclamConfig(layout SharedRuntimeLayout) string {
	return strings.Join([]string{
		"DatabaseDirectory " + layout.DatabasePath,
		"UpdateLogFile " + layout.FreshclamLogPath,
		"LogTime yes",
		"LogVerbose yes",
		"PidFile " + filepath.Join(layout.RunPath, "freshclam.pid"),
		"DatabaseMirror database.clamav.net",
		"ScriptedUpdates yes",
		"NotifyClamd " + layout.ClamdConfigPath,
		"",
	}, "\n")
}

func clamdConfig(layout SharedRuntimeLayout) string {
	return strings.Join([]string{
		"DatabaseDirectory " + layout.DatabasePath,
		"LocalSocket " + layout.ClamdSocketPath,
		"LocalSocketMode 666",
		"LogFile " + layout.ClamdLogPath,
		"LogTime yes",
		"LogVerbose yes",
		"PidFile " + filepath.Join(layout.RunPath, "clamd.pid"),
		"Foreground yes",
		"FixStaleSocket yes",
		"AlertEncrypted yes",
		"AlgorithmicDetection yes",
		"StreamMaxLength 100M",
		// 大檔（>StreamMaxLength）改走 SCAN 依路徑掃描，需放寬 clamd 的單檔/掃描大小上限才能完整檢查
		"MaxFileSize 2000M",
		"MaxScanSize 2000M",
		"MaxThreads 4",
		"ReadTimeout 120",
		"CommandReadTimeout 30",
		"",
	}, "\n")
}

func freshclamLaunchDaemonPlist(layout SharedRuntimeLayout) string {
	return launchDaemonPlist(launchDaemonSpec{
		Label: freshclamLaunchDaemonLabel,
		ProgramArguments: []string{
			layout.FreshclamPath,
			"--foreground",
			"--config-file=" + layout.FreshclamConfigPath,
		},
		RunAtLoad:         true,
		StartInterval:     21600,
		StandardOutPath:   layout.FreshclamLaunchdLog,
		StandardErrorPath: layout.FreshclamLaunchdErrLog,
		WorkingDirectory:  layout.BasePath,
		ProcessType:       "Background",
	})
}

func clamdLaunchDaemonPlist(layout SharedRuntimeLayout) string {
	return launchDaemonPlist(launchDaemonSpec{
		Label: clamdLaunchDaemonLabel,
		ProgramArguments: []string{
			layout.ClamdPath,
			"--foreground=true",
			"--config-file=" + layout.ClamdConfigPath,
		},
		RunAtLoad:         true,
		KeepAlive:         true,
		StandardOutPath:   layout.ClamdLaunchdLog,
		StandardErrorPath: layout.ClamdLaunchdErrLog,
		WorkingDirectory:  layout.BasePath,
		ProcessType:       "Interactive",
	})
}

type launchDaemonSpec struct {
	Label             string
	ProgramArguments  []string
	RunAtLoad         bool
	KeepAlive         bool
	StartInterval     int
	StandardOutPath   string
	StandardErrorPath string
	WorkingDirectory  string
	ProcessType       string
}

func launchDaemonPlist(spec launchDaemonSpec) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	writePlistString(&b, "Label", spec.Label)
	writePlistArray(&b, "ProgramArguments", spec.ProgramArguments)
	writePlistBool(&b, "RunAtLoad", spec.RunAtLoad)
	if spec.KeepAlive {
		writePlistBool(&b, "KeepAlive", true)
	}
	if spec.StartInterval > 0 {
		writePlistInteger(&b, "StartInterval", spec.StartInterval)
	}
	writePlistString(&b, "StandardOutPath", spec.StandardOutPath)
	writePlistString(&b, "StandardErrorPath", spec.StandardErrorPath)
	writePlistString(&b, "WorkingDirectory", spec.WorkingDirectory)
	writePlistString(&b, "ProcessType", spec.ProcessType)
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String()
}

func writePlistString(b *strings.Builder, key string, value string) {
	fmt.Fprintf(b, "  <key>%s</key>\n", xmlEscape(key))
	fmt.Fprintf(b, "  <string>%s</string>\n", xmlEscape(value))
}

func writePlistArray(b *strings.Builder, key string, values []string) {
	fmt.Fprintf(b, "  <key>%s</key>\n", xmlEscape(key))
	b.WriteString("  <array>\n")
	for _, value := range values {
		fmt.Fprintf(b, "    <string>%s</string>\n", xmlEscape(value))
	}
	b.WriteString("  </array>\n")
}

func writePlistBool(b *strings.Builder, key string, value bool) {
	fmt.Fprintf(b, "  <key>%s</key>\n", xmlEscape(key))
	if value {
		b.WriteString("  <true/>\n")
		return
	}
	b.WriteString("  <false/>\n")
}

func writePlistInteger(b *strings.Builder, key string, value int) {
	fmt.Fprintf(b, "  <key>%s</key>\n", xmlEscape(key))
	fmt.Fprintf(b, "  <integer>%d</integer>\n", value)
}

func xmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	value = strings.ReplaceAll(value, "'", "&apos;")
	return value
}

func (m LaunchDaemonManager) Load(ctx context.Context, plistPath string) error {
	_, err := m.runLaunchctl(ctx, "bootstrap", m.domain(), plistPath)
	return err
}

func (m LaunchDaemonManager) Unload(ctx context.Context, label string) error {
	_, err := m.runLaunchctl(ctx, "bootout", launchdTarget(m.domain(), label))
	return err
}

func (m LaunchDaemonManager) Status(ctx context.Context, label string) (string, error) {
	out, err := m.runLaunchctl(ctx, "print", launchdTarget(m.domain(), label))
	return string(out), err
}

func (m LaunchDaemonManager) runLaunchctl(ctx context.Context, args ...string) ([]byte, error) {
	run := m.run
	if run == nil {
		run = runLaunchctl
	}
	out, err := run(ctx, args...)
	if err != nil {
		return out, fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (m LaunchDaemonManager) domain() string {
	if m.Domain == "" {
		return "system"
	}
	return m.Domain
}

func launchdTarget(domain string, label string) string {
	return domain + "/" + label
}

func runLaunchctl(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
}
