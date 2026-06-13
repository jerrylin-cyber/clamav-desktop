#!/usr/bin/env bash
# uninstall-app.sh — 移除 ClamAV Desktop app 與其設定檔，並檢查 ClamAV runtime 的解除安裝方式。
# 流程：
#   1. 偵測 app（含狀態列）是否仍在執行，提示並協助完全關閉
#   2. 移除 app bundle 與 app 自身的設定檔／LaunchAgents
#   3. 偵測 app 管理的系統 runtime（LaunchDaemons，需 sudo）並提示移除
#   4. 偵測 ClamAV runtime 安裝位置，提示對應的解除安裝方式（不會自動移除）
# 使用方式：
#   ./scripts/uninstall-app.sh        # 互動模式，破壞性操作前會逐項確認
#   ./scripts/uninstall-app.sh -y     # 對 app 與 app 設定檔的移除自動回答 yes（仍不會自動 sudo 或移除 ClamAV）
set -eu

APP_NAME="ClamAV Desktop"
APP_BUNDLE_NAME="ClamAV Desktop.app"
APP_EXECUTABLE="clamav-desktop"
BUNDLE_ID="studio.crazyjerry.clamavdesktop"

ASSUME_YES=0
for arg in "$@"; do
  case "$arg" in
    -y|--yes) ASSUME_YES=1 ;;
    -h|--help)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *)
      printf '未知參數：%s（使用 -h 查看說明）\n' "$arg" >&2
      exit 2
      ;;
  esac
done

# 僅在輸出為終端機時上色，避免污染重新導向的輸出
if [ -t 1 ]; then
  C_RESET="$(printf '\033[0m')"; C_INFO="$(printf '\033[36m')"
  C_WARN="$(printf '\033[33m')"; C_OK="$(printf '\033[32m')"; C_ERR="$(printf '\033[31m')"
else
  C_RESET=""; C_INFO=""; C_WARN=""; C_OK=""; C_ERR=""
fi
info() { printf '%s==>%s %s\n' "$C_INFO" "$C_RESET" "$1"; }
warn() { printf '%s[!]%s %s\n' "$C_WARN" "$C_RESET" "$1"; }
ok()   { printf '%s[✓]%s %s\n' "$C_OK" "$C_RESET" "$1"; }
err()  { printf '%s[✗]%s %s\n' "$C_ERR" "$C_RESET" "$1" >&2; }

# 互動確認；非互動（-y）時對 app 移除自動回 yes。讀取 /dev/tty 以支援管線執行。
confirm() {
  if [ "$ASSUME_YES" = "1" ]; then
    return 0
  fi
  printf '%s [y/N] ' "$1"
  if ! read -r reply </dev/tty 2>/dev/null; then
    return 1
  fi
  case "$reply" in
    [yY]|[yY][eE][sS]) return 0 ;;
    *) return 1 ;;
  esac
}

HOME_DIR="${HOME:?HOME 未設定}"
APP_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# ---------------------------------------------------------------------------
# 步驟一：偵測 app 是否執行，協助完全關閉（含狀態列）
# ---------------------------------------------------------------------------
app_running() {
  pgrep -x "$APP_EXECUTABLE" >/dev/null 2>&1 && return 0
  pgrep -f "$APP_BUNDLE_NAME/Contents/MacOS/$APP_EXECUTABLE" >/dev/null 2>&1
}

info "步驟一：檢查 $APP_NAME 是否正在執行"
if app_running; then
  warn "$APP_NAME 正在執行（可能僅常駐於狀態列）。移除前必須完全關閉，包含狀態列圖示。"

  # 先嘗試以 AppleScript 正常結束（會一併關閉狀態列）
  osascript -e "tell application \"$APP_NAME\" to quit" >/dev/null 2>&1 || true

  # 等待最多約 8 秒讓程式自行結束
  waited=0
  while app_running && [ "$waited" -lt 8 ]; do
    sleep 1
    waited=$((waited + 1))
  done

  if app_running; then
    warn "$APP_NAME 仍在執行。你可以點擊狀態列圖示選擇「結束」，或由本腳本強制終止。"
    if confirm "要強制終止 $APP_NAME 程序嗎？"; then
      pkill -x "$APP_EXECUTABLE" 2>/dev/null || true
      sleep 2
      if app_running; then
        pkill -9 -x "$APP_EXECUTABLE" 2>/dev/null || true
        sleep 1
      fi
      if app_running; then
        err "無法終止 $APP_NAME，請手動關閉後再執行本腳本。"
        exit 1
      fi
      ok "已終止 $APP_NAME。"
    else
      err "請先完全關閉 $APP_NAME（含狀態列）後再執行本腳本。"
      exit 1
    fi
  else
    ok "$APP_NAME 已關閉。"
  fi
else
  ok "$APP_NAME 未在執行。"
fi

# ---------------------------------------------------------------------------
# 步驟二：移除 app bundle 與 app 設定檔
# ---------------------------------------------------------------------------
info "步驟二：移除 app 與 app 設定檔"

# app bundle 可能存在的位置（含本專案 build 輸出）
APP_BUNDLE_CANDIDATES="
/Applications/$APP_BUNDLE_NAME
$HOME_DIR/Applications/$APP_BUNDLE_NAME
$APP_DIR/build/bin/$APP_BUNDLE_NAME
"

# app 自身的設定檔／資料／LaunchAgents（皆為本 app 專屬路徑）
LAUNCH_AGENTS="
$HOME_DIR/Library/LaunchAgents/com.lazyjerry.clamavdesktop.agent.plist
$HOME_DIR/Library/LaunchAgents/com.lazyjerry.clamavdesktop.clamscan-downloads.plist
"
CONFIG_PATHS="
$HOME_DIR/Library/Application Support/ClamAVDesktop
$HOME_DIR/Library/Logs/ClamAVDesktop
"

# 蒐集實際存在的目標
existing_targets=""
for p in $APP_BUNDLE_CANDIDATES $LAUNCH_AGENTS $CONFIG_PATHS; do
  [ -e "$p" ] && existing_targets="$existing_targets$p
"
done

if [ -z "$existing_targets" ]; then
  ok "未發現 app bundle 或設定檔，可能已移除。"
else
  printf '即將移除以下項目：\n'
  printf '%s' "$existing_targets" | sed 's/^/  - /'
  if confirm "確定要移除上述 app 與設定檔嗎？此操作無法復原。"; then
    # 先卸載 LaunchAgents，再刪除 plist
    for plist in $LAUNCH_AGENTS; do
      if [ -e "$plist" ]; then
        launchctl unload "$plist" >/dev/null 2>&1 || true
        rm -f "$plist" && ok "已移除 LaunchAgent：$plist"
      fi
    done
    # 刪除 app bundle
    for bundle in $APP_BUNDLE_CANDIDATES; do
      if [ -e "$bundle" ]; then
        rm -rf "$bundle" && ok "已移除 app：$bundle"
      fi
    done
    # 刪除設定檔／資料目錄
    for cfg in $CONFIG_PATHS; do
      if [ -e "$cfg" ]; then
        rm -rf "$cfg" && ok "已移除設定檔：$cfg"
      fi
    done
    warn "登入項目（Login Item）若曾以「登入時啟動」註冊，移除 app 後 macOS 會自動失效；如清單仍殘留，可至「系統設定 → 一般 → 登入項目」手動移除。"
  else
    warn "已略過 app 與設定檔的移除。"
  fi
fi

# ---------------------------------------------------------------------------
# 步驟三：app 管理的系統 runtime（LaunchDaemons，需 sudo）
# ---------------------------------------------------------------------------
info "步驟三：檢查 app 管理的系統 runtime（需要管理者權限）"

SYSTEM_DAEMONS="
/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.freshclam.plist
/Library/LaunchDaemons/com.lazyjerry.clamavdesktop.clamd.plist
"
SYSTEM_PATHS="
/Library/Application Support/ClamAVDesktop
/Library/Logs/ClamAVDesktop
"

system_existing=""
for p in $SYSTEM_DAEMONS $SYSTEM_PATHS; do
  [ -e "$p" ] && system_existing="$system_existing$p
"
done

if [ -z "$system_existing" ]; then
  ok "未發現 app 管理的系統 runtime。"
else
  warn "偵測到 app 曾安裝的系統層級項目（需 sudo 才能移除）："
  printf '%s' "$system_existing" | sed 's/^/  - /'
  if confirm "要立即以 sudo 卸載並移除這些系統項目嗎？"; then
    for plist in $SYSTEM_DAEMONS; do
      if [ -e "$plist" ]; then
        sudo launchctl bootout system "$plist" >/dev/null 2>&1 || sudo launchctl unload "$plist" >/dev/null 2>&1 || true
        sudo rm -f "$plist" && ok "已移除系統 LaunchDaemon：$plist"
      fi
    done
    for p in $SYSTEM_PATHS; do
      if [ -e "$p" ]; then
        sudo rm -rf "$p" && ok "已移除系統路徑：$p"
      fi
    done
  else
    warn "未移除系統項目。若要手動移除，請執行："
    for plist in $SYSTEM_DAEMONS; do
      [ -e "$plist" ] && printf '    sudo launchctl bootout system %s\n    sudo rm -f %s\n' "\"$plist\"" "\"$plist\""
    done
    for p in $SYSTEM_PATHS; do
      [ -e "$p" ] && printf '    sudo rm -rf %s\n' "\"$p\""
    done
  fi
fi

# ---------------------------------------------------------------------------
# 步驟四：偵測 ClamAV runtime 安裝位置，提示解除安裝方式（不自動移除）
# ---------------------------------------------------------------------------
info "步驟四：檢查 ClamAV runtime 安裝位置"
warn "$APP_NAME 不會自動移除 Homebrew 或官方安裝的 ClamAV，以下僅提示解除安裝方式。"

found_clamav=0

# Homebrew（Apple Silicon / Intel）
for prefix in /opt/homebrew /usr/local; do
  if [ -e "$prefix/opt/clamav" ] || [ -x "$prefix/bin/clamscan" ]; then
    found_clamav=1
    ok "偵測到 Homebrew ClamAV：$prefix/opt/clamav"
    printf '    解除安裝：\n'
    printf '      brew uninstall clamav\n'
    printf '    （選用）一併移除病毒碼資料庫與設定：\n'
    printf '      rm -rf %s/var/lib/clamav %s/etc/clamav\n' "$prefix" "$prefix"
  fi
done

# ClamAV 官方 pkg
if [ -e /usr/local/clamav ]; then
  found_clamav=1
  ok "偵測到 ClamAV 官方安裝：/usr/local/clamav"
  printf '    解除安裝（需 sudo，請先確認該目錄確由 ClamAV 官方安裝建立）：\n'
  printf '      sudo rm -rf /usr/local/clamav\n'
fi

# 其他位置的 clamscan（提示參考）
if [ "$found_clamav" -eq 0 ]; then
  if command -v clamscan >/dev/null 2>&1; then
    found_clamav=1
    clam_path="$(command -v clamscan)"
    warn "偵測到 clamscan，但不在已知的 Homebrew／官方路徑：$clam_path"
    printf '    請依你的安裝方式自行移除（例如套件管理器或手動安裝）。\n'
  else
    ok "未偵測到 ClamAV runtime。"
  fi
fi

info "完成。"
