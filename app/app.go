package main

import (
	"context"
	"encoding/xml"
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

const appVersion = "1.1.3"

// App 是 Wails 應用的主結構，統籌各服務（設定、掃描、病毒碼更新、隔離、登入項、排程等），
// 並對外暴露給前端呼叫的 binding 方法。
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
	messageDialog      messageDialogRunner
	hideWindow         windowHideRunner
	startStatusItemRun func()

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

// NewApp 建立 App 實例，並初始化預設的設定儲存位置。
func NewApp() *App {
	return &App{settingsStore: defaultSettingsStore()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 上次掃描若因 app 關閉而未正常結束，狀態會卡在 queued/scanning；啟動時標記為 interrupted，
	// 讓前端能顯示「已中斷」並提供重新掃描，已逐批寫入的結果仍保留在結果庫。
	if marked, err := a.scanJobs().MarkInterruptedJobs(); err == nil && marked > 0 {
		_ = a.logs().WriteAppLog("info", fmt.Sprintf("已將 %d 筆未完成的掃描標記為中斷", marked))
	}
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

type messageDialogRunner func(ctx context.Context, options wailsruntime.MessageDialogOptions) (string, error)

type windowHideRunner func(ctx context.Context)

// beforeClose 在使用者關閉視窗或按下 Cmd+Q 時觸發。回傳 true 會阻止 app 結束。
// 當「保留狀態列圖示」開啟時，確認關閉後改為隱藏視窗並保留 process，
// 讓 menu bar 圖示、背景排程與執行中的掃描都能持續運作；只有 menu bar 的「結束」才真正退出。
func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return false
	}
	if !a.confirmClose(ctx) {
		return true
	}
	// 掃描進行中時不結束 process，改為隱藏到 menu bar 讓掃描在背景繼續，避免中途中斷與結果遺失。
	if a.scanJobs().HasRunningJobs() {
		a.startStatusItem()
		a.showScanningCloseNotice(ctx)
		a.windowHide(ctx)
		return true
	}
	if settings.Background.KeepMenuBarIcon {
		a.startStatusItem()
		a.showBackgroundCloseNotice(ctx)
		a.windowHide(ctx)
		return true
	}
	return false
}

func (a *App) confirmClose(ctx context.Context) bool {
	selected, err := a.runMessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.QuestionDialog,
		Title:         "是否關閉",
		Message:       "要關閉 ClamAV Desktop 嗎？",
		Buttons:       []string{"關閉", "取消"},
		DefaultButton: "取消",
		CancelButton:  "取消",
	})
	if err != nil {
		return true
	}
	return dialogSelectionAllowsClose(selected)
}

func (a *App) showScanningCloseNotice(ctx context.Context) {
	_, _ = a.runMessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.InfoDialog,
		Title:         "掃描將在背景繼續",
		Message:       "目前有掃描正在進行，ClamAV Desktop 會保留在 menu bar 讓掃描繼續完成；要完全結束請從 menu bar 選擇「結束」。",
		Buttons:       []string{"知道了"},
		DefaultButton: "知道了",
	})
}

func (a *App) showBackgroundCloseNotice(ctx context.Context) {
	_, _ = a.runMessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.InfoDialog,
		Title:         "仍然會在背景運作",
		Message:       "ClamAV Desktop 會保留在 menu bar，背景排程與執行中的掃描會繼續運作。",
		Buttons:       []string{"知道了"},
		DefaultButton: "知道了",
	})
}

func dialogSelectionAllowsClose(selected string) bool {
	switch strings.ToLower(strings.TrimSpace(selected)) {
	case "關閉", "yes", "ok":
		return true
	default:
		return false
	}
}

func (a *App) runMessageDialog(ctx context.Context, options wailsruntime.MessageDialogOptions) (string, error) {
	run := a.messageDialog
	if run == nil {
		run = wailsruntime.MessageDialog
	}
	return run(ctx, options)
}

func (a *App) windowHide(ctx context.Context) {
	run := a.hideWindow
	if run == nil {
		run = wailsruntime.WindowHide
	}
	run(ctx)
}

func (a *App) shutdown(ctx context.Context) {
	a.stopBackgroundWorker()
}

// RuntimeProfile 描述目前採用的 ClamAV 執行環境：各執行檔/設定/socket 路徑、來源與警告。
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

// AppStatus 彙整前端儀表板所需的整體狀態：執行環境、健康檢查、病毒碼狀態與頁面清單。
type AppStatus struct {
	Runtime  RuntimeProfile `json:"runtime"`
	Health   RuntimeHealth  `json:"health"`
	Database DatabaseStatus `json:"database"`
	Pages    []string       `json:"pages"`
}

// AboutInfo 提供「關於」頁所需資訊：版本、機器、執行環境、各項路徑、可複製指令與功能清單。
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

// ComputerInfo 描述執行本 App 的機器資訊（主機名稱、家目錄、作業系統與架構）。
type ComputerInfo struct {
	Hostname string `json:"hostname"`
	HomeDir  string `json:"homeDir"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// AboutPaths 集中列出「關於」頁要顯示的各項檔案與目錄路徑。
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

// CommandInfo 為「關於」頁提供的一條可複製的等效 CLI 指令與其標籤。
type CommandInfo struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

// FeatureStatus 描述功能清單中的單一功能：名稱、狀態與說明。
type FeatureStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

// GetAppStatus 回傳前端儀表板所需的整體狀態（執行環境、健康檢查、病毒碼狀態與頁面清單）。
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

// GetRuntimeSetupStatus 回傳安裝/啟動引導的目前狀態，供前端決定是否顯示引導視窗。
func (a *App) GetRuntimeSetupStatus() RuntimeSetupStatus {
	return runtimeSetupStatus()
}

// GetAboutInfo 組出「關於」頁所需的版本、機器、路徑、等效指令與功能清單。
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
			{Label: "掃描資料夾（以下載資料夾為例）", Command: shellQuote(profile.ClamScanPath) + " --database=" + shellQuote(database.Path) + " --recursive \"$HOME/Downloads\""},
			{Label: "建立每日掃描排程", Command: scheduledClamScanCommand(profile.ClamScanPath, database.Path, homeDir)},
			{Label: "檢查 clamd", Command: "printf 'zPING\\0' | nc -U " + shellQuote(profile.ClamdSocket)},
		},
		OfficialURL: "https://www.clamav.net/",
		GitHubURL:   "https://github.com/lazyjerry/clamav-desktop",
		Features: []FeatureStatus{
			{Name: "病毒碼更新", Status: "可用", Note: "手動或排程向 ClamAV 官方同步最新病毒碼；App 內建 freshclam 設定，外部執行環境會自動改用使用者可寫的 freshclam 設定。"},
			{Name: "單次掃描", Status: "可用", Note: "選擇檔案或資料夾即時掃描，透過 clamd 的 INSTREAM 串流送檢，超過串流上限的大檔會自動改以路徑掃描。"},
			{Name: "排程掃描", Status: "可用", Note: "可設定每日／每週自動掃描指定路徑；設定頁與排程頁共用同一份設定，變更後立即喚醒背景 worker 生效。"},
			{Name: "隔離區", Status: "可用", Note: "將感染檔案移出原位置並以 XOR 編碼存放，避免惡意內容以原始形態留存於磁碟；可隨時還原、移到垃圾桶或永久刪除。"},
			{Name: "Runtime 引導", Status: "可用", Note: "以 Homebrew 版 ClamAV 為主要安裝路線；偵測到執行環境未就緒時，會跳出引導視窗協助完成安裝與啟動。"},
			{Name: "Status bar", Status: "可用", Note: "在 macOS 選單列提供狀態列圖示與快速選單（NSStatusItem），方便背景常駐時操作與查看狀態。"},
			{Name: "登入時啟動", Status: "可用", Note: "登入 macOS 時自動啟動以維持背景排程運作；優先使用系統 SMAppService，失敗時改用 per-user LaunchAgent。"},
		},
	}
}

// GetSettings 讀取目前的使用者設定供前端顯示。
func (a *App) GetSettings() (Settings, error) {
	return a.settingsStore.Load()
}

func shellQuote(value string) string {
	if strings.TrimSpace(value) == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func scheduledClamScanCommand(clamScanPath string, databasePath string, homeDir string) string {
	plistPath := filepath.Join(homeDir, "Library/LaunchAgents/com.lazyjerry.clamavdesktop.clamscan-downloads.plist")
	logDir := filepath.Join(homeDir, "Library/Logs/ClamAVDesktop")
	outPath := filepath.Join(logDir, "clamscan-schedule.log")
	errPath := filepath.Join(logDir, "clamscan-schedule.err.log")
	scanPath := filepath.Join(homeDir, "Downloads")

	return strings.Join([]string{
		"mkdir -p " + shellQuote(filepath.Dir(plistPath)) + " " + shellQuote(logDir) + " && cat > " + shellQuote(plistPath) + " <<'PLIST'",
		scheduledClamScanPlist(clamScanPath, databasePath, scanPath, outPath, errPath),
		"PLIST",
		"launchctl unload " + shellQuote(plistPath) + " 2>/dev/null; launchctl load " + shellQuote(plistPath),
	}, "\n")
}

func scheduledClamScanPlist(clamScanPath string, databasePath string, scanPath string, outPath string, errPath string) string {
	return strings.Join([]string{
		xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
		`<plist version="1.0">`,
		`<dict>`,
		`	<key>Label</key>`,
		`	<string>com.lazyjerry.clamavdesktop.clamscan-downloads</string>`,
		`	<key>ProgramArguments</key>`,
		`	<array>`,
		`		<string>` + plistEscape(clamScanPath) + `</string>`,
		`		<string>--database=` + plistEscape(databasePath) + `</string>`,
		`		<string>--recursive</string>`,
		`		<string>` + plistEscape(scanPath) + `</string>`,
		`	</array>`,
		`	<key>StartCalendarInterval</key>`,
		`	<dict>`,
		`		<key>Hour</key>`,
		`		<integer>12</integer>`,
		`		<key>Minute</key>`,
		`		<integer>0</integer>`,
		`	</dict>`,
		`	<key>StandardOutPath</key>`,
		`	<string>` + plistEscape(outPath) + `</string>`,
		`	<key>StandardErrorPath</key>`,
		`	<string>` + plistEscape(errPath) + `</string>`,
		`</dict>`,
		`</plist>`,
		"",
	}, "\n")
}

// SaveSettings 儲存設定並套用相關副作用（登入啟動項、喚醒背景排程 worker），回傳正規化後的設定。
func (a *App) SaveSettings(settings Settings) (Settings, error) {
	current, err := a.settingsStore.Load()
	if err != nil {
		_ = a.logs().WriteAppLog("error", "儲存設定失敗（讀取現有設定）："+err.Error())
		return Settings{}, err
	}
	if loginItemSettingsChanged(current, settings) {
		if err := a.loginItems().Apply(settings); err != nil {
			_ = a.logs().WriteAppLog("error", "儲存設定失敗（套用 login item）："+err.Error())
			return Settings{}, err
		}
	}
	if err := a.settingsStore.Save(settings); err != nil {
		_ = a.logs().WriteAppLog("error", "儲存設定失敗（寫入檔案）："+err.Error())
		return Settings{}, err
	}
	saved, err := a.settingsStore.Load()
	if err != nil {
		_ = a.logs().WriteAppLog("error", "儲存設定失敗（驗證回讀）："+err.Error())
		return Settings{}, err
	}
	_ = a.logs().WriteAppLog("info", "設定已儲存")
	a.wakeBackgroundWorker()
	return saved, nil
}

func loginItemSettingsChanged(current Settings, next Settings) bool {
	if current.Login.LaunchAtLogin != next.Login.LaunchAtLogin {
		return true
	}
	// startHidden 只影響 LaunchAgent plist 的 -j flag，
	// 只有在登入時啟動已開啟時，變更才需要重新套用 login item
	return next.Login.LaunchAtLogin && current.Background.StartHidden != next.Background.StartHidden
}

// GetLoginItemStatus 回傳目前「登入時啟動」的狀態。
func (a *App) GetLoginItemStatus() LoginItemStatus {
	return a.loginItems().Status()
}

// OpenFullDiskAccessSettings 開啟「系統設定 → 完整磁碟取用權限」面板。
func (a *App) OpenFullDiskAccessSettings() error {
	return a.systemSettingsService().OpenFullDiskAccess(a.context())
}

// OpenNotificationSettings 開啟 macOS「系統設定 → 通知」頁面，供使用者手動調整本 App 的通知權限。
// 作為 Wails binding 供前端呼叫；前端目前暫時隱藏通知 UI（未簽章 build 下通知無法穩定運作），但保留此 binding 以便日後恢復。
func (a *App) OpenNotificationSettings() error {
	return a.systemSettingsService().OpenNotifications(a.context())
}

// GetSystemPermissionStatus 回傳 App 關注的 macOS 權限檢測結果。
func (a *App) GetSystemPermissionStatus() SystemPermissionStatus {
	return a.systemSettingsService().PermissionStatus()
}

// GetDatabaseStatus 回傳目前病毒碼資料庫的狀態。
func (a *App) GetDatabaseStatus() (DatabaseStatus, error) {
	return a.freshclam().LoadStatus()
}

// UpdateDatabase 立即執行病毒碼更新並回傳更新後狀態；過程與結果寫入 App log。
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

// StartScan 對指定路徑啟動一次掃描工作，過程以 Wails 事件推播進度，回傳新建立的工作。
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

// CancelScanJob 取消執行中的掃描工作，回傳是否確實取消了對應工作。
func (a *App) CancelScanJob(id string) bool {
	canceled := a.scanJobs().CancelScanJob(id)
	if canceled {
		_ = a.logs().WriteAppLog("info", "已取消掃描："+id)
	}
	return canceled
}

// ListScanJobs 回傳所有已保存的掃描工作紀錄。
func (a *App) ListScanJobs() ([]ScanJob, error) {
	return a.scanJobs().ListScanJobs()
}

// GetScanJob 依 ID 讀取單一掃描工作。
func (a *App) GetScanJob(id string) (ScanJob, error) {
	return a.scanJobs().GetScanJob(id)
}

// LoadScanResults 依工作 ID 讀取該次掃描的逐檔結果。
func (a *App) LoadScanResults(id string) ([]ScanResult, error) {
	return a.scanJobs().LoadResults(id)
}

// LoadScanResultsPage 依工作 ID 取得一頁結果（後端分頁，避免一次把全部結果傳給前端）。
// status 可為空字串或 "all" 表示不篩選；query 對路徑與病毒碼名稱做模糊搜尋。
func (a *App) LoadScanResultsPage(id, status, query string, offset, limit int) (ScanResultsPage, error) {
	if limit <= 0 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	return a.scanJobs().LoadResultsPage(id, status, query, offset, limit)
}

// MarkScanResultStatus 在隔離／移到垃圾桶／永久刪除等動作成功後，同步更新結果庫中該筆結果的狀態，
// 讓後續分頁查詢反映最新狀態。
func (a *App) MarkScanResultStatus(jobID, path, status string) error {
	return a.scanJobs().results.UpdateStatus(jobID, path, status)
}

// GetDownloadsPath 回傳目前使用者的下載資料夾路徑，供前端作為掃描預設目標。
func (a *App) GetDownloadsPath() string {
	return downloadsPath()
}

// ScanPathPreset 為前端可一鍵選用的常見掃描路徑預設項目。
type ScanPathPreset struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

// GetCommonScanPaths 回傳常見掃描路徑（如下載、桌面）的預設清單。
func (a *App) GetCommonScanPaths() []ScanPathPreset {
	return commonScanPaths()
}

// OpenScanResultLocation 在 Finder 中顯示掃描結果檔案的位置。
func (a *App) OpenScanResultLocation(result ScanResult) error {
	return a.fileActionService().OpenScanResultLocation(a.context(), result)
}

// QuarantineScanResult 將感染的掃描結果隔離，並記錄到 App log。
func (a *App) QuarantineScanResult(result ScanResult) (QuarantineRecord, error) {
	record, err := a.fileActionService().Quarantine(result)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "隔離失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已隔離："+filepath.Base(result.Path))
	}
	return record, err
}

// RestoreQuarantineRecord 將隔離紀錄還原回原始位置，並記錄到 App log。
func (a *App) RestoreQuarantineRecord(id string) (QuarantineRecord, error) {
	record, err := a.fileActionService().Restore(id)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "還原失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已還原："+filepath.Base(record.OriginalPath))
	}
	return record, err
}

// OpenQuarantineLocation 在 Finder 中顯示隔離檔（或已還原檔）的位置。
func (a *App) OpenQuarantineLocation(record QuarantineRecord) error {
	return a.fileActionService().OpenQuarantineLocation(a.context(), record)
}

// ListQuarantineRecords 回傳所有隔離紀錄，最新偵測者排在前。
func (a *App) ListQuarantineRecords() ([]QuarantineRecord, error) {
	return a.fileActionService().ListQuarantineRecords()
}

// MoveQuarantineRecordToTrash 將隔離檔移到垃圾桶並更新紀錄狀態。
func (a *App) MoveQuarantineRecordToTrash(id string) (QuarantineRecord, error) {
	record, err := a.fileActionService().MoveQuarantineToTrash(a.context(), id)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "移到垃圾桶失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已將隔離項目移到垃圾桶："+filepath.Base(record.OriginalPath))
	}
	return record, err
}

// PermanentlyDeleteQuarantineRecord 永久刪除隔離檔並更新紀錄狀態；此操作不可復原，呼叫前須取得使用者確認。
func (a *App) PermanentlyDeleteQuarantineRecord(id string) (QuarantineRecord, error) {
	record, err := a.fileActionService().PermanentlyDeleteQuarantine(id)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "永久刪除失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已永久刪除隔離項目："+filepath.Base(record.OriginalPath))
	}
	return record, err
}

// MoveScanResultToTrash 將掃描結果檔案移到垃圾桶（可從垃圾桶復原）。
func (a *App) MoveScanResultToTrash(result ScanResult) error {
	err := a.fileActionService().MoveToTrash(a.context(), result.Path)
	if err != nil {
		_ = a.logs().WriteAppLog("error", "移到垃圾桶失敗："+err.Error())
	} else {
		_ = a.logs().WriteAppLog("info", "已移到垃圾桶："+filepath.Base(result.Path))
	}
	return err
}

// PermanentlyDeleteScanResult 永久刪除掃描結果檔案；此操作不可復原，呼叫前須取得使用者確認。
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
