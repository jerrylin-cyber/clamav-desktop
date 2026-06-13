# ClamAV Desktop 正式版 Task List

日期：2026-06-11  
範圍：Golang + Wails macOS desktop app，正式版首輪。  
主線：Homebrew ClamAV 偵測與阻擋式安裝/啟動引導、`freshclam` 使用者可寫 config fallback、`clamd` Unix socket、`INSTREAM`。  
非主線：app-managed runtime artifact、privileged installer apply、app-managed runtime uninstaller、`clamscan` 主掃描流程。

## 0. 執行狀態

- [x] Repository：已執行 `git init`。
- [x] 檔名：已改為 `tasks.md`。
- [x] Phase 0：專案初始化。
  - [x] T0.1：建立 Wails app skeleton。
  - [x] T0.2：建立專案品質工具。
- [x] Phase 1：Runtime Bundle。（規格已改版，app-managed artifact 移出主線）
- [x] Phase 2：Shared Runtime Installer。（規格已改版，privileged installer apply 移出主線；保留既有 installer 程式作歷史/診斷）
- [x] Phase 3：Runtime Resolver。
- [x] Phase 4：Database Update。
- [x] Phase 5：ClamD Client。
- [x] Phase 6：Scan Job Manager。
- [x] Phase 7：GUI。（已改為 Homebrew ClamAV blocking setup popup 與檢測/移除提示）
- [x] Phase 8：File Actions。（T8.1 / T8.2 / T8.3 已完成）
- [x] Phase 9：Settings。（T9.1/T9.2/T9.3 已完成；含 runtime 檢測、移除提示、login/background 設定）
- [x] Phase 10：Scheduler / Background。（T10.1 / T10.2 / T10.3 已完成，App startup/shutdown 已接入背景 worker）
- [x] Phase 11：Status Bar / Login。（已加入 NSStatusItem；login item 優先 SMAppService、fallback LaunchAgent）
- [x] Phase 12：Uninstaller。（規格已改為提示移除 Homebrew ClamAV；T12.2 使用者資料移除已完成）
- [ ] Phase 13：Packaging / Signing / Notarization。

## 1. 成功條件

- macOS app 可顯示 GUI，並可由狀態列 icon 背景常駐。
- App 可偵測 Homebrew ClamAV；不可用時顯示阻擋式引導，不再自行安裝 app-managed runtime。
- ClamAV runtime 以 Homebrew 位置為主；病毒碼 database 可使用 app 產生的使用者可寫 fallback。
- 每位使用者有獨立 settings、scan jobs、results、quarantine、schedule、login item。
- 掃描主流程使用 `clamd` + Unix socket + `INSTREAM`。
- `clamscan` 僅作 diagnostic fallback，不自動替代正式掃描 job。
- 可單次掃描、查詢進度、取消掃描、顯示警告、開啟位置、隔離、還原、移到 Trash。
- 可設定掃描排程、database 更新排程、省電策略、登入啟動、背景常駐。
- 移除 runtime 時只提示使用者自行移除 Homebrew ClamAV，不刪 external runtime。
- 不宣稱 macOS real-time protection。

## 2. 任務規則

- `PATH` 不作為 runtime 判斷主依據，所有 binary / config / database / socket 都使用絕對路徑。
- 不使用 `/tmp/clamd.socket` 作為正式版 socket。
- 不啟用遠端 TCP `clamd`。
- 不預設永久刪除偵測檔案。
- 不把使用者 quarantine 放在 shared system path。
- `per-user runtime installer` 與 privileged shared runtime installer 移出主線。
- 無可用 ClamAV 時，顯示 Homebrew 安裝/設定/啟動引導。

## 3. 目標路徑

系統共用：

```text
/Library/Application Support/ClamAVDesktop/Runtime/
/Library/Application Support/ClamAVDesktop/Runtime/bin/
/Library/Application Support/ClamAVDesktop/Runtime/sbin/
/Library/Application Support/ClamAVDesktop/Database/
/Library/Application Support/ClamAVDesktop/Config/
/Library/Application Support/ClamAVDesktop/Run/clamd.sock
/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.freshclam.plist
/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.clamd.plist
```

使用者資料：

```text
~/Library/Application Support/ClamAVDesktop/settings.json
~/Library/Application Support/ClamAVDesktop/jobs/
~/Library/Application Support/ClamAVDesktop/results/
~/Library/Application Support/ClamAVDesktop/quarantine/
~/Library/Logs/ClamAVDesktop/
~/Library/LaunchAgents/com.lazyjerry.clamavdesktop.agent.plist
```

## 4. Phase 0：專案初始化

### T0.1 建立 Wails app skeleton

狀態：完成。

- 建立 Go module。
- 建立 Wails v2 + React TypeScript app。
- 建立基礎 app shell：Dashboard、Scan、Results、Schedule、Quarantine、Settings、Logs、About。
- 建立 frontend / backend binding 範例。

驗證：

- `wails dev` 可啟動 dev server 與 dev build。
- `wails build -skipbindings` 可產生 macOS app。
- GUI 有基本路由與空狀態。

### T0.2 建立專案品質工具

狀態：完成。

- Go lint / test script。
- Frontend lint / typecheck script。
- Build script。
- CI workflow：lint、test、build。

驗證：

- 本機與 CI 可執行 lint / test / build。
- 失敗時 CI 有明確錯誤。

## 5. Phase 1：Runtime Bundle

### T1.1 建立 ClamAV runtime build pipeline

- 從 ClamAV source 在 CI 建置 macOS runtime。
- 使用 universal artifact 支援 arm64 / x86_64。
- 指定 staging prefix：

```text
Runtime/
Config/
Database/
```

- 產出：

```text
clamav-runtime-darwin-universal.tar.zst
clamav-runtime-darwin-universal.sha256
clamav-runtime-darwin-universal.sig
runtime-metadata.json
```

驗證：

- artifact 解壓後可執行 `freshclam --version`、`clamd --version`、`clamscan --version`。
- binary / dylib 可 codesign。
- artifact checksum 可驗證。

### T1.2 建立 runtime metadata

metadata 欄位：

```json
{
  "name": "clamav-runtime-darwin-universal",
  "clamavVersion": "1.5.2",
  "arch": ["arm64", "x86_64"],
  "builtAt": "2026-06-11T15:00:00+08:00",
  "sha256": "..."
}
```

驗證：

- App 可讀取 metadata。
- metadata 與 artifact checksum 一致。

## 6. Phase 2：Shared Runtime Installer

### T2.1 建立 installer service

狀態：歷史保留。已建立 shared runtime layout、install plan、`freshclam.conf`、`clamd.conf` 生成與測試；已新增 `ApplyInstallPlan` 與 `ExtractRuntimeArchive` 測試。但新規格已改為 Homebrew ClamAV 偵測與引導，本段不再是主線交付要求。

- 解壓 runtime artifact 到 `/Library/Application Support/ClamAVDesktop/Runtime/`。
- 建立 `Config`、`Database`、`Run` 目錄。
- 建立 `freshclam.conf`。
- 建立 `clamd.conf`，內含固定安全預設值 `AlertEncrypted yes`、`AlgorithmicDetection yes`（取代原 per-job heuristic/加密警告選項，見 T6.4）。
- 設定 `LocalSocket` 到 app-owned path。
- 設定 shared database path。

驗證：

- 新規格：clean macOS 上顯示 Homebrew 安裝與 clamd 啟動引導。
- 目錄權限符合 system updater 可寫 database、`clamd` 可讀 database。
- socket path 不在 `/tmp`。
- `clamd.conf` 內 `AlertEncrypted`、`AlgorithmicDetection` 預設為 `yes`。
- `ApplyInstallPlan` 寫入的目錄/檔案權限已由 `TestApplyInstallPlanWritesDirectoriesAndFilesWithModes` 驗證。
- `ExtractRuntimeArchive` 的 checksum 驗證、`Runtime/` 子樹解壓（含 symlink）、略過 `Config/`、防止 `..` 路徑逸出，已由 `TestExtractRuntimeArchiveExtractsRuntimeTreeAndSkipsOtherEntries`、`TestExtractRuntimeArchiveRejectsChecksumMismatch` 驗證。

後續工作（待 privileged apply 完成後評估）：

- 若需讓使用者調整 `AlertEncrypted` / heuristic（`AlgorithmicDetection`），改為 Settings 全域設定，存檔後透過 admin 提權重寫共用 `clamd.conf` 並 `launchctl kickstart` 重啟 `clamd`；GUI 需提示此操作為全域設定且會中斷所有使用者目前進行中的掃描。

### T2.2 建立 install manifest

狀態：完成。

manifest 欄位：

```json
{
  "mode": "system-shared",
  "runtimeVersion": "1.5.2",
  "installedPaths": [],
  "launchdLabels": [],
  "databasePath": "/Library/Application Support/ClamAVDesktop/Database",
  "configPath": "/Library/Application Support/ClamAVDesktop/Config",
  "installedByAppVersion": "..."
}
```

驗證：

- 每個 app-managed path 都有 manifest 記錄。
- 移除流程只依 manifest 刪除。
- Homebrew、官方 `.pkg`、manual path 不會被記入 app-managed manifest。
- 已由 `TestInstallManifestRecordsOnlyAppManagedPaths` 驗證。

### T2.3 建立 LaunchDaemon plist

狀態：完成。

- `com.lazyjerry.clamavdesktop.freshclam.plist`
- `com.lazyjerry.clamavdesktop.clamd.plist`
- 支援 load / unload / status。
- log 寫入 app-managed log path。

驗證：

- 重新開機後 `freshclam` / `clamd` 可由 launchd 管理。
- `clamd` socket 可被使用者層 app 連線。
- `freshclam` 不會由多個使用者同時寫 shared database。
- plist 內容已由 `TestLaunchDaemonPlistsUseAbsoluteBinariesAndLaunchdLogs` 驗證，load / unload / status 指令參數已由 `TestLaunchDaemonManagerUsesLaunchctlSystemDomain` 驗證；實機 `launchctl bootstrap` 仍待 T2.1 privileged apply 完成後驗證。

## 7. Phase 3：Runtime Resolver

### T3.1 建立 `RuntimeResolver`

狀態：完成。

偵測順序：

1. app-managed system-shared runtime。
2. per-user runtime，僅顯示診斷狀態。
3. Homebrew runtime，僅顯示 external source。
4. 官方 `.pkg` `/usr/local/clamav`，僅顯示 external source。
5. manual path，透過 `CLAMAV_DESKTOP_RUNTIME_PATH` 指定，僅顯示 diagnostic source。
6. missing。

Runtime profile：

```go
type RuntimeProfile struct {
    Mode          string
    ClamScanPath  string
    FreshclamPath string
    ClamdPath     string
    ClamdSocket   string
    DatabasePath  string
    ConfigPath    string
    Source        string
    Warnings      []string
}
```

驗證：

- GUI 不依賴 shell `PATH`。
- 未安裝 runtime 時顯示 Homebrew blocking setup popup。
- 偵測到 external/Homebrew runtime 時顯示來源與健康檢查結果。
- 偵測到 per-user runtime 時顯示非主線診斷。

### T3.2 建立 Runtime Health Check

狀態：完成。

- 檢查 binary 是否存在與可執行。
- 檢查 config 是否存在。
- 檢查 database 是否存在。
- 檢查 `clamd` socket 是否存在。
- 執行 `PING` / `VERSIONCOMMANDS`。

驗證：

- Dashboard 可顯示 runtime status。
- `clamd` 不可用時顯示 repair required。
- 不自動改用 `clamscan` 建立正式掃描 job。

## 8. Phase 4：Database Update

### T4.1 建立 `FreshclamService`

狀態：完成。

- 手動更新 database。
- 串流 stdout / stderr 到 GUI。
- 儲存 last updated metadata。
- 分類 network、permission、config、lock error。

驗證：

- 更新成功時 Dashboard 顯示 database 時間。
- 更新失敗時 GUI 顯示可理解錯誤。
- `freshclam` lock 時不啟動第二個 updater。
- 已由 `TestFreshclamServiceUpdatesDatabaseAndStoresStatus`、`TestFreshclamServiceClassifiesNetworkError`、`TestFreshclamServiceClassifiesPermissionConfigAndLockErrors`、`TestFreshclamServicePreventsConcurrentUpdates` 驗證；Wails `UpdateDatabase` 會發送 `freshclam:event`。
- GUI 端：「紀錄」頁已訂閱 `freshclam:event`，即時顯示 stdout/stderr 串流（最多保留 50 筆，更新開始時清空）。

### T4.2 建立 database status model

狀態：完成。

欄位：

```go
type DatabaseStatus struct {
    Path        string
    LastUpdated time.Time
    Version     string
    Signatures  int
    Error       string
}
```

驗證：

- Dashboard、Settings、Logs 可讀取同一份狀態。
- 已由 `TestFreshclamServiceLoadStatusReturnsEmptyStatusWhenMissing`、`TestParseFreshclamStatusUsesLatestVersionLine`、`TestAppDatabaseMethodsUseFreshclamService` 驗證；GUI 的 Dashboard、Settings、Logs 皆讀取 `AppStatus.database`。

## 9. Phase 5：ClamD Client

### T5.1 建立 Unix socket client

狀態：完成。

- 支援 `PING`。
- 支援 `VERSIONCOMMANDS`。
- 支援 `SCAN`。
- 支援 `INSTREAM`。
- 支援 context cancellation。
- 設定 read / write timeout。

驗證：

- `PING` 回 `PONG`。
- `VERSIONCOMMANDS` 可解析 capability。
- `INSTREAM` 可掃描測試檔。
- 取消掃描會中止 socket job。
- 已由 `TestClamDClientPing`、`TestClamDClientVersionCommands`、`TestClamDClientScan`、`TestClamDClientInstreamSendsChunks`、`TestClamDClientInstreamHonorsContextCancellation`、`TestClamDClientSetsReadWriteDeadline` 驗證。

### T5.2 建立 `INSTREAM` file reader

狀態：完成。

- 由使用者層 app 讀取檔案。
- 使用 chunk 傳送。
- 受 `StreamMaxLength` 限制時回報可理解錯誤。
- access denied 時回報權限提示。

驗證：

- daemon 沒有直接讀取使用者 path 權限時，仍可透過 `INSTREAM` 掃描。
- 受保護目錄顯示 skipped / access denied。
- 已由 `TestClamDClientInstreamFileReadsUserFile`、`TestClamDClientInstreamReportsStreamMaxLength`、`TestClassifyFileReadErrorReportsAccessDenied` 驗證。

## 10. Phase 6：Scan Job Manager

### T6.1 建立 scan job model

狀態：完成。

欄位：

```go
type ScanJob struct {
    ID        string
    Paths     []string
    Options   ScanOptions
    Status    string
    StartedAt time.Time
    EndedAt   *time.Time
}
```

```go
type ScanOptions struct {
    Recursive bool
    AllMatch  bool
}
```

驗證：

- 可建立、查詢、取消 scan job。
- 同一使用者的 job 不和其他使用者混用。
- 已由 `TestScanJobManagerRunsScanAndStoresResults`、`TestScanJobManagerCanCancelRunningScan`、`TestScanJobManagerKeepsUserStoresSeparate` 驗證。

### T6.2 建立 scan progress event

狀態：完成。

事件：

```json
{
  "jobId": "scan_20260611_001",
  "status": "scanning",
  "currentPath": "/Users/jerry/Downloads/file.zip",
  "scannedFiles": 120,
  "detections": 1,
  "errors": 2
}
```

驗證：

- GUI 可即時顯示目前檔案、掃描數、detections、errors。
- 取消後狀態變成 canceled。
- 取消後狀態已由 `TestScanJobManagerCanCancelRunningScan` 驗證；Wails `scan:progress` event 已串接 Scan page 的目前檔案、掃描數、detections、errors 與取消操作。

### T6.3 建立 result parser / store

狀態：完成。

結果：

```go
type ScanResult struct {
    Path      string
    Status    string
    Signature string
    Engine    string
    Error     string
}
```

驗證：

- `FOUND` 顯示 infected。
- clean 顯示 clean。
- permission error 顯示 skipped / error。
- 結果寫入 per-user results path。
- 已由 `TestScanJobManagerParsesInfectedAndErrorResults`、`TestScanJobManagerScansRecursiveDirectory`、`TestScanResultFromReplyParsesClamdReplies` 驗證。

### T6.4 移除 ScanOptions 中無效的 per-job heuristic/加密警告選項

狀態：完成。

背景：`ScanOptions.Heuristic`、`ScanOptions.AlertEncrypted` 目前不影響 `ScanJobManager.RunScan` 與 `ClamDClient.Instream`，Scan 頁面對應勾選框是無效開關。`clamd` 為系統共用單一 daemon，`AlertEncrypted`、heuristic（對應 `AlgorithmicDetection`）只能是 daemon 全域設定，無法做到 per-job。

- 從 `ScanOptions` 移除 `Heuristic`、`AlertEncrypted` 欄位，僅保留 `Recursive`、`AllMatch`。
- 移除 Scan 頁面「啟用 heuristic」「警告加密檔案」勾選框與對應 state。
- 改由 T2.1 的 `clamd.conf` 模板寫入固定預設值 `AlertEncrypted yes`、`AlgorithmicDetection yes`。

驗證：

- `ScanOptions` 結構與既有測試不再含 `Heuristic`/`AlertEncrypted`。
- Scan 頁面不再顯示這兩個勾選框。
- `go test ./...`、frontend typecheck/build 全部通過。
- `clamd.conf` 內 `AlertEncrypted yes`、`AlgorithmicDetection yes` 已由 `TestSharedRuntimeConfigsUseUnixSocketAndSharedDatabase` 驗證。

## 11. Phase 7：GUI

### T7.1 Dashboard

狀態：完成。已完成 Runtime status、Database status、`clamd` health（checks table）、最近掃描摘要（`ListScanJobs` 最新一筆，含掃描/偵測/錯誤統計）、目前掃描 job（共用 `scanProgress` 事件，顯示 queued/scanning 中的工作）；無可用 ClamAV 或 `clamd` unhealthy 時會顯示 Homebrew blocking setup popup，檢測通過前不能關閉繼續使用。

功能：

- Runtime status。
- Database status。
- `clamd` health。
- 最近掃描摘要。
- 目前掃描 job。

驗證：

- 無 runtime 顯示 Homebrew 安裝引導。
- `clamd` unhealthy 顯示 clamd 設定與啟動引導。
- database 過期顯示 Update。

### T7.2 Scan page

狀態：完成。

- 選檔（`SelectScanFiles`，native 多選檔案對話框，`wailsruntime.OpenMultipleFilesDialog`）。
- 選資料夾（`SelectScanFolder`，native 資料夾對話框，`wailsruntime.OpenDirectoryDialog`）；兩者皆以注入式 runner 設計，取消時回傳空值不視為錯誤。選擇結果會合併進掃描路徑欄位（去除重複）。
- 快速填入 Downloads、手動路徑輸入、Scan options（遞迴掃描）、Start / Cancel、進度顯示與結果摘要。
- 原「啟用 heuristic」「警告加密檔案」勾選框已依 T6.4 移除，改由 `clamd.conf` 固定預設值提供。

驗證：

- 可掃單檔與資料夾。
- 掃描中可取消。
- 無權限 path：`clamd_client.go` 將 `os.ErrPermission` 分類為「權限不足，無法讀取掃描檔案」，結果以 `skipped` 狀態與錯誤訊息顯示於 Scan / Results page。
- 已由 `TestSelectScanFilesReturnsChosenPaths`、`TestSelectScanFilesReturnsEmptyWhenCanceled`、`TestSelectScanFolderReturnsChosenPath`、`TestSelectScanFolderReturnsEmptyWhenCanceled` 驗證；前端 `npm run typecheck` 與 `npm run build` 通過。

### T7.3 Results page

狀態：完成。

- 獨立「結果」頁：依掃描紀錄（`ListScanJobs`）選擇要檢視的 job，再以 `LoadScanResults` 讀取該 job 的結果列表。
- filter：全部、clean、infected、error、skipped（依 `ScanResult.status` 篩選）。
- 每筆結果顯示 signature、path、engine、error。
- 動作入口：Open Location、Quarantine、Move to Trash、Permanent Delete（皆僅對 infected 結果顯示，Permanent Delete 依 `settings.actions.confirmPermanentDelete` 二次確認）。Restore 僅適用於已隔離項目，由 T7.4 Quarantine page 提供，Results page 不重複提供。
- Scan page 的結果摘要（含相同動作入口）維持不變，供掃描完成後立即操作。

驗證：

- infected 結果有可行動按鈕，clean/skipped/error 結果不顯示危險動作。
- Permanent Delete 必須二次確認。
- 隔離、移到垃圾桶、永久刪除成功後前端會將該筆結果標記為對應狀態（移除已執行過的動作按鈕、不再重新讀取已過期的持久化結果），避免對同一筆結果重複觸發動作。
- 前端 `npm run typecheck` 與 `npm run build` 通過。

### T7.4 Quarantine page

狀態：完成。

- 隔離檔案列表（`ListQuarantineRecords`，依 detectedAt 由新到舊排序）。
- 查看 metadata（signature、original path、偵測時間、狀態）。
- Restore。
- Move to Trash。
- Permanent Delete（依 `settings.actions.confirmPermanentDelete` 二次確認）。

驗證：

- Restore 可回原位置。
- 原位置已有檔案時 `Restore` 回傳錯誤並於 GUI 顯示訊息，使用者需自行處理原位置衝突後再試（尚未提供改名 UI）。
- Quarantine 不跨使用者顯示（per-user `QuarantinePath`）。
- 已由 `TestListQuarantineRecordsReturnsSortedRecords`、`TestListQuarantineRecordsReturnsEmptyWhenMissing`、`TestAppQuarantineListAndTrashMethodsUseSameService` 驗證。

### T7.5 Logs page

狀態：完成。

- App log（`ListAppLogEntries`，per-user JSON-lines，`~/Library/Logs/ClamAVDesktop/app.log`，0600 權限）。
- Scan log（`ListScanJobs`，依 startedAt 由新到舊排序，含 scannedFiles / detections / errors 統計）。
- `freshclam` log / `clamd` log（`ReadFreshclamLog` / `ReadClamdLog`，讀取共用路徑 `/Library/Logs/ClamAVDesktop/`，缺檔時回傳空陣列）。
- 匯出診斷資訊（`ExportDiagnostics`，彙整 runtime / health / database / 最近掃描 / app log / 共用 log，輸出至 `~/Library/Logs/ClamAVDesktop/diagnostics-<timestamp>.txt` 並於 Finder 顯示）。

驗證：

- 使用者可於「紀錄」頁重新整理並匯出診斷報告（Finder 會開啟並標示檔案）。
- log 訊息僅記錄檔名（`filepath.Base`），不包含完整私人路徑。
- 已由 `TestWriteAppLogAndListAppLogEntries`、`TestListAppLogEntriesAppliesLimit`、`TestListAppLogEntriesReturnsEmptyWhenMissing`、`TestReadSharedLogReturnsLinesWithinLimit`、`TestReadSharedLogReturnsEmptyWhenMissing`、`TestExportDiagnosticsWritesReportAndRevealsLocation`、`TestListScanJobsReturnsSortedJobs`、`TestListScanJobsReturnsEmptyWhenMissing`、`TestAppScanMethodsUseSameManager` 驗證。

## 12. Phase 8：File Actions

### T8.1 Open Location

狀態：完成。

- 呼叫 `/usr/bin/open -R <path>`。
- path 必須來自掃描結果或 quarantine metadata。

驗證：

- Finder 可選取目標檔案。
- 檔案不存在時顯示已移除。
- 已由 `TestOpenScanResultLocationUsesFinderReveal` 驗證；App method 已暴露 `OpenScanResultLocation` / `OpenQuarantineLocation`。

### T8.2 Quarantine / Restore

狀態：完成。

- 移動到 per-user quarantine path。
- 保存 original path、quarantine path、signature、detectedAt、sha256。
- Restore 時檢查原位置是否存在同名檔案。

驗證：

- Quarantine 後原檔不在原位置。
- Restore 後 quarantine metadata 更新。
- A 使用者看不到 B 使用者 quarantine。
- 已由 `TestQuarantineMovesFileAndStoresMetadata`、`TestRestoreMovesQuarantineFileBack`、`TestRestoreRefusesToOverwriteExistingFile`、`TestNewFileActionServiceUsesPerUserQuarantinePath`、`TestAppFileActionMethodsUseSameService` 驗證。

### T8.3 Move to Trash / Permanent Delete

狀態：完成。

- Move to Trash 透過 `osascript`／Finder（`tell application "Finder" to delete POSIX file ...`）執行，屬 macOS native trash API，不使用 `rm`。
- Permanent Delete 直接 `os.Remove`，由 GUI 的 `confirmPermanentDelete()` 依 `settings.actions.confirmPermanentDelete` 決定是否彈出二次確認對話框。
- 兩個動作皆會寫入 per-user audit log（`~/Library/Application Support/ClamAVDesktop/audit.log`，append-only JSON Lines，0600 權限，記錄 `at`/`action`/`path`）。

驗證：

- 不使用 `rm` 作為 Trash。
- Permanent Delete 有不可逆提示。
- 已由 `TestMoveToTrashInvokesRunnerAndWritesAuditLog`、`TestMoveToTrashRejectsMissingPath`、`TestPermanentlyDeleteRemovesFileAndWritesAuditLog`、`TestMoveQuarantineToTrashUpdatesStatus`、`TestPermanentlyDeleteQuarantineUpdatesStatus`、`TestMoveOrDeleteQuarantineRejectsNonQuarantinedStatus`、`TestAppMoveAndDeleteScanResultUseSameService` 驗證。

## 13. Phase 9：Settings

### T9.1 建立 `SettingsStore`

狀態：完成。

路徑：

```text
~/Library/Application Support/ClamAVDesktop/settings.json
```

要求：

- atomic write。
- schema version。
- migration hook。
- 預設值。

驗證：

- 寫入中斷不會留下壞檔。
- schema version 不相容時顯示修復提示。
- 已由 `TestSettingsStoreLoadsDefaultsWhenMissing`、`TestSettingsStoreSavesAndLoadsAtomically`、`TestSettingsStoreRejectsUnsupportedSchemaVersion`、`TestSettingsStoreRunsMigrationHook` 驗證。

### T9.2 Settings model

狀態：完成。已建立 model、預設值、per-user path 與 Wails backend method；Settings 存檔後已喚醒背景排程即時套用，Schedule 頁與 Settings 頁共用同一份 settings，Dashboard app state 也會載入同一份 settings。

```go
type Settings struct {
    RuntimeMode    string         `json:"runtimeMode"`
    ScanSchedule   ScanSchedule   `json:"scanSchedule"`
    UpdateSchedule UpdateSchedule `json:"updateSchedule"`
    PowerPolicy    PowerPolicy    `json:"powerPolicy"`
    Background     Background     `json:"background"`
    Login          LoginSettings  `json:"login"`
    Actions        ActionSettings `json:"actions"`
}
```

驗證：

- Dashboard、Schedule、Settings 讀取同一份 settings。
- 修改 Settings 後 background scheduler 會立即重新檢查排程。
- `GetSettings` / `SaveSettings` 共用 store 已由 `TestAppSettingsMethodsUseSameStore` 驗證。

### T9.3 Settings page controls

狀態：完成。Settings GUI 已串接 scan schedule、update schedule、power policy、login、background toggles、Full Disk Access/Notifications 設定入口、runtime 重新檢測、runtime 移除提示、SMAppService/LaunchAgent login item、使用者資料移除與儲存。

- Runtime status。
- Runtime 重新檢測。
- Runtime 移除提示。
- Scan schedule。
- Update schedule。
- Run on battery。
- Run in Low Power Mode。
- Defer until charging。
- Launch at login。
- Start hidden。
- Keep menu bar icon。
- Full Disk Access status。
- Notifications。

驗證：

- 每個 toggle 都有後端 method。
- 修改排程不需重開 app，`SaveSettings` 會喚醒背景 worker。
- Full Disk Access / Notifications 按鈕會開啟對應 macOS System Settings 頁面。
- 登入啟動 toggle 可優先透過 SMAppService 註冊 / 取消；失敗時 fallback 目前使用者的 LaunchAgent login item。

## 14. Phase 10：Scheduler / Background

### T10.1 建立 `SchedulerService`

狀態：完成。已建立 `SchedulerService`（`scheduler.go`）：`NextScanRun` 依 `ScanSchedule`（`Frequency`/`TimeOfDay`/`Weekday`）計算下一次觸發時間，支援 daily 與 weekly；`Enabled=false` 即視為暫停，`NextScanRun`/`ScanDue` 直接回傳未啟用。`ScanDue` 以「上次執行時間之後的下一次觸發時間」與目前時間比較，若應用程式關閉導致錯過排程，下次檢查時會持續回報 due=true 直到補跑完成。`RunScheduledScan` 先呼叫 T10.3 的 `PowerPolicyService.ShouldDefer`，未被延後才呼叫 `ScanJobManager.RunScan` 依 `ScanSchedule.Paths` 建立並執行 scan job。per-user job 停止與不掃其他使用者資料為既有架構特性（所有路徑與 job 皆綁定呼叫者 `homeDir`），無需額外程式碼。

- 支援每日、每週、自訂時間。
- 支援錯過補跑。
- 支援暫停排程。
- 依使用者 settings 建立 job。

驗證：

- 排程時間到會建立 scan job。
- 登出後 per-user job 停止。
- 不掃其他使用者資料。
- 已由 `TestParseTimeOfDayValidAndInvalid`、`TestNextScheduledTimeDaily`、`TestNextScheduledTimeWeekly`、`TestNextScheduledTimeRejectsInvalidInput`、`TestNextScanRunReturnsFalseWhenDisabled`、`TestScanDueCatchesUpAfterMissedRun`、`TestScanDueFalseWhenDisabled`、`TestRunScheduledScanDefersPerPowerPolicy`、`TestRunScheduledScanReturnsPowerPolicyError`、`TestRunScheduledScanRunsWhenNotDeferred` 驗證。

### T10.2 建立 `UpdateSchedulerService`

狀態：完成。已建立 `UpdateSchedulerService`（`scheduler.go`）：`NextUpdateRun` 依 `UpdateSchedule` 計算每日更新時間，並使用 hostname + user home 的 deterministic jitter 分散多台或多使用者更新時間；`UpdateDue` 依上次執行時間判斷是否到期；`RunScheduledUpdate` 會先判斷 `RuntimeMode == "system-shared"`，此模式回傳「病毒碼更新由 system updater 管理」的 defer 決策，呼叫端不會再呼叫 `FreshclamService.UpdateDatabase`。非 system-shared 模式會先套用 `PowerPolicyService.ShouldDefer`，未被延後才執行 freshclam。App 已在 `background.go` 建立 startup/shutdown 背景 worker，定期讀取 settings 執行 scan/update 排程，`SaveSettings` 會喚醒 worker 重新檢查。

- 依 settings 執行 database update。
- 使用 jitter，避免多台或多使用者同時更新。
- shared mode 只允許 system updater 寫 database。

驗證：

- 更新排程變更後立即喚醒 background worker 重新檢查。
- `system-shared` 不會由使用者 App 重複寫 shared database。
- 已由 `TestNextUpdateRunAppliesDeterministicJitter`、`TestNextUpdateRunRollsAfterJitteredTime`、`TestRunScheduledUpdateDefersSystemSharedRuntime`、`TestRunScheduledUpdateDefersPerPowerPolicy` 驗證。

### T10.3 建立 `PowerPolicyService`

狀態：完成。已建立 `PowerPolicyService`（`power_policy.go`），透過 `pmset -g batt` 與 `pmset -g` 讀取電源狀態（`ReadStatus`），並依 `PowerPolicy`（`RunOnBattery`、`RunInLowPowerMode`）判斷是否延後（`ShouldDefer`）；`DeferUntilCharging` 作為 T10.1 排程補跑時機的提示，由排程層在下次 tick 重新呼叫 `ShouldDefer` 即可實現接上電源後補跑。command runner 採可注入設計，測試不需實際執行 `pmset`。

- 檢查 battery 狀態。
- 檢查 Low Power Mode。
- 支援 defer until charging。

驗證：

- battery 且 `runOnBattery=false` 時延後 scan/update。
- Low Power Mode 且 `runInLowPowerMode=false` 時延後 scan/update。
- 接上電源後補跑。
- 已由 `TestParsePowerStatusOnACWithLowPowerModeOff`、`TestParsePowerStatusOnBatteryWithLowPowerModeOn`、`TestReadStatusUsesRunner`、`TestReadStatusReturnsErrorWhenPmsetFails`、`TestShouldDeferOnBatteryWhenRunOnBatteryDisabled`、`TestShouldDeferInLowPowerModeWhenRunInLowPowerModeDisabled`、`TestShouldDeferNotDeferredWhenPolicyAllows`、`TestShouldDeferRecoversOncePluggedIn` 驗證。

## 15. Phase 11：Status Bar / Login

### T11.1 建立 status bar helper

狀態：完成。已透過 darwin cgo / AppKit 建立 `NSStatusItem`，啟動時依 `KeepMenuBarIcon` 建立狀態列選單。

- 使用 `NSStatusItem`。
- 顯示掃描狀態。
- menu actions：
  - Open Window（已完成）
  - Scan Downloads（已完成）
  - Update Database（已完成）
  - Pause Schedule（已完成）
  - Last Result（已完成：開啟主視窗）
  - Quit（已完成）

驗證：

- 關閉視窗後 app 仍在狀態列。
- Quit 會停止使用者層 background process。
- 掃描中 icon 狀態會變更。

### T11.2 建立 login item

狀態：完成。已完成 SMAppService 優先路線，若 macOS 版本、簽署或 app 位置導致 SMAppService 不可用，會 fallback 到 per-user LaunchAgent。Settings 會顯示目前 method。

- 優先使用 `SMAppService`。（已完成）
- fallback 使用 per-user LaunchAgent。（已完成）
- 支援 Start Hidden。（LaunchAgent fallback 已完成）
- 支援 Keep Menu Bar Icon。

驗證：

- 開啟 Launch at Login 後重新登入會啟動。
- 關閉 Launch at Login 後重新登入不啟動。
- 多使用者設定互不影響。
- 已由 `TestLoginItemRegisterWritesLaunchAgent`、`TestLoginItemUnregisterRemovesLaunchAgent`、`TestAppSaveSettingsAppliesLoginItemChange` 驗證 fallback 寫入 / 移除 / settings 串接；SMAppService native bridge 已由 build/test 編譯驗證，仍需實機重新登入驗證。

## 16. Phase 12：Uninstaller

### T12.1 建立 runtime removal guidance

狀態：完成。新規格不刪除 app-managed runtime 或 Homebrew runtime；Settings 顯示移除提示，請使用者自行 `brew uninstall clamav`，並保留 T12.2 的使用者資料移除。

- 不讀取 `install-manifest.json` 作刪除依據。
- 不 unload LaunchDaemon。
- 不刪 external runtime。
- 顯示移除提示。

驗證：

- shared runtime 可完整移除。
- Homebrew ClamAV 不會被刪。
- 官方 `/usr/local/clamav` 不會被刪，除非 manifest 明確記錄為 app-managed。

### T12.2 建立使用者資料移除選項

狀態：完成。已建立 `RemoveUserData` Wails backend method 與 Settings GUI「使用者資料」區塊，可移除目前使用者的 settings、scan jobs、scan results、app log；quarantine 只有在使用者明確勾選並通過二次確認後才移除。移除範圍限制在目前使用者的 `~/Library/Application Support/ClamAVDesktop` 與 `~/Library/Logs/ClamAVDesktop`，不處理 shared runtime 或 external runtime。

選項：

- Remove Settings。
- Remove Scan Jobs。
- Remove Scan Results。
- Remove Quarantine。
- Keep User Data。
- Remove App Log。

驗證：

- 預設不刪除 quarantine。
- 刪除 quarantine 需要二次確認。
- 已由 `TestRemoveUserDataRequiresSelection`、`TestRemoveUserDataRemovesSelectedPathsOnly`、`TestRemoveUserDataRemovesQuarantineOnlyWhenExplicit` 驗證。

## 17. Phase 13：Packaging / Signing / Notarization

### T13.1 建立 production build

- `wails build -platform darwin/universal -clean`
- app codesign。
- runtime binaries / dylibs codesign。
- installer package。
- notarization。
- stapler。

驗證：

- Gatekeeper 不阻擋。
- clean macOS VM 可安裝。
- Apple Silicon / Intel 都可開啟。

### T13.2 建立 release smoke test

測試：

- 無 Homebrew。
- 未安裝 ClamAV。
- 依 popup 安裝 Homebrew ClamAV。
- 更新 database。
- 啟動 `clamd`。
- 掃描 EICAR test file。
- Quarantine / Restore / Trash。
- 登入啟動。
- 移除 runtime。

驗證：

- 每個步驟有截圖或 log。
- 失敗時能定位到 installer、daemon、database、GUI 或權限問題。

## 18. 測試矩陣

| 場景 | 預期 |
|------|------|
| 無 ClamAV | 顯示 Homebrew 安裝引導 popup |
| 無 Homebrew | 顯示 Homebrew 官網與 `brew install clamav` 步驟 |
| Homebrew runtime 安裝完成 | `freshclam` 可更新，`clamd` socket 通過檢測後解除 popup |
| `clamd` socket 被占用 | 顯示衝突與修復指引 |
| `clamd` 停止 | GUI 顯示 repair required |
| 單檔掃描 | 透過 `INSTREAM` 完成 |
| 資料夾掃描 | 逐檔建立 progress event |
| 掃描取消 | job 變 canceled |
| access denied | 顯示 skipped / permission hint |
| infected | 顯示 signature 與 actions |
| false positive | 不自動刪除，可忽略或 quarantine |
| Quarantine | 移到 per-user quarantine |
| Restore | 可回復或要求改名 |
| Trash | 使用 macOS native trash API |
| 多使用者 shared runtime | 共用 database，各自 results / quarantine |
| A 使用者掃描 | B 使用者看不到結果與 quarantine |
| 更新排程 | shared updater 不重複執行 |
| 掃描排程 | per-user job 到點執行 |
| battery policy | 依設定延後或執行 |
| Low Power Mode | 依設定延後或執行 |
| login item | 每位使用者各自生效 |
| 移除 runtime | 只顯示移除提示，不刪 runtime |
| external Homebrew runtime | 作為主要支援路線，不刪除 |
| per-user runtime | 顯示非主線診斷 |

## 19. 不列入首輪

- macOS 官方意義的 real-time protection。
- 直接 link `libclamav`。
- per-user runtime installer。
- 無 admin 權限安裝 shared runtime。
- 遠端 TCP `clamd`。
- 預設永久刪除。
- 自動批次刪除 infected files。
- Mac App Store 版。
- enterprise MDM profile。

## 20. 建議開發順序

1. Wails skeleton。
2. Runtime resolver。
3. Runtime bundle pipeline。
4. Shared runtime installer。
5. LaunchDaemon：`freshclam` / `clamd`。
6. `FreshclamService`。
7. `ClamdClient` + `INSTREAM`。
8. Scan job manager。
9. Results / quarantine / file actions。
10. Dashboard / Scan / Results GUI。
11. Settings store / Settings GUI。
12. Scheduler / update scheduler / power policy。
13. Status bar helper。
14. Login item。
15. Uninstaller。
16. Signing / notarization。
17. Clean VM smoke test。

## 21. 交付物

- macOS `.app`。
- Homebrew ClamAV setup guidance。
- runtime setup status API。
- NSStatusItem status bar helper。
- SMAppService login item bridge。
- user LaunchAgent plist fallback。
- technical README。
- troubleshooting guide。
- release smoke test report。
