#!/bin/sh
# build-runtime.sh — 從原始碼編譯 ClamAV runtime，並封裝成可供 app 使用的 .tar.zst 壓縮包。
# 用途：產生 clamav-runtime-darwin-*.tar.zst 與 runtime-metadata.json，供 app 在首次啟動時解壓安裝。
# 注意：若本機已透過 Homebrew 安裝可用的 ClamAV（一般執行模式），通常不需要執行此腳本。
#       此腳本主要用於產生 runtime artifact（例如 CI/發佈流程）。
# 使用方式：./scripts/build-runtime.sh
# 環境變數（均可選）：
#   CLAMAV_VERSION      — ClamAV 版本（預設：1.5.2）
#   RUNTIME_ARCHS       — 目標架構，可為 arm64 / x86_64 / arm64;x86_64（預設：arm64;x86_64 通用 binary）
#   RUNTIME_WORK_DIR    — 建置暫存目錄（預設：build/runtime-work）
#   RUNTIME_OUT_DIR     — 產出目錄（預設：build/runtime-artifacts）
#   CLAMAV_SOURCE_URL   — 覆蓋原始碼下載 URL
set -eu

# 版本與路徑設定
CLAMAV_VERSION="${CLAMAV_VERSION:-1.5.2}"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="${RUNTIME_WORK_DIR:-$ROOT_DIR/build/runtime-work}"
OUT_DIR="${RUNTIME_OUT_DIR:-$ROOT_DIR/build/runtime-artifacts}"
SOURCE_URL="${CLAMAV_SOURCE_URL:-https://www.clamav.net/downloads/production/clamav-${CLAMAV_VERSION}.tar.gz}"
RUNTIME_ARCHS="${RUNTIME_ARCHS:-arm64;x86_64}"

# 根據目標架構決定產出檔名與 metadata 中的 arch 陣列
case "$RUNTIME_ARCHS" in
  "arm64;x86_64"|"x86_64;arm64")
    # 同時包含兩種架構 → 產生 universal binary
    ARTIFACT_NAME="clamav-runtime-darwin-universal"
    ARCH_JSON='["arm64", "x86_64"]'
    ;;
  "arm64")
    ARTIFACT_NAME="clamav-runtime-darwin-arm64"
    ARCH_JSON='["arm64"]'
    ;;
  "x86_64")
    ARTIFACT_NAME="clamav-runtime-darwin-x86_64"
    ARCH_JSON='["x86_64"]'
    ;;
  *)
    printf 'Unsupported RUNTIME_ARCHS: %s\n' "$RUNTIME_ARCHS" >&2
    printf 'Use arm64, x86_64, or arm64;x86_64.\n' >&2
    exit 1
    ;;
esac

# ClamAV 包含 Rust 元件；rustup 安裝的 cargo 不在預設 PATH，手動載入環境
if [ -f "$HOME/.cargo/env" ]; then
  # shellcheck disable=SC1091
  . "$HOME/.cargo/env"
fi

# 檢查必要工具是否存在
require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

require_command curl
require_command tar
require_command cmake
require_command cargo
require_command rustc
require_command zstd    # 用於最終壓縮產出
require_command shasum  # 產生 SHA-256 checksum

# 確認 rustup 是否已加入所需的交叉編譯 target
require_rust_target() {
  if ! command -v rustup >/dev/null 2>&1; then
    return
  fi

  if ! rustup target list --installed | grep -qx "$1"; then
    printf 'Missing Rust target: %s\n' "$1" >&2
    printf 'Run: rustup target add %s\n' "$1" >&2
    exit 1
  fi
}

# 確認指定的 Mach-O binary 包含所需架構（用於驗證 Homebrew 相依函式庫）
require_macho_arch() {
  path="$1"
  arch="$2"

  if [ ! -f "$path" ]; then
    return
  fi

  if ! lipo -verify_arch "$arch" "$path" >/dev/null 2>&1; then
    printf 'Dependency does not contain %s: %s\n' "$arch" "$path" >&2
    printf 'Set RUNTIME_ARCHS=%s for a native build or provide universal dependencies.\n' "$(uname -m)" >&2
    exit 1
  fi
}

# 依照目標架構確認對應的 Rust target 已安裝
case "$RUNTIME_ARCHS" in
  *arm64*) require_rust_target aarch64-apple-darwin ;;
esac

case "$RUNTIME_ARCHS" in
  *x86_64*) require_rust_target x86_64-apple-darwin ;;
esac

# 若 Homebrew 可用，設定 CMake 與 pkg-config 搜尋路徑，指向 Homebrew 安裝的相依套件
if command -v brew >/dev/null 2>&1; then
  BREW_PREFIX="$(brew --prefix)"
  export CMAKE_PREFIX_PATH="$BREW_PREFIX/opt/bzip2;$BREW_PREFIX/opt/check;$BREW_PREFIX/opt/curl;$BREW_PREFIX/opt/json-c;$BREW_PREFIX/opt/libxml2;$BREW_PREFIX/opt/ncurses;$BREW_PREFIX/opt/openssl@3;$BREW_PREFIX/opt/pcre2;$BREW_PREFIX/opt/zlib${CMAKE_PREFIX_PATH:+;$CMAKE_PREFIX_PATH}"
  export PKG_CONFIG_PATH="$BREW_PREFIX/opt/bzip2/lib/pkgconfig:$BREW_PREFIX/opt/check/lib/pkgconfig:$BREW_PREFIX/opt/curl/lib/pkgconfig:$BREW_PREFIX/opt/json-c/lib/pkgconfig:$BREW_PREFIX/opt/libxml2/lib/pkgconfig:$BREW_PREFIX/opt/ncurses/lib/pkgconfig:$BREW_PREFIX/opt/openssl@3/lib/pkgconfig:$BREW_PREFIX/opt/pcre2/lib/pkgconfig:$BREW_PREFIX/opt/zlib/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
  OPENSSL_ROOT_DIR="$BREW_PREFIX/opt/openssl@3"

  # 若需要 x86_64，驗證 Homebrew 相依函式庫確實包含 x86_64 slice（M1 Mac 上預設只有 arm64）
  case "$RUNTIME_ARCHS" in
    *x86_64*)
      require_command lipo
      require_macho_arch "$BREW_PREFIX/opt/openssl@3/lib/libssl.dylib" x86_64
      require_macho_arch "$BREW_PREFIX/opt/openssl@3/lib/libcrypto.dylib" x86_64
      require_macho_arch "$BREW_PREFIX/opt/libxml2/lib/libxml2.dylib" x86_64
      require_macho_arch "$BREW_PREFIX/opt/json-c/lib/libjson-c.dylib" x86_64
      ;;
  esac
else
  OPENSSL_ROOT_DIR=""
fi

# 建立工作目錄與產出目錄
mkdir -p "$WORK_DIR" "$OUT_DIR"

# 路徑定義
SOURCE_ARCHIVE="$WORK_DIR/clamav-${CLAMAV_VERSION}.tar.gz"
SOURCE_DIR="$WORK_DIR/clamav-${CLAMAV_VERSION}"
BUILD_DIR="$WORK_DIR/build"
STAGE_DIR="$WORK_DIR/stage"

# 下載原始碼（若已存在則跳過）
if [ ! -f "$SOURCE_ARCHIVE" ]; then
  curl -fL "$SOURCE_URL" -o "$SOURCE_ARCHIVE"
fi

# 清除上次建置殘留，確保乾淨編譯
rm -rf "$SOURCE_DIR" "$BUILD_DIR" "$STAGE_DIR"
tar -xzf "$SOURCE_ARCHIVE" -C "$WORK_DIR"
mkdir -p "$BUILD_DIR"

# 執行 CMake 設定；若有 OpenSSL root 則明確指定，否則依賴 CMake 自動尋找
if [ -n "$OPENSSL_ROOT_DIR" ]; then
  cmake -S "$SOURCE_DIR" -B "$BUILD_DIR" \
    -G Ninja \
    -D CMAKE_BUILD_TYPE=Release \
    -D CMAKE_OSX_ARCHITECTURES="$RUNTIME_ARCHS" \
    -D CMAKE_INSTALL_PREFIX="$STAGE_DIR/Runtime" \
    -D APP_CONFIG_DIRECTORY="$STAGE_DIR/Config" \
    -D DATABASE_DIRECTORY="$STAGE_DIR/Database" \
    -D OPENSSL_ROOT_DIR="$OPENSSL_ROOT_DIR"
else
  cmake -S "$SOURCE_DIR" -B "$BUILD_DIR" \
    -G Ninja \
    -D CMAKE_BUILD_TYPE=Release \
    -D CMAKE_OSX_ARCHITECTURES="$RUNTIME_ARCHS" \
    -D CMAKE_INSTALL_PREFIX="$STAGE_DIR/Runtime" \
    -D APP_CONFIG_DIRECTORY="$STAGE_DIR/Config" \
    -D DATABASE_DIRECTORY="$STAGE_DIR/Database"
fi

# 編譯並安裝到 stage 目錄
cmake --build "$BUILD_DIR" --config Release
cmake --build "$BUILD_DIR" --target install

# 產出路徑
ARTIFACT_PATH="$OUT_DIR/${ARTIFACT_NAME}.tar.zst"
CHECKSUM_PATH="$OUT_DIR/${ARTIFACT_NAME}.sha256"
METADATA_PATH="$OUT_DIR/runtime-metadata.json"

# 將 Runtime / Config / Database 三個目錄封裝成高壓縮率的 .tar.zst
tar -C "$STAGE_DIR" -cf - Runtime Config Database | zstd -19 -o "$ARTIFACT_PATH"
# 產生 SHA-256 checksum，app 安裝時會驗證完整性
shasum -a 256 "$ARTIFACT_PATH" | awk '{print $1}' > "$CHECKSUM_PATH"

SHA256="$(cat "$CHECKSUM_PATH")"
BUILT_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

# 寫入 runtime-metadata.json，app 啟動時讀取此檔案決定是否需要更新 runtime
cat > "$METADATA_PATH" <<EOF
{
  "name": "$ARTIFACT_NAME",
  "clamavVersion": "$CLAMAV_VERSION",
  "arch": $ARCH_JSON,
  "builtAt": "$BUILT_AT",
  "sha256": "$SHA256",
  "sourceUrl": "$SOURCE_URL"
}
EOF

printf '%s\n' "$ARTIFACT_PATH"
