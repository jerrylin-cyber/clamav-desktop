# ClamAV Desktop 正式版 Task List

日期：2026-06-11  
範圍：Golang + Wails macOS desktop app，正式版首輪。  
主線：`app-managed system-shared runtime`、shared `freshclam`、shared `clamd`、Unix socket、`INSTREAM`。  
非主線：`per-user runtime installer`、Homebrew runtime、`clamscan` 主掃描流程。

## 0. 執行狀態

- [x] Repository：已執行 `git init`。
- [x] 檔名：已改為 `tasks.md`。
- [x] Phase 0：專案初始化。
  - [x] T0.1：建立 Wails app skeleton。
  - [x] T0.2：建立專案品質工具。
- [ ] Phase 1：Runtime Bundle。（已建立 build script / manual CI，待實際產出 artifact 後標完成）
- [ ] Phase 2：Shared Runtime Installer。
- [ ] Phase 3：Runtime Resolver。
- [ ] Phase 4：Database Update。
- [ ] Phase 5：ClamD Client。
- [ ] Phase 6：Scan Job Manager。
- [ ] Phase 7：GUI。
- [ ] Phase 8：File Actions。
- [ ] Phase 9：Settings。
- [ ] Phase 10：Scheduler / Background。
- [ ] Phase 11：Status Bar / Login。
- [ ] Phase 12：Uninstaller。
- [ ] Phase 13：Packaging / Signing / Notarization。

## 1. 成功條件

- macOS app 可顯示 GUI，並可由狀態列 icon 背景常駐。
- App 可安裝 app-managed ClamAV runtime，不要求使用者安裝 Homebrew。
- ClamAV runtime / database / daemon 位於系統共用位置。
- 每位使用者有獨立 settings、scan jobs、results、quarantine、schedule、login item。
- 掃描主流程使用 `clamd` + Unix socket + `INSTREAM`。
- `clamscan` 僅作 diagnostic fallback，不自動替代正式掃描 job。
- 可單次掃描、查詢進度、取消掃描、顯示警告、開啟位置、隔離、還原、移到 Trash。
- 可設定掃描排程、database 更新排程、省電策略、登入啟動、背景常駐。
- 移除 app-managed runtime 時，只刪除 manifest 記錄的 app-managed paths。
- 不宣稱 macOS real-time protection。

## 2. 任務規則

- `PATH` 不作為 runtime 判斷主依據，所有 binary / config / database / socket 都使用絕對路徑。
- 不使用 `/tmp/clamd.socket` 作為正式版 socket。
- 不啟用遠端 TCP `clamd`。
- 不預設永久刪除偵測檔案。
- 不把使用者 quarantine 放在 shared system path。
- `per-user runtime installer` 只保留為後續例外模式。
- 無 admin 權限時，首輪顯示「需要 admin 權限安裝 shared runtime」，不自動改走 per-user runtime。

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

- 解壓 runtime artifact 到 `/Library/Application Support/ClamAVDesktop/Runtime/`。
- 建立 `Config`、`Database`、`Run` 目錄。
- 建立 `freshclam.conf`。
- 建立 `clamd.conf`。
- 設定 `LocalSocket` 到 app-owned path。
- 設定 shared database path。

驗證：

- clean macOS 上不安裝 Homebrew 也可完成 runtime 安裝。
- 目錄權限符合 system updater 可寫 database、`clamd` 可讀 database。
- socket path 不在 `/tmp`。

### T2.2 建立 install manifest

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

### T2.3 建立 LaunchDaemon plist

- `com.lazyjerry.clamavdesktop.freshclam.plist`
- `com.lazyjerry.clamavdesktop.clamd.plist`
- 支援 load / unload / status。
- log 寫入 app-managed log path。

驗證：

- 重新開機後 `freshclam` / `clamd` 可由 launchd 管理。
- `clamd` socket 可被使用者層 app 連線。
- `freshclam` 不會由多個使用者同時寫 shared database。

## 7. Phase 3：Runtime Resolver

### T3.1 建立 `RuntimeResolver`

偵測順序：

1. app-managed system-shared runtime。
2. per-user runtime，僅顯示診斷狀態。
3. Homebrew runtime，僅顯示 external source。
4. 官方 `.pkg` `/usr/local/clamav`，僅顯示 external source。
5. manual path，僅顯示 external source。
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
- 未安裝 runtime 時顯示 Install Runtime。
- 偵測到 external runtime 時顯示來源與遷移提示。
- 偵測到 per-user runtime 時顯示 unsupported / experimental。

### T3.2 建立 Runtime Health Check

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

- 手動更新 database。
- 串流 stdout / stderr 到 GUI。
- 儲存 last updated metadata。
- 分類 network、permission、config、lock error。

驗證：

- 更新成功時 Dashboard 顯示 database 時間。
- 更新失敗時 GUI 顯示可理解錯誤。
- `freshclam` lock 時不啟動第二個 updater。

### T4.2 建立 database status model

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

## 9. Phase 5：ClamD Client

### T5.1 建立 Unix socket client

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

### T5.2 建立 `INSTREAM` file reader

- 由使用者層 app 讀取檔案。
- 使用 chunk 傳送。
- 受 `StreamMaxLength` 限制時回報可理解錯誤。
- access denied 時回報權限提示。

驗證：

- daemon 沒有直接讀取使用者 path 權限時，仍可透過 `INSTREAM` 掃描。
- 受保護目錄顯示 skipped / access denied。

## 10. Phase 6：Scan Job Manager

### T6.1 建立 scan job model

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
    Recursive      bool
    AllMatch       bool
    Heuristic      bool
    AlertEncrypted bool
}
```

驗證：

- 可建立、查詢、取消 scan job。
- 同一使用者的 job 不和其他使用者混用。

### T6.2 建立 scan progress event

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

### T6.3 建立 result parser / store

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

## 11. Phase 7：GUI

### T7.1 Dashboard

功能：

- Runtime status。
- Database status。
- `clamd` health。
- 最近掃描摘要。
- 目前掃描 job。

驗證：

- 無 runtime 顯示 Install Runtime。
- `clamd` unhealthy 顯示 Repair。
- database 過期顯示 Update。

### T7.2 Scan page

功能：

- 選檔。
- 選資料夾。
- 快速掃描 Downloads。
- Scan options。
- Start / Cancel。
- 進度顯示。

驗證：

- 可掃單檔與資料夾。
- 掃描中可取消。
- 無權限 path 顯示權限提示。

### T7.3 Results page

功能：

- 結果列表。
- filter：clean、infected、error、skipped。
- 顯示 signature、path、engine、error。
- 動作入口：Open Location、Quarantine、Restore、Move to Trash、Permanent Delete。

驗證：

- infected 結果有可行動按鈕。
- clean 結果不顯示危險動作。
- Permanent Delete 必須二次確認。

### T7.4 Quarantine page

功能：

- 隔離檔案列表。
- 查看 metadata。
- Restore。
- Move to Trash。
- Permanent Delete。

驗證：

- Restore 可回原位置。
- 原位置已有檔案時要求改名或取消。
- Quarantine 不跨使用者顯示。

### T7.5 Logs page

功能：

- App log。
- Scan log。
- `freshclam` log。
- `clamd` log。
- 匯出診斷資訊。

驗證：

- 使用者可複製或匯出 log。
- log 不包含不必要的私人檔案內容。

## 12. Phase 8：File Actions

### T8.1 Open Location

- 呼叫 `/usr/bin/open -R <path>`。
- path 必須來自掃描結果或 quarantine metadata。

驗證：

- Finder 可選取目標檔案。
- 檔案不存在時顯示已移除。

### T8.2 Quarantine / Restore

- 移動到 per-user quarantine path。
- 保存 original path、quarantine path、signature、detectedAt、sha256。
- Restore 時檢查原位置是否存在同名檔案。

驗證：

- Quarantine 後原檔不在原位置。
- Restore 後 quarantine metadata 更新。
- A 使用者看不到 B 使用者 quarantine。

### T8.3 Move to Trash / Permanent Delete

- Move to Trash 使用 macOS native trash API。
- Permanent Delete 需要二次確認與 audit log。

驗證：

- 不使用 `rm` 作為 Trash。
- Permanent Delete 有不可逆提示。

## 13. Phase 9：Settings

### T9.1 建立 `SettingsStore`

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

### T9.2 Settings model

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
- 修改 Settings 後 domain service 立即套用。

### T9.3 Settings page controls

- Runtime status。
- Install ClamAV Runtime。
- Remove App-managed Runtime。
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
- 修改排程不需重開 app。
- 登入啟動 toggle 可註冊 / 取消 login item。

## 14. Phase 10：Scheduler / Background

### T10.1 建立 `SchedulerService`

- 支援每日、每週、自訂時間。
- 支援錯過補跑。
- 支援暫停排程。
- 依使用者 settings 建立 job。

驗證：

- 排程時間到會建立 scan job。
- 登出後 per-user job 停止。
- 不掃其他使用者資料。

### T10.2 建立 `UpdateSchedulerService`

- 依 settings 執行 database update。
- 使用 jitter，避免多台或多使用者同時更新。
- shared mode 只允許 system updater 寫 database。

驗證：

- 更新排程變更後立即重建下次 trigger。
- 多使用者不會同時寫 shared database。

### T10.3 建立 `PowerPolicyService`

- 檢查 battery 狀態。
- 檢查 Low Power Mode。
- 支援 defer until charging。

驗證：

- battery 且 `runOnBattery=false` 時延後 scan/update。
- Low Power Mode 且 `runInLowPowerMode=false` 時延後 scan/update。
- 接上電源後補跑。

## 15. Phase 11：Status Bar / Login

### T11.1 建立 status bar helper

- 使用 `NSStatusItem`。
- 顯示掃描狀態。
- menu actions：
  - Open Window
  - Scan Downloads
  - Update Database
  - Pause Schedule
  - Last Result
  - Quit

驗證：

- 關閉視窗後 app 仍在狀態列。
- Quit 會停止使用者層 background process。
- 掃描中 icon 狀態會變更。

### T11.2 建立 login item

- 優先使用 `SMAppService`。
- fallback 使用 per-user LaunchAgent。
- 支援 Start Hidden。
- 支援 Keep Menu Bar Icon。

驗證：

- 開啟 Launch at Login 後重新登入會啟動。
- 關閉 Launch at Login 後重新登入不啟動。
- 多使用者設定互不影響。

## 16. Phase 12：Uninstaller

### T12.1 建立 app-managed runtime uninstaller

- 讀取 `install-manifest.json`。
- unload LaunchDaemon。
- 停止 `clamd`。
- 停止 running scan jobs。
- 刪除 manifest 內 paths。
- 不刪 external runtime。

驗證：

- shared runtime 可完整移除。
- Homebrew ClamAV 不會被刪。
- 官方 `/usr/local/clamav` 不會被刪，除非 manifest 明確記錄為 app-managed。

### T12.2 建立使用者資料移除選項

選項：

- Remove Settings。
- Remove Scan Results。
- Remove Quarantine。
- Keep User Data。

驗證：

- 預設不刪除 quarantine。
- 刪除 quarantine 需要二次確認。

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
- 安裝 app-managed runtime。
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
| 無 ClamAV | 顯示 Install Runtime |
| 無 Homebrew | 可安裝 app-managed runtime |
| shared runtime 安裝完成 | `freshclam` / `clamd` 由 LaunchDaemon 管理 |
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
| 移除 runtime | 只刪 manifest paths |
| external Homebrew runtime | 只顯示診斷，不刪除 |
| per-user runtime | 顯示 unsupported / experimental |

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
- installer package。
- app-managed ClamAV runtime artifact。
- runtime metadata。
- install manifest。
- LaunchDaemon plists。
- user LaunchAgent plist fallback。
- technical README。
- troubleshooting guide。
- release smoke test report。
