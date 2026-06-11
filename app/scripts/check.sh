#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

go test ./...
go vet ./...

cd frontend
npm run typecheck
npm run build
