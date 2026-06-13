#!/usr/bin/env bash
# build-app.sh — 編譯前端與 Go binary，並注入既有的 Wails app bundle。
# 用途：在 `wails build` 產生初始 app bundle 之後，快速重建 binary 而不需完整執行 wails build。
# 使用方式：./scripts/build-app.sh
# 環境變數（均可選）：
#   APP_BUNDLE  — 覆蓋目標 app bundle 路徑（預設：build/bin/ClamAV Desktop.app）
#   GOCACHE     — 覆蓋 Go 編譯快取目錄（預設：build/go-cache）
set -eu

# 將工作目錄切換到 app 根目錄，確保相對路徑正確
APP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$APP_DIR"

# 使用專案內建的 go-cache 目錄，避免污染全域快取
export GOCACHE="${GOCACHE:-$PWD/build/go-cache}"
# macOS 需要連結 UniformTypeIdentifiers framework（檔案選擇器相依）
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -framework UniformTypeIdentifiers"

# app bundle 與 binary 路徑
APP_BUNDLE="${APP_BUNDLE:-$APP_DIR/build/bin/ClamAV Desktop.app}"
APP_EXECUTABLE="$APP_BUNDLE/Contents/MacOS/clamav-desktop"
# 先輸出到暫存檔，成功後再替換正式 binary，避免建置中途 app 處於破損狀態
TEMP_EXECUTABLE="$APP_DIR/build/bin/clamav-desktop.next"

# 確認 app bundle 已存在；此腳本不負責從零建立 bundle
if [ ! -d "$APP_BUNDLE/Contents/MacOS" ]; then
  printf '找不到 app bundle：%s\n' "$APP_BUNDLE" >&2
  printf '請先建立一次 Wails app bundle，再執行此腳本覆蓋最新 binary。\n' >&2
  exit 1
fi

# 步驟一：編譯前端（Vite build → wailsjs 靜態資源）
printf '==> Building frontend\n'
(cd frontend && npm run build)

# 步驟二：以 production tags 編譯 Go binary
# -buildvcs=false 避免在 Dropbox 路徑下因 VCS 偵測失敗而報錯
# -tags production  啟用 Wails production 模式（embed 靜態資源）
printf '==> Building production binary\n'
mkdir -p "$APP_DIR/build/bin"
go build \
  -buildvcs=false \
  -tags "desktop,wv2runtime.download,production" \
  -ldflags "-w -s" \
  -o "$TEMP_EXECUTABLE" \
  .

# 步驟三：用暫存 binary 替換 bundle 內的執行檔
printf '==> Overwriting app executable\n'
cp "$TEMP_EXECUTABLE" "$APP_EXECUTABLE"
chmod 755 "$APP_EXECUTABLE"

# 步驟四（選用）：以 ad-hoc 方式重新簽署 app bundle
# 在沒有 Apple 開發者憑證的環境下，ad-hoc 簽署可讓 macOS 允許直接執行
if command -v codesign >/dev/null 2>&1; then
  printf '==> Ad-hoc codesigning app bundle\n'
  codesign --force --deep --sign - "$APP_BUNDLE"
  codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"
fi

printf '==> Done: %s\n' "$APP_BUNDLE"
