# ClamAV Desktop

macOS 桌面應用，為 [ClamAV](https://www.clamav.net/) 提供原生 GUI 操作介面。基於 **Wails v2**（Go 1.23 + React 18 + TypeScript）建置。

## 功能

- **儀表板** — 系統總覽：執行環境模式、健康檢查（clamd PING、binary、config、database、socket）、病毒碼資料庫狀態、最近掃描摘要
- **檔案/資料夾掃描** — 透過 clamd `INSTREAM` 協定傳送檔案，支援即時進度回報與取消掃描
- **掃描結果** — 依掃描任務瀏覽結果，支援過濾（全部/乾淨/感染/錯誤/略過），對感染檔案可開啟 Finder 位置、隔離、移到垃圾桶、永久刪除
- **隔離區** — 檢視隔離檔案 metadata（signature、原始路徑、偵測時間、SHA256），可還原、移到垃圾桶、永久刪除
- **掃描排程** — 每日/每週排程，錯過補跑，電源策略 defer
- **病毒碼更新** — 一鍵更新（freshclam foreground），支援即時事件推播
- **設定** — 排程開關與時間、電源策略（電池/低耗電是否執行）、登入時啟動、啟動時隱藏、背景 worker 設定、使用者資料移除、ClamAV 移除提示
- **紀錄** — App log、掃描紀錄、freshclam.log、clamd.log，可匯出診斷報告
- **執行環境管理** — Homebrew ClamAV 優先偵測；不可用時顯示阻擋式安裝/啟動引導，移除時僅提示使用者自行移除 Homebrew runtime
- **macOS 整合** — NSStatusItem 狀態列選單、SMAppService login item 優先、LaunchAgent fallback

## 專案結構

```
clamav-desktop/
├── app/                          # Wails 應用主目錄
│   ├── main.go                   # 進入點，Wails app 初始化
│   ├── app.go                    # App struct，所有 Wails 綁定方法
│   ├── background.go             # 背景排程 worker（tick-based）
│   ├── settings.go               # 設定檔讀寫（JSON，atomic write）
│   ├── freshclam.go              # 病毒碼更新服務（呼叫 freshclam）
│   ├── clamd_client.go           # clamd Unix socket 客戶端（PING/SCAN/INSTREAM）
│   ├── scan_job.go               # 掃描任務管理
│   ├── file_actions.go           # 檔案操作（隔離/還原/Trash/永久刪除）
│   ├── installer.go              # ClamAV 安裝/解除
│   ├── runtime.go                # 執行環境偵測（system-shared / Homebrew / per-user / missing）
│   ├── scheduler.go              # 掃描排程 + 病毒碼更新排程
│   ├── power_policy.go           # 電源狀態偵測與 defer 策略
│   ├── dialogs.go                # 原生檔案選擇對話框
│   ├── logs.go                   # App log / shared log 讀寫
│   ├── user_data.go              # 使用者資料移除
│   ├── darwin_uniform_type_identifiers.go  # macOS UTI
│   ├── go.mod / go.sum
│   ├── *_test.go                 # Go 單元測試
│   └── frontend/                 # React + TypeScript 前端
│       ├── index.html
│       ├── vite.config.ts
│       ├── tsconfig.json
│       ├── package.json
│       └── src/
│           ├── main.tsx          # React entry
│           ├── App.tsx           # SPA 元件（首頁、掃描、結果、隔離區、排程、設定、紀錄、關於）
│           ├── App.css
│           └── style.css         # 全域樣式 + Nunito 字體
│
├── docs/
└── .github/workflows/            # CI pipeline
```

## 前置需求

- macOS（Apple Silicon 或 Intel）
- [ClamAV](https://www.clamav.net/downloads)（可透過 Homebrew：`brew install clamav`）
- [Go 1.23+](https://go.dev/dl/)
- [Node.js 22+](https://nodejs.org/)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)

## 安裝相依

```bash
cd app/frontend && npm ci
```

## 開發

```bash
cd app

# 開發模式（前端 hot reload）
wails dev

# 前端獨立開發
cd frontend && npm run dev

# TypeScript 型別檢查
cd frontend && npm run typecheck
```

## 驗證

```bash
cd app && ./scripts/check.sh
```

這會依序執行 Go 單元測試、`go vet`、前端型別檢查與 Vite build。

## 建置

```bash
cd app && wails build
```

產出 `.app` 位於 `app/build/bin/ClamAV Desktop.app`。

如果已經有既有 app bundle，只想快速覆蓋最新 binary：

```bash
cd app && ./scripts/build-app.sh
```

這個腳本會重建前端、重新編譯 production binary、覆蓋現有 app bundle 內的執行檔，並在可用時做 ad-hoc codesign。

## 安裝

正式 release：

1. 到 GitHub Releases 下載 `ClamAV-Desktop-<version>-macos.zip`
2. 解壓縮後把 `ClamAV Desktop.app` 拖到 `/Applications`
3. 第一次啟動前，若系統顯示來自網路下載的提示，先在 Finder 對 App 按右鍵，選「打開」
4. 之後可從 Launchpad、Spotlight 或 `/Applications` 啟動

本機開發 build（推薦）：

1. 目前 `wails build` 與 `./scripts/build-app.sh` 產出的 App 是 ad-hoc 簽署，不是 Apple notarized release
2. 若看到 Gatekeeper 警告，請用 Finder 右鍵「打開」一次，或在測試機自行移除 quarantine attribute
3. 正式對外發布時，請改用 Developer ID signing + notarization 流程；完整步驟見 `docs/developer-id-release.md`

## 測試

```bash
cd app && go test ./...
```

若在全新環境先跑 Go 測試，請先執行一次前端 build：

```bash
cd app/frontend && npm run build
```

原因是 Wails 會在編譯時 embed `frontend/dist`。

## CI

GitHub Actions 目前在發佈 GitHub Release 時執行：前端安裝、typecheck、build、`go test`、`go vet`、`wails build -skipbindings`。

tag release 產物目前會附上 macOS zip，但若未接上 Developer ID signing 與 notarization，仍可能被 Gatekeeper 警告。

## 授權

MIT
