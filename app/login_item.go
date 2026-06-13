package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const loginAgentLabel = "com.lazyjerry.clamavdesktop.agent"

// LoginItemStatus 描述登入啟動項目的目前狀態；Method 標示採用的機制（SMAppService 或 LaunchAgent）。
type LoginItemStatus struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Error   string `json:"error"`
	Method  string `json:"method"`
}

// LoginItemService 管理「登入時啟動」設定，優先使用 SMAppService，失敗時退回 per-user LaunchAgent。
type LoginItemService struct {
	HomeDir    string
	BundlePath string
}

func newLoginItemService(homeDir string) *LoginItemService {
	return &LoginItemService{HomeDir: homeDir}
}

// Apply 依設定登記或取消登入啟動項目：開啟時 Register、關閉時 Unregister。
func (s *LoginItemService) Apply(settings Settings) error {
	if settings.Login.LaunchAtLogin {
		return s.Register(settings.Background.StartHidden)
	}
	return s.Unregister()
}

// Register 登記登入啟動項目；優先用 SMAppService，成功後清掉舊的 LaunchAgent，否則退回寫入 LaunchAgent plist。
func (s *LoginItemService) Register(startHidden bool) error {
	if s.shouldUseSMAppService() {
		if err := registerSMAppService(); err == nil {
			_ = s.unregisterLaunchAgent()
			return nil
		}
	}
	return s.registerLaunchAgent(startHidden)
}

// Unregister 取消登入啟動項目，同時清除 SMAppService 登記與 LaunchAgent plist。
func (s *LoginItemService) Unregister() error {
	if s.shouldUseSMAppService() {
		// 忽略 SMAppService 的錯誤：app 可能從未透過 SMAppService 登記，
		// 或在開發環境中不在正式 bundle 內，此時 unregister 失敗是預期行為
		_ = unregisterSMAppService()
	}
	return s.unregisterLaunchAgent()
}

// Status 回傳目前登入啟動狀態，優先回報 SMAppService，否則檢查 LaunchAgent plist 是否存在。
func (s *LoginItemService) Status() LoginItemStatus {
	if s.shouldUseSMAppService() {
		status := smAppServiceStatus()
		if status.Enabled || status.Error != "" {
			return status
		}
	}

	path := s.plistPath()
	status := LoginItemStatus{Path: path, Method: "LaunchAgent"}
	if path == "" {
		status.Error = "login item plist path 不可為空"
		return status
	}

	if _, err := os.Stat(path); err == nil {
		status.Enabled = true
	} else if !errors.Is(err, os.ErrNotExist) {
		status.Error = err.Error()
	}
	return status
}

func (s *LoginItemService) shouldUseSMAppService() bool {
	return strings.TrimSpace(s.BundlePath) == ""
}

func (s *LoginItemService) registerLaunchAgent(startHidden bool) error {
	plistPath := s.plistPath()
	if plistPath == "" {
		return errors.New("login item plist path 不可為空")
	}

	bundlePath, err := s.appBundlePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0700); err != nil {
		return fmt.Errorf("建立 LaunchAgents 目錄失敗: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(loginAgentPlist(bundlePath, startHidden)), 0600); err != nil {
		return fmt.Errorf("寫入 login item 失敗: %w", err)
	}
	return nil
}

func (s *LoginItemService) unregisterLaunchAgent() error {
	plistPath := s.plistPath()
	if plistPath == "" {
		return errors.New("login item plist path 不可為空")
	}

	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("移除 login item 失敗: %w", err)
	}
	return nil
}

func (s *LoginItemService) plistPath() string {
	homeDir := s.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	if homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, "Library/LaunchAgents/"+loginAgentLabel+".plist")
}

func (s *LoginItemService) appBundlePath() (string, error) {
	if strings.TrimSpace(s.BundlePath) != "" {
		return s.BundlePath, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位 app executable 失敗: %w", err)
	}
	bundlePath, ok := bundlePathForExecutable(executable)
	if !ok {
		return "", fmt.Errorf("目前執行檔不在 .app bundle 中，無法註冊 login item: %s", executable)
	}
	return bundlePath, nil
}

func bundlePathForExecutable(executable string) (string, bool) {
	cleaned := filepath.Clean(executable)
	marker := ".app/Contents/MacOS/"
	index := strings.Index(cleaned, marker)
	if index == -1 {
		return "", false
	}
	return cleaned[:index+len(".app")], true
}

func loginAgentPlist(bundlePath string, startHidden bool) string {
	args := []string{"/usr/bin/open", "-g"}
	if startHidden {
		args = append(args, "-j")
	}
	args = append(args, bundlePath)

	return plistDocument(map[string]any{
		"Label":            loginAgentLabel,
		"ProgramArguments": args,
		"RunAtLoad":        true,
	})
}

func plistDocument(values map[string]any) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")

	writePlistKeyString(&b, "Label", values["Label"].(string))
	writePlistKeyArray(&b, "ProgramArguments", values["ProgramArguments"].([]string))
	writePlistKeyBool(&b, "RunAtLoad", values["RunAtLoad"].(bool))

	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func writePlistKeyString(b *strings.Builder, key string, value string) {
	fmt.Fprintf(b, "\t<key>%s</key>\n\t<string>%s</string>\n", plistEscape(key), plistEscape(value))
}

func writePlistKeyArray(b *strings.Builder, key string, values []string) {
	fmt.Fprintf(b, "\t<key>%s</key>\n\t<array>\n", plistEscape(key))
	for _, value := range values {
		fmt.Fprintf(b, "\t\t<string>%s</string>\n", plistEscape(value))
	}
	b.WriteString("\t</array>\n")
}

func writePlistKeyBool(b *strings.Builder, key string, value bool) {
	fmt.Fprintf(b, "\t<key>%s</key>\n", plistEscape(key))
	if value {
		b.WriteString("\t<true/>\n")
	} else {
		b.WriteString("\t<false/>\n")
	}
}

func plistEscape(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
