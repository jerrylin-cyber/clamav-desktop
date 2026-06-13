package main

import (
	"os"
	"path/filepath"
	"strings"
)

type RuntimeSetupStatus struct {
	Ready       bool               `json:"ready"`
	Blocking    bool               `json:"blocking"`
	Message     string             `json:"message"`
	Profile     RuntimeProfile     `json:"profile"`
	Health      RuntimeHealth      `json:"health"`
	Steps       []RuntimeSetupStep `json:"steps"`
	RemoveNotes []string           `json:"removeNotes"`
}

type RuntimeSetupStep struct {
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Command string `json:"command"`
	URL     string `json:"url"`
}

func runtimeSetupStatus() RuntimeSetupStatus {
	profile := runtimeProfile()
	health := runtimeHealth(profile)
	ready := health.Status == "healthy"
	status := RuntimeSetupStatus{
		Ready:    ready,
		Blocking: !ready,
		Message:  runtimeSetupMessage(profile, health, ready),
		Profile:  profile,
		Health:   health,
		Steps:    runtimeSetupSteps(profile, health),
		RemoveNotes: []string{
			"ClamAV Desktop 不再移除 Homebrew 或手動安裝的 ClamAV runtime。",
			"若要移除 Homebrew ClamAV，請先關閉 app，再執行 `brew uninstall clamav`。",
			"使用者資料仍可在 Settings 的「使用者資料」區塊移除。",
		},
	}
	return status
}

func runtimeSetupMessage(profile RuntimeProfile, health RuntimeHealth, ready bool) string {
	if ready {
		return "ClamAV 已可使用。"
	}
	if profile.Source == "not-found" || strings.TrimSpace(profile.FreshclamPath) == "" {
		return "尚未偵測到可用的 ClamAV，請依 Homebrew 步驟安裝。"
	}
	if check := firstFailedRuntimeCheck(health.Checks, "clamd ping", "clamd socket", "clamd commands"); check != nil {
		return "ClamAV binary 已偵測到，但 clamd 尚未正常啟動。"
	}
	return "ClamAV 尚未通過啟動檢測，請完成下列步驟後重新檢測。"
}

func runtimeSetupSteps(profile RuntimeProfile, health RuntimeHealth) []RuntimeSetupStep {
	brewPrefix := homebrewPrefix(profile)
	steps := []RuntimeSetupStep{}
	if brewPrefix == "" && profile.Source == "not-found" {
		brewPrefix = "/opt/homebrew"
	}

	if profile.Source == "not-found" || !isExecutable(filepath.Join(brewPrefix, "bin/brew")) && !isExecutable("/opt/homebrew/bin/brew") && !isExecutable("/usr/local/bin/brew") {
		steps = append(steps, RuntimeSetupStep{
			Title:  "安裝 Homebrew",
			Detail: "ClamAV Desktop 以 Homebrew ClamAV 為主要支援路線。",
			URL:    "https://brew.sh/",
		})
	}

	if !isExecutable(profile.ClamScanPath) || !isExecutable(profile.FreshclamPath) || !isExecutable(profile.ClamdPath) {
		steps = append(steps, RuntimeSetupStep{
			Title:   "安裝 ClamAV",
			Detail:  "安裝 clamscan、freshclam 與 clamd。",
			Command: "brew install clamav",
		})
	}

	if firstFailedRuntimeCheck(health.Checks, "config") != nil {
		steps = append(steps, RuntimeSetupStep{
			Title:   "建立 Homebrew 設定檔",
			Detail:  "Homebrew 安裝後通常只有 .sample，需建立 freshclam.conf 與 clamd.conf。",
			Command: "cp \"$(brew --prefix)/etc/clamav/freshclam.conf.sample\" \"$(brew --prefix)/etc/clamav/freshclam.conf\"\ncp \"$(brew --prefix)/etc/clamav/clamd.conf.sample\" \"$(brew --prefix)/etc/clamav/clamd.conf\"\nperl -0pi -e 's/^Example/#Example/m' \"$(brew --prefix)/etc/clamav/freshclam.conf\" \"$(brew --prefix)/etc/clamav/clamd.conf\"",
		})
	}

	if firstFailedRuntimeCheck(health.Checks, "database") != nil {
		steps = append(steps, RuntimeSetupStep{
			Title:   "建立病毒碼資料庫目錄",
			Detail:  "freshclam 需要可寫入的 database 目錄；app 也會使用使用者資料夾 fallback。",
			Command: "mkdir -p \"$HOME/Library/Application Support/ClamAVDesktop/Database\"",
		})
	}

	if firstFailedRuntimeCheck(health.Checks, "clamd socket", "clamd ping", "clamd commands") != nil {
		databasePath := filepath.Join(userHomeDir(), "Library/Application Support/ClamAVDesktop/Database")
		socketPath := defaultHomebrewSocketPath(brewPrefix)
		logPath := filepath.Join(userHomeDir(), "Library/Logs/ClamAVDesktop/clamd.log")
		steps = append(steps, RuntimeSetupStep{
			Title:  "設定並啟動 clamd",
			Detail: "掃描功能需要 clamd 啟動並提供 Unix socket。",
			Command: "mkdir -p " + shellQuote(filepath.Dir(socketPath)) + "\n" +
				"mkdir -p " + shellQuote(filepath.Dir(logPath)) + "\n" +
				"perl -0pi -e 's|^#?LocalSocket .*|LocalSocket " + socketPath + "|m; s|^#?DatabaseDirectory .*|DatabaseDirectory " + databasePath + "|m; s|^#?LogFile .*|LogFile " + logPath + "|m or $_ .= \"\\nLogFile " + logPath + "\\n\"' \"$(brew --prefix)/etc/clamav/clamd.conf\"\n" +
				"brew services restart clamav",
		})
	}

	steps = append(steps, RuntimeSetupStep{
		Title:   "重新檢測",
		Detail:  "完成步驟後回到 ClamAV Desktop，按下重新檢測。檢測通過後 popup 會自動解除。",
		Command: "",
	})
	return steps
}

func firstFailedRuntimeCheck(checks []RuntimeCheck, names ...string) *RuntimeCheck {
	for _, name := range names {
		for index := range checks {
			check := &checks[index]
			if check.Name == name && check.Status != "ok" {
				return check
			}
		}
	}
	return nil
}

func homebrewPrefix(profile RuntimeProfile) string {
	switch profile.Source {
	case "homebrew-arm64":
		return "/opt/homebrew"
	case "homebrew-x86_64":
		return "/usr/local"
	}
	for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
		if isExecutable(filepath.Join(prefix, "bin/brew")) {
			return prefix
		}
	}
	return ""
}

func defaultHomebrewSocketPath(prefix string) string {
	if prefix == "" {
		prefix = "/opt/homebrew"
	}
	return filepath.Join(prefix, "var/run/clamav/clamd.sock")
}

func userHomeDir() string {
	homeDir, _ := os.UserHomeDir()
	return homeDir
}
