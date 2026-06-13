package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const systemRuntimeBase = "/Library/Application Support/ClamAVDesktop"

var dialUnix = net.DialTimeout

type RuntimeHealth struct {
	Status string         `json:"status"`
	Checks []RuntimeCheck `json:"checks"`
}

type RuntimeCheck struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type RuntimeResolver struct {
	systemBase         string
	homeDir            string
	manualBase         string
	externalCandidates []RuntimeProfile
}

func runtimeProfile() RuntimeProfile {
	homeDir, _ := os.UserHomeDir()
	return RuntimeResolver{
		systemBase: systemRuntimeBase,
		homeDir:    homeDir,
		manualBase: strings.TrimSpace(os.Getenv("CLAMAV_DESKTOP_RUNTIME_PATH")),
	}.Resolve()
}

func (r RuntimeResolver) Resolve() RuntimeProfile {
	for _, candidate := range r.candidateProfiles() {
		if isExecutable(candidate.ClamdPath) || isExecutable(candidate.FreshclamPath) || isExecutable(candidate.ClamScanPath) {
			candidate.Warnings = append(candidate.Warnings, "偵測到外部 ClamAV 執行環境；目前以 Homebrew 為主要支援路線。")
			return candidate
		}
	}

	if r.manualBase != "" {
		manualProfile := externalRuntimeProfile("manual", r.manualBase)
		manualProfile.Mode = "manual"
		if isExecutable(manualProfile.ClamdPath) || isExecutable(manualProfile.FreshclamPath) || isExecutable(manualProfile.ClamScanPath) {
			manualProfile.Warnings = append(manualProfile.Warnings, "偵測到手動指定 ClamAV 執行環境。")
			return manualProfile
		}
	}

	profile := appManagedProfile(r.systemBase)
	if isExecutable(profile.ClamdPath) && isExecutable(profile.FreshclamPath) && isExecutable(profile.ClamScanPath) {
		profile.Mode = "system-shared"
		profile.Warnings = append(profile.Warnings, "偵測到舊版 app-managed runtime；新規格以 Homebrew 為主要支援路線。")
		return profile
	}

	if r.homeDir != "" {
		userProfile := appManagedProfile(filepath.Join(r.homeDir, "Library/Application Support/ClamAVDesktop"))
		if isExecutable(userProfile.ClamdPath) || isExecutable(userProfile.FreshclamPath) || isExecutable(userProfile.ClamScanPath) {
			userProfile.Mode = "per-user"
			userProfile.Source = "per-user-runtime"
			userProfile.Warnings = []string{"偵測到 per-user runtime；新規格以 Homebrew 為主要支援路線。"}
			return userProfile
		}
	}

	profile.Warnings = append(profile.Warnings, "尚未偵測到可用的 ClamAV；請依 Homebrew 引導安裝。")
	profile.Source = "not-found"
	return profile
}

func (r RuntimeResolver) candidateProfiles() []RuntimeProfile {
	if r.externalCandidates != nil {
		return r.externalCandidates
	}
	return externalRuntimeCandidates()
}

func runtimeHealth(profile RuntimeProfile) RuntimeHealth {
	checks := []RuntimeCheck{
		executableCheck("clamscan", profile.ClamScanPath),
		executableCheck("freshclam", profile.FreshclamPath),
		executableCheck("clamd", profile.ClamdPath),
		directoryCheck("config", profile.ConfigPath),
		directoryCheck("database", profile.DatabasePath),
		socketCheck("clamd socket", profile.ClamdSocket),
		clamdCommandCheck("clamd ping", profile.ClamdSocket, "PING", func(reply string) bool {
			return reply == "PONG"
		}),
		clamdCommandCheck("clamd commands", profile.ClamdSocket, "VERSIONCOMMANDS", func(reply string) bool {
			return strings.Contains(reply, "COMMANDS") || strings.Contains(reply, "ClamAV")
		}),
	}

	status := "healthy"
	for _, check := range checks {
		if check.Status == "missing" || check.Status == "unhealthy" {
			status = "repair-required"
			break
		}
	}
	return RuntimeHealth{Status: status, Checks: checks}
}

func appManagedProfile(base string) RuntimeProfile {
	return RuntimeProfile{
		Mode:          "missing",
		ClamScanPath:  filepath.Join(base, "Runtime/bin/clamscan"),
		FreshclamPath: filepath.Join(base, "Runtime/bin/freshclam"),
		ClamdPath:     filepath.Join(base, "Runtime/sbin/clamd"),
		ClamdSocket:   filepath.Join(base, "Run/clamd.sock"),
		DatabasePath:  filepath.Join(base, "Database"),
		ConfigPath:    filepath.Join(base, "Config"),
		Source:        "app-managed-system-shared",
		Warnings:      []string{},
	}
}

func externalRuntimeCandidates() []RuntimeProfile {
	return []RuntimeProfile{
		externalRuntimeProfile("homebrew-arm64", "/opt/homebrew/opt/clamav"),
		externalRuntimeProfile("homebrew-x86_64", "/usr/local/opt/clamav"),
		externalRuntimeProfile("official-pkg", "/usr/local/clamav"),
	}
}

func externalRuntimeProfile(source string, base string) RuntimeProfile {
	profile := RuntimeProfile{
		Mode:          "external",
		ClamScanPath:  filepath.Join(base, "bin/clamscan"),
		FreshclamPath: filepath.Join(base, "bin/freshclam"),
		ClamdPath:     filepath.Join(base, "sbin/clamd"),
		ClamdSocket:   "",
		DatabasePath:  filepath.Join(base, "share/clamav"),
		ConfigPath:    filepath.Join(base, "etc"),
		Source:        source,
		Warnings:      []string{},
	}
	switch source {
	case "homebrew-arm64":
		profile.ConfigPath = "/opt/homebrew/etc/clamav"
		profile.DatabasePath = "/opt/homebrew/var/lib/clamav"
		profile.ClamdSocket = "/opt/homebrew/var/run/clamav/clamd.sock"
	case "homebrew-x86_64":
		profile.ConfigPath = "/usr/local/etc/clamav"
		profile.DatabasePath = "/usr/local/var/lib/clamav"
		profile.ClamdSocket = "/usr/local/var/run/clamav/clamd.sock"
	}
	return profile
}

func executableCheck(name string, path string) RuntimeCheck {
	if isExecutable(path) {
		return RuntimeCheck{Name: name, Path: path, Status: "ok", Message: "可執行"}
	}
	if fileExists(path) {
		return RuntimeCheck{Name: name, Path: path, Status: "unhealthy", Message: "存在但不可執行"}
	}
	return RuntimeCheck{Name: name, Path: path, Status: "missing", Message: "找不到 binary"}
}

func directoryCheck(name string, path string) RuntimeCheck {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return RuntimeCheck{Name: name, Path: path, Status: "ok", Message: "目錄存在"}
	}
	if err == nil {
		return RuntimeCheck{Name: name, Path: path, Status: "unhealthy", Message: "路徑存在但不是目錄"}
	}
	return RuntimeCheck{Name: name, Path: path, Status: "missing", Message: "找不到目錄"}
}

func socketCheck(name string, path string) RuntimeCheck {
	info, err := os.Stat(path)
	if err != nil {
		return RuntimeCheck{Name: name, Path: path, Status: "missing", Message: "找不到 socket"}
	}
	if info.Mode()&os.ModeSocket == 0 {
		return RuntimeCheck{Name: name, Path: path, Status: "unhealthy", Message: "路徑存在但不是 Unix socket"}
	}
	return RuntimeCheck{Name: name, Path: path, Status: "ok", Message: "socket 存在"}
}

func clamdCommandCheck(name string, socketPath string, command string, accepts func(string) bool) RuntimeCheck {
	check := RuntimeCheck{Name: name, Path: socketPath}
	if socketPath == "" {
		check.Status = "missing"
		check.Message = "沒有 clamd socket path"
		return check
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reply, err := ClamDClient{
		SocketPath:  socketPath,
		DialTimeout: 2 * time.Second,
		IOTimeout:   2 * time.Second,
		dial: func(_ context.Context, network string, address string) (net.Conn, error) {
			return dialUnix(network, address, 2*time.Second)
		},
	}.command(ctx, command)
	if err != nil {
		check.Status = "unhealthy"
		check.Message = err.Error()
		return check
	}
	if accepts(reply) {
		check.Status = "ok"
		check.Message = reply
		return check
	}

	check.Status = "unhealthy"
	check.Message = command + " 回應不符預期"
	return check
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0111 != 0
}
