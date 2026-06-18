#!/usr/bin/env bash
# clean.sh — 清除所有 build 產物，讓專案回到乾淨狀態。
# 清除範圍：
#   build/bin/             — 編譯後的 binary 與 app bundle
#   build/go-cache/        — Go 編譯快取
#   build/runtime-artifacts/ — runtime 封裝產物
#   build/runtime-work/    — runtime 建置工作目錄
#   build/go-wrapper.log   — 建置 log
#   frontend/dist/         — Vite 靜態資源輸出
# 使用方式：
#   ./scripts/clean.sh         # 互動模式，刪除前會確認
#   ./scripts/clean.sh -y      # 跳過確認，直接刪除
set -eu

APP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$APP_DIR"

AUTO_YES=0
if [ "${1:-}" = "-y" ]; then
  AUTO_YES=1
fi

# 列出將被清除的目標
TARGETS=(
  "build/bin"
  "build/go-cache"
  "build/runtime-artifacts"
  "build/runtime-work"
  "build/go-wrapper.log"
  "frontend/dist"
)

printf '以下目錄／檔案將被清除：\n'
for t in "${TARGETS[@]}"; do
  if [ -e "$APP_DIR/$t" ]; then
    printf '  %s\n' "$t"
  else
    printf '  %s  (不存在，略過)\n' "$t"
  fi
done
printf '\n'

if [ "$AUTO_YES" -eq 0 ]; then
  printf '確認清除？[y/N] '
  read -r REPLY
  case "$REPLY" in
    [yY]) ;;
    *) printf '已取消。\n'; exit 0 ;;
  esac
fi

for t in "${TARGETS[@]}"; do
  TARGET_PATH="$APP_DIR/$t"
  if [ -e "$TARGET_PATH" ]; then
    rm -rf "$TARGET_PATH"
    printf '已刪除：%s\n' "$t"
  fi
done

printf '\n==> 清除完成。\n'
