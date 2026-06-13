package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "1.0.0"

type App struct {
	ctx                context.Context
	settingsStore      SettingsStore
	freshclamService   *FreshclamService
	scanJobManager     *ScanJobManager
	fileActions        *FileActionService
	logService         *LogService
	loginItemService   *LoginItemService
	systemSettings     *SystemSettingsService
	powerPolicyService *PowerPolicyService
	schedulerService   *SchedulerService
	updateScheduler    *UpdateSchedulerService
	selectFilesDialog  filesDialogRunner
	selectFolderDialog folderDialogRunner

	serviceMu sync.Mutex

	quitting atomic.Bool

	backgroundMu             sync.Mutex
	backgroundCancel         context.CancelFunc
	backgroundWake           chan struct{}
	backgroundTickInterval   time.Duration
	lastScheduledScanRun     time.Time
	lastScheduledDatabaseRun time.Time
	scheduledWorkMu          sync.Mutex
}

func NewApp() *App {
	return &App{settingsStore: defaultSettingsStore()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if settings, err := a.settingsStore.Load(); err == nil {
		if settings.Background.KeepMenuBarIcon {
			a.startStatusItem()
		}
		if settings.Background.StartHidden {
			go func() {
				time.Sleep(250 * time.Millisecond)
				wailsruntime.WindowHide(ctx)
			}()
		}
	}
	a.startBackgroundWorker(ctx)
}

// beforeClose 在使用者關閉視窗時觸發。回傳 true 會阻止 app 結束。
// 當「保留狀態列圖示」開啟時，關閉視窗改為隱藏視窗並保留 process，
// 讓 menu bar 圖示、背景排程與執行中的掃描都能持續運作；只有 menu bar 的「結束」才真正退出。
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return false
	}
	if settings.Background.KeepMenuBarIcon {
		a.startStatusItem()
		wailsruntime.WindowHide(ctx)
		return true
	}
	return false
}

func (a *App) shutdown(ctx context.Context) {
	a.stopBackgroundWorker()
}

type RuntimeProfile struct {
	Mode          string   `json:"mode"`
	ClamScanPath  string   `json:"clamScanPath"`
	FreshclamPath string   `json:"freshclamPath"`
	ClamdPath     string   `json:"clamdPath"`
	ClamdSocket   string   `json:"clamdSocket"`
	DatabasePath  string   `json:"databasePath"`
	ConfigPath    string   `json:"configPath"`
	Source        string   `json:"source"`
	Warnings      []string `json:"warnings"`
}

type AppStatus struct {
	Runtime  RuntimeProfile `json:"runtime"`
	Health   RuntimeHealth  `json:"health"`
	Database DatabaseStatus `json:"database"`
	Pages    []string       `json:"pages"`
}

type AboutInfo struct {
	Version     string          `json:"version"`
	Computer    ComputerInfo    `json:"computer"`
	Runtime     RuntimeProfile  `json:"runtime"`
	Database    DatabaseStatus  `json:"database"`
	Paths       AboutPaths      `json:"paths"`
	Commands    []CommandInfo   `json:"commands"`
	OfficialURL string          `json:"officialUrl"`
	GitHubURL   string          `json:"githubUrl"`
	Features    []FeatureStatus `json:"features"`
}

type ComputerInfo struct {
	Hostname string `json:"hostname"`
	HomeDir  string `json:"homeDir"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

type AboutPaths struct {
	ClamScan        string `json:"clamScan"`
	Freshclam       string `json:"freshclam"`
	Clamd           string `json:"clamd"`
	ClamdSocket     string `json:"clamdSocket"`
	RuntimeConfig   string `json:"runtimeConfig"`
	FreshclamConfig string `json:"freshclamConfig"`
	Database        string `json:"database"`
	Quarantine      string `json:"quarantine"`
	Settings        string `json:"settings"`
	Logs            string `json:"logs"`
}

type CommandInfo struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

type FeatureStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (a *App) GetAppStatus() AppStatus {
	profile := runtimeProfile()
	database, err := newFreshclamService(profile).LoadStatus()
	if err != nil {
		database.Error = err.Error()
	}

	return AppStatus{
		Runtime:  profile,
		Health:   runtimeHealth(profile),
		Database: database,
		Pages: []string{
			"儀表板",
			"掃描",
			"結果",
			"排程",
			"隔離區",
			"設定",
			"紀錄",
			"關於",
		},
	}
}

func (a *App) GetRuntimeSetupStatus() RuntimeSetupStatus {
	return runtimeSetupStatus()
}

func (a *App) GetAboutInfo() AboutInfo {
	homeDir, _ := os.UserHomeDir()
	hostname, _ := os.Hostname()
	profile := runtimeProfile()
	freshclam := newFreshclamService(profile)
	database, _ := freshclam.LoadStatus()
	paths := userDataPathsForHome(homeDir)
	freshclamConfig := freshclam.configuredFreshclamConfigPath()
	if !fileExists(freshclamConfig) {
		freshclamConfig = freshclam.generatedFreshclamConfigPath()
	}

	return AboutInfo{
		Version: appVersion,
		Computer: ComputerInfo{
			Hostname: hostname,
			HomeDir:  homeDir,
			OS:       goruntime.GOOS,
			Arch:     goruntime.GOARCH,
		},
		Runtime:  profile,
		Database: database,
		Paths: AboutPaths{
			ClamScan:        profile.ClamScanPath,
			Freshclam:       profile.FreshclamPath,
			Clamd:           profile.ClamdPath,
			ClamdSocket:     profile.ClamdSocket,
			RuntimeConfig:   profile.ConfigPath,
			FreshclamConfig: freshclamConfig,
			Database:        database.Path,
			Quarantine:      paths.Quarantine,
			Settings:        paths.Settings,
			Logs:            paths.Logs,
		},
		Commands: []CommandInfo{
			{Label: "查看版本", Command: shellQuote(profile.ClamScanPath) + " --version"},
			{Label: "更新病毒碼", Command: shellQuote(profile.FreshclamPath) + " --foreground --config-file=" + shellQuote(freshclamConfig)},
			{Label: "掃描資料夾", Command: shellQuote(profile.ClamScanPath) + " --database=" + shellQuote(database.Path) + " --recursive " + shellQuote(filepath.Join(homeDir, "Downloads"))},
			{Label: "檢查 clamd", Command: "printf 'zPING\\0' | nc -U " + shellQuote(profile.ClamdSocket)},
		},
		OfficialURL: "https://www.clamav.net/",
		GitHubURL:   "https://github.com/lazyjerry/clamav-desktop",
		Features: []FeatureStatus{
			{Name: "病毒碼更新", Status: "可用", Note: "支援 app-managed config；外部執行環境會 fallback 到使用者可寫的 freshclam config。"},
			{Name: "單次掃描", Status: "可用", Note: "透過 clamd INSTREAM 執行掃描工作。"},
			{Name: "排程掃描", Status: "可用", Note: "Settings 與 Schedule 共用同一份設定，變更後會喚醒背景 worker。"},
			{Name: "隔離區", Status: "可用", Note: "感染檔案可隔離、還原、移到垃圾桶或永久刪除。"},
			{Name: "Runtime 引導", Status: "可用", Note: "以 Homebrew ClamAV 為主要支援路線；未通過檢測時顯示 blocking setup popup。"},
			{Name: "Status bar", Status: "可用", Note: "已支援 NSStatusItem 狀態列選單。"},
			{Name: "登入時啟動", Status: "可用", Note: "優先使用 SMAppService，失敗時 fallback per-user LaunchAgent。"},
		},
	}
}

func (a *App) GetSettings() (Settings, error) {
	return a.settingsStore.Load()
}

func shellQuote(value string) string {
	if strings.TrimSpace(value) == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (a *App) SaveSettings(settings Settings) (Settings, error) {
	current, err := a.settingsStore.Load()
	if err != nil {
		return Settings{}, err
	}
	if loginItemSettingsChanged(current, settings) {
		if err := a.loginItems().Apply(settings); err != nil {
			return Settings{}, err
		}
	}
	if err := a.settingsStore.Save(settings); err != nil {
		return Settings{}, err
	}
	saved, err := a.settingsStore.Load()
	if err != nil {
		return Settings{}, err
	}
	a.wakeBackgroundWorker()
	return saved, nil
}

func loginItemSettingsChanged(current Settings, next Settings) bool {
	return current.Login.LaunchAtLogin != next.Login.LaunchAtLogin ||
		current.Background.StartHidden != next.Background.StartHidden
}

func (a *App) GetLoginItemStatus() LoginItemStatus {
	return a.loginItems().Status()
}

func (a *App) OpenFullDiskAccessSettings() error {
	return a.systemSettingsService().OpenFullDiskAccess(a.context())
}

func (a *App) OpenNotificationSettings() error {
	return a.systemSettingsService().OpenNotifications(a.context())
}

func (a *App) GetDatabaseStatus() (DatabaseStatus, error) {
	return a.freshclam().LoadStatus()
}

func (a *App) UpdateDatabase() (DatabaseStatus, error) {
	status, err := a.freshclam().UpdateDatabase(a.context(), func(event FreshclamEvent) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "freshclam:event", event)
		}
	})
	if err != nil {
		_ = a.logs().WriteAppLog("error", "病毒碼更新失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "病毒碼更新完成")
	}
	return status, err
}

func (a *App) StartScan(paths []string, options ScanOptions) (ScanJob, error) {
	job, _, err := a.scanJobs().RunScan(a.context(), paths, options, func(event ScanProgressEvent) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "scan:progress", event)
		}
	})
	if err != nil {
		_ = a.logs().WriteAppLog("error", "掃描失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", fmt.Sprintf("掃描完成：狀態 %s，掃描 %d 筆，偵測 %d 筆，錯誤 %d 筆", job.Status, job.ScannedFiles, job.Detections, job.Errors))
	}
	return job, err
}

func (a *App) CancelScanJob(id string) bool {
	canceled := a.scanJobs().CancelScanJob(id)
	if canceled {
		_ = a.logs().WriteAppLog("info", "已取消掃描："+id)
	}
	return canceled
}

func (a *App) ListScanJobs() ([]ScanJob, error) {
	return a.scanJobs().ListScanJobs()
}

func (a *App) GetScanJob(id string) (ScanJob, error) {
	return a.scanJobs().GetScanJob(id)
}

func (a *App) LoadScanResults(id string) ([]ScanResult, error) {
	return a.scanJobs().LoadResults(id)
}

func (a *App) GetDownloadsPath() string {
	return downloadsPath()
}

type ScanPathPreset struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

func (a *App) GetCommonScanPaths() []ScanPathPreset {
	return commonScanPaths()
}

func (a *App) OpenScanResultLocation(result ScanResult) error {
	return a.fileActionService().OpenScanResultLocation(a.context(), result)
}

func (a *App) QuarantineScanResult(result ScanResult) (QuarantineRecord, error) {
	record, err := a.fileActionService().Quarantine(result)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "隔離失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已隔離："+filepath.Base(result.Path))
	}
	return record, err
}

func (a *App) RestoreQuarantineRecord(id string) (QuarantineRecord, error) {
	record, err := a.fileActionService().Restore(id)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "還原失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已還原："+filepath.Base(record.OriginalPath))
	}
	return record, err
}

func (a *App) OpenQuarantineLocation(record QuarantineRecord) error {
	return a.fileActionService().OpenQuarantineLocation(a.context(), record)
}

func (a *App) ListQuarantineRecords() ([]QuarantineRecord, error) {
	return a.fileActionService().ListQuarantineRecords()
}

func (a *App) MoveQuarantineRecordToTrash(id string) (QuarantineRecord, error) {
	record, err := a.fileActionService().MoveQuarantineToTrash(a.context(), id)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "移到垃圾桶失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已將隔離項目移到垃圾桶："+filepath.Base(record.OriginalPath))
	}
	return record, err
}

func (a *App) PermanentlyDeleteQuarantineRecord(id string) (QuarantineRecord, error) {
	record, err := a.fileActionService().PermanentlyDeleteQuarantine(id)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "永久刪除失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已永久刪除隔離項目："+filepath.Base(record.OriginalPath))
	}
	return record, err
}

func (a *App) MoveScanResultToTrash(result ScanResult) error {
	err := a.fileActionService().MoveToTrash(a.context(), result.Path)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "移到垃圾桶失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已移到垃圾桶："+filepath.Base(result.Path))
	}
	return err
}

func (a *App) PermanentlyDeleteScanResult(result ScanResult) error {
	err := a.fileActionService().PermanentlyDelete(result.Path)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "永久刪除失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已永久刪除："+filepath.Base(result.Path))
	}
	return err
}

// ListAppLogEntries returns the most recent per-user app log entries, newest
// first.
func (a *App) ListAppLogEntries(limit int) ([]LogEntry, error) {
	return a.logs().ListAppLogEntries(limit)
}

// ReadFreshclamLog returns the most recent lines of the shared freshclam.log.
func (a *App) ReadFreshclamLog(limit int) []string {
	return a.logs().ReadSharedLog("freshclam.log", limit)
}

// ReadClamdLog returns the most recent lines of the shared clamd.log.
func (a *App) ReadClamdLog(limit int) []string {
	return a.logs().ReadSharedLog("clamd.log", limit)
}

// ExportDiagnostics writes a combined diagnostics report covering runtime
// status, recent scans, the app log, and shared service logs, then reveals
// it in Finder.
func (a *App) ExportDiagnostics() (string, error) {
	return a.logs().ExportDiagnostics(a.context(), a.buildDiagnosticsReport())
}

func (a *App) buildDiagnosticsReport() string {
	var b strings.Builder
	now := time.Now().UTC()
	fmt.Fprintf(&b, "ClamAV Desktop 診斷報告\n產生時間：%s\n\n", now.Format(time.RFC3339))

	status := a.GetAppStatus()
	fmt.Fprintf(&b, "== Runtime ==\nMode: %s\nSource: %s\nWarnings: %s\n\n",
		status.Runtime.Mode, status.Runtime.Source, strings.Join(status.Runtime.Warnings, "; "))

	fmt.Fprintf(&b, "== Health ==\nStatus: %s\n\n", status.Health.Status)

	fmt.Fprintf(&b, "== Database ==\nLastUpdated: %s\nVersion: %s\nSignatures: %d\n\n",
		status.Database.LastUpdated.Format(time.RFC3339), status.Database.Version, status.Database.Signatures)

	b.WriteString("== 最近掃描 ==\n")
	jobs, _ := a.scanJobs().ListScanJobs()
	if len(jobs) == 0 {
		b.WriteString("（無）\n")
	} else {
		if len(jobs) > 10 {
			jobs = jobs[:10]
		}
		for _, job := range jobs {
			fmt.Fprintf(&b, "%s 狀態：%s 已掃描：%d 偵測：%d 錯誤：%d\n",
				job.StartedAt.Format(time.RFC3339), job.Status, job.ScannedFiles, job.Detections, job.Errors)
		}
	}
	b.WriteString("\n")

	b.WriteString("== App Log（最近 50 筆）==\n")
	entries, _ := a.logs().ListAppLogEntries(50)
	if len(entries) == 0 {
		b.WriteString("（無）\n")
	} else {
		for _, entry := range entries {
			fmt.Fprintf(&b, "%s [%s] %s\n", entry.At.Format(time.RFC3339), entry.Level, entry.Message)
		}
	}
	b.WriteString("\n")

	writeLogSection(&b, "freshclam.log（最近 100 行）", a.logs().ReadSharedLog("freshclam.log", 100))
	writeLogSection(&b, "clamd.log（最近 100 行）", a.logs().ReadSharedLog("clamd.log", 100))

	return b.String()
}

func writeLogSection(b *strings.Builder, title string, lines []string) {
	fmt.Fprintf(b, "== %s ==\n", title)
	if len(lines) == 0 {
		b.WriteString("（無）\n")
		return
	}
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func (a *App) freshclam() *FreshclamService {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.freshclamService != nil {
		return a.freshclamService
	}
	a.freshclamService = newFreshclamService(runtimeProfile())
	return a.freshclamService
}

func (a *App) scanJobs() *ScanJobManager {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.scanJobManager != nil {
		return a.scanJobManager
	}
	homeDir, _ := os.UserHomeDir()
	profile := runtimeProfile()
	a.scanJobManager = newScanJobManager(homeDir, ClamDClient{SocketPath: profile.ClamdSocket, IOTimeout: 5 * time.Minute})
	return a.scanJobManager
}

func (a *App) fileActionService() *FileActionService {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.fileActions != nil {
		return a.fileActions
	}
	homeDir, _ := os.UserHomeDir()
	a.fileActions = newFileActionService(homeDir)
	return a.fileActions
}

func (a *App) logs() *LogService {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.logService != nil {
		return a.logService
	}
	homeDir, _ := os.UserHomeDir()
	a.logService = newLogService(homeDir)
	return a.logService
}

func (a *App) loginItems() *LoginItemService {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.loginItemService != nil {
		return a.loginItemService
	}
	homeDir, _ := os.UserHomeDir()
	a.loginItemService = newLoginItemService(homeDir)
	return a.loginItemService
}

func (a *App) systemSettingsService() *SystemSettingsService {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.systemSettings != nil {
		return a.systemSettings
	}
	a.systemSettings = newSystemSettingsService()
	return a.systemSettings
}

func (a *App) powerPolicy() *PowerPolicyService {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.powerPolicyService != nil {
		return a.powerPolicyService
	}
	a.powerPolicyService = newPowerPolicyService()
	return a.powerPolicyService
}

func (a *App) scheduler() *SchedulerService {
	a.serviceMu.Lock()
	service := a.schedulerService
	a.serviceMu.Unlock()
	if service != nil {
		return service
	}

	service = newSchedulerService(a.scanJobs(), a.powerPolicy())
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.schedulerService == nil {
		a.schedulerService = service
	}
	return a.schedulerService
}

func (a *App) updateSchedulerService() *UpdateSchedulerService {
	a.serviceMu.Lock()
	service := a.updateScheduler
	a.serviceMu.Unlock()
	if service != nil {
		return service
	}

	service = newUpdateSchedulerService(a.freshclam(), a.powerPolicy())
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.updateScheduler == nil {
		a.updateScheduler = service
	}
	return a.updateScheduler
}

func downloadsPath() string {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, "Downloads")
}

func commonScanPaths() []ScanPathPreset {
	homeDir, _ := os.UserHomeDir()
	candidates := []ScanPathPreset{}
	if homeDir != "" {
		candidates = append(candidates,
			ScanPathPreset{Label: "家目錄", Path: homeDir},
			ScanPathPreset{Label: "桌面", Path: filepath.Join(homeDir, "Desktop")},
			ScanPathPreset{Label: "下載", Path: filepath.Join(homeDir, "Downloads")},
		)
	}
	candidates = append(candidates,
		ScanPathPreset{Label: "暫存", Path: "/tmp"},
		ScanPathPreset{Label: "全系統", Path: "/"},
	)

	presets := make([]ScanPathPreset, 0, len(candidates))
	for _, preset := range candidates {
		if preset.Path == "" {
			continue
		}
		if _, err := os.Stat(preset.Path); err != nil {
			continue
		}
		presets = append(presets, preset)
	}
	return presets
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
