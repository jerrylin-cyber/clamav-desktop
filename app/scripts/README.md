# scripts/

此目錄包含三支本機開發用的輔助腳本。所有腳本均需在 `app/` 目錄層級執行，或由腳本自行切換工作目錄。

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
