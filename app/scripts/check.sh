#!/bin/sh
# check.sh — 執行完整的靜態分析與建置驗證，確保提交前程式碼品質。
# 包含：Go 單元測試、go vet 靜態分析、前端 TypeScript 型別檢查、前端建置驗證。
# 使用方式：./scripts/check.sh
set -eu

# 切換到 app 根目錄
cd "$(dirname "$0")/.."

# 使用專案內建的 go-cache，避免污染全域 Go 快取
export GOCACHE="${GOCACHE:-$PWD/build/go-cache}"

# 執行所有 Go 單元測試
go test ./...
# 執行 Go 靜態分析，檢查常見錯誤模式
go vet ./...

# 切換到前端目錄執行 TypeScript 型別檢查與 Vite 建置
cd frontend
npm run typecheck
npm run build
