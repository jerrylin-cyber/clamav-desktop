#!/bin/sh
set -eu

CLAMAV_VERSION="${CLAMAV_VERSION:-1.5.2}"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="${RUNTIME_WORK_DIR:-$ROOT_DIR/build/runtime-work}"
OUT_DIR="${RUNTIME_OUT_DIR:-$ROOT_DIR/build/runtime-artifacts}"
SOURCE_URL="${CLAMAV_SOURCE_URL:-https://www.clamav.net/downloads/production/clamav-${CLAMAV_VERSION}.tar.gz}"
ARTIFACT_NAME="clamav-runtime-darwin-universal"

mkdir -p "$WORK_DIR" "$OUT_DIR"

SOURCE_ARCHIVE="$WORK_DIR/clamav-${CLAMAV_VERSION}.tar.gz"
SOURCE_DIR="$WORK_DIR/clamav-${CLAMAV_VERSION}"
BUILD_DIR="$WORK_DIR/build"
STAGE_DIR="$WORK_DIR/stage"

if [ ! -f "$SOURCE_ARCHIVE" ]; then
  curl -fL "$SOURCE_URL" -o "$SOURCE_ARCHIVE"
fi

rm -rf "$SOURCE_DIR" "$BUILD_DIR" "$STAGE_DIR"
tar -xzf "$SOURCE_ARCHIVE" -C "$WORK_DIR"
mkdir -p "$BUILD_DIR"

cmake -S "$SOURCE_DIR" -B "$BUILD_DIR" \
  -D CMAKE_BUILD_TYPE=Release \
  -D CMAKE_OSX_ARCHITECTURES="arm64;x86_64" \
  -D CMAKE_INSTALL_PREFIX="$STAGE_DIR/Runtime" \
  -D APP_CONFIG_DIRECTORY="$STAGE_DIR/Config" \
  -D DATABASE_DIRECTORY="$STAGE_DIR/Database"

cmake --build "$BUILD_DIR" --config Release
cmake --build "$BUILD_DIR" --target install

ARTIFACT_PATH="$OUT_DIR/${ARTIFACT_NAME}.tar.zst"
CHECKSUM_PATH="$OUT_DIR/${ARTIFACT_NAME}.sha256"
METADATA_PATH="$OUT_DIR/runtime-metadata.json"

tar -C "$STAGE_DIR" -cf - Runtime Config Database | zstd -19 -o "$ARTIFACT_PATH"
shasum -a 256 "$ARTIFACT_PATH" | awk '{print $1}' > "$CHECKSUM_PATH"

SHA256="$(cat "$CHECKSUM_PATH")"
BUILT_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

cat > "$METADATA_PATH" <<EOF
{
  "name": "$ARTIFACT_NAME",
  "clamavVersion": "$CLAMAV_VERSION",
  "arch": ["arm64", "x86_64"],
  "builtAt": "$BUILT_AT",
  "sha256": "$SHA256",
  "sourceUrl": "$SOURCE_URL"
}
EOF

printf '%s\n' "$ARTIFACT_PATH"
