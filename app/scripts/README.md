# scripts/

此目錄包含本機開發與維運用的輔助腳本。所有腳本均需在 `app/` 目錄層級執行，或由腳本自行切換工作目錄。

---

## build-app.sh

**用途**：重建前端與 Go binary，並注入既有的 Wails app bundle。

此腳本適合在 `wails build` 首次產生 app bundle 之後，用於快速迭代，避免每次都執行完整的 `wails build`。

### 前置條件

- 已執行過一次 `wails build`，`build/bin/ClamAV Desktop.app` 存在
- Node.js 與 npm 可用
- Go 工具鏈可用
- （選用）`codesign` — 用於 ad-hoc 簽署，無 Apple 開發者憑證時也可使用

### 執行方式

```sh
./scripts/build-app.sh
```

### 可覆蓋的環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `APP_BUNDLE` | `build/bin/ClamAV Desktop.app` | 目標 app bundle 路徑 |
| `GOCACHE` | `build/go-cache` | Go 編譯快取目錄 |
| `CGO_LDFLAGS` | `-framework UniformTypeIdentifiers` | 額外的 C linker flags |

### 執行流程

1. **編譯前端** — `npm run build`（Vite 輸出靜態資源）
2. **編譯 Go binary** — production tags，包含嵌入靜態資源
3. **注入 binary** — 覆蓋 `Contents/MacOS/clamav-desktop`
4. **Ad-hoc 簽署** — 若系統有 `codesign` 則自動執行

---

## build-runtime.sh

**用途**：從原始碼編譯 ClamAV runtime，封裝成 `.tar.zst` 壓縮包，供 app 首次啟動時下載安裝。

### 前置條件

| 工具 | 說明 |
|------|------|
| `curl` | 下載原始碼 |
| `cmake` + `ninja` | 建置系統 |
| `cargo` + `rustc` | ClamAV 包含 Rust 元件 |
| `zstd` | 高壓縮率封裝 |
| `shasum` | 產生 SHA-256 checksum |
| Homebrew 套件 | `bzip2 check curl json-c libxml2 ncurses openssl@3 pcre2 zlib` |

若要建置 universal binary（`arm64;x86_64`），Homebrew 相依函式庫需包含 x86_64 slice（需在 x86_64 Mac 或使用 universal Homebrew 安裝）。

### 執行方式

```sh
# 預設：universal binary（arm64 + x86_64）
./scripts/build-runtime.sh

# 僅建置 arm64（M 系列 Mac）
RUNTIME_ARCHS=arm64 ./scripts/build-runtime.sh

# 指定 ClamAV 版本
CLAMAV_VERSION=1.4.1 ./scripts/build-runtime.sh
```

### 可覆蓋的環境變數

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `CLAMAV_VERSION` | `1.5.2` | ClamAV 版本號 |
| `RUNTIME_ARCHS` | `arm64;x86_64` | 目標架構 |
| `RUNTIME_WORK_DIR` | `build/runtime-work` | 建置暫存目錄 |
| `RUNTIME_OUT_DIR` | `build/runtime-artifacts` | 產出目錄 |
| `CLAMAV_SOURCE_URL` | ClamAV 官方下載 URL | 覆蓋原始碼來源 |

### 產出檔案

```
build/runtime-artifacts/
├── clamav-runtime-darwin-universal.tar.zst   # runtime 壓縮包
├── clamav-runtime-darwin-universal.sha256    # SHA-256 checksum
└── runtime-metadata.json                    # 版本與架構 metadata
```

---

## check.sh

**用途**：提交前的完整驗證，確保 Go 與前端程式碼均可正常建置且通過檢查。

### 執行內容

| 步驟 | 指令 | 說明 |
|------|------|------|
| Go 單元測試 | `go test ./...` | 執行所有套件的測試 |
| Go 靜態分析 | `go vet ./...` | 檢查常見錯誤模式 |
| TS 型別檢查 | `npm run typecheck` | 確保前端型別正確 |
| 前端建置 | `npm run build` | 確保 Vite 建置無錯誤 |

### 執行方式

```sh
./scripts/check.sh
```

任一步驟失敗即中止（`set -e`），並回傳非零 exit code。

---

## uninstall-app.sh

**用途**：移除 ClamAV Desktop app 與其設定檔，並提示 ClamAV runtime 的解除安裝方式。

### 執行方式

```sh
# 互動模式：破壞性操作前逐項確認（建議）
./scripts/uninstall-app.sh

# 對 app 與 app 設定檔的移除自動回答 yes（仍不會自動 sudo 或移除 ClamAV）
./scripts/uninstall-app.sh -y
```

### 執行流程

1. **偵測執行狀態** — 若 app（含狀態列）仍在執行，先嘗試正常結束；必要時詢問是否強制終止
2. **移除 app 與設定檔** — 刪除 app bundle、`~/Library/Application Support/ClamAVDesktop`、`~/Library/Logs/ClamAVDesktop` 與兩個 LaunchAgents
3. **系統 runtime（需 sudo）** — 偵測 app 曾安裝的 `/Library/LaunchDaemons/*clamavdesktop*` 與系統路徑，詢問是否以 sudo 移除（否則印出手動指令）
4. **ClamAV runtime** — 偵測 Homebrew／官方安裝位置，提示 `brew uninstall clamav` 等解除安裝方式（**不會自動移除**）

### 移除範圍

| 類別 | 路徑 | 移除方式 |
|------|------|----------|
| app bundle | `/Applications`、`~/Applications`、`build/bin` 內的 `ClamAV Desktop.app` | 自動（確認後） |
| 設定／資料 | `~/Library/Application Support/ClamAVDesktop` | 自動（確認後） |
| 日誌 | `~/Library/Logs/ClamAVDesktop` | 自動（確認後） |
| LaunchAgents | `com.lazyjerry.clamavdesktop.agent` / `.clamscan-downloads` | 自動（確認後） |
| 系統 LaunchDaemons | `/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.{freshclam,clamd}` | sudo（確認後）或印出手動指令 |
| ClamAV runtime | Homebrew `/opt/homebrew`、`/usr/local`；官方 `/usr/local/clamav` | 僅提示，不自動移除 |
