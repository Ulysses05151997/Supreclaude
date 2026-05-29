#!/usr/bin/env bash
# Cross-compile the Mederos CRM binary for each target OS.
# Pure-Go (CGO disabled) so no C toolchain is needed.
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p dist

LDFLAGS="-s -w"
PKG="./cmd/crm"

echo "Building Windows (amd64) -> dist/crm.exe"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/crm.exe "$PKG"

echo "Building macOS (arm64)   -> dist/crm-mac-arm64"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o dist/crm-mac-arm64 "$PKG"

echo "Building macOS (amd64)   -> dist/crm-mac-amd64"
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/crm-mac-amd64 "$PKG"

echo "Building Linux (amd64)   -> dist/crm-linux-amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/crm-linux-amd64 "$PKG"

echo "Done. Artifacts in ./dist"
