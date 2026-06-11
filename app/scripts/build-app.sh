#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

WAILS_BIN="${WAILS_BIN:-wails}"
"$WAILS_BIN" build -skipbindings
