#!/usr/bin/env bash
#
# Build the collector. Two channels, kept apart:
#   dev     -> dev channel. dist/glow-collector-dev<N>[.exe], version "dev<N>".
#              Auto-bumps the DEV counter after a successful build.
#   release -> dist/glow-live-v<VERSION>-<os>-x64[.exe], version "v<VERSION>".
#
# App ids come from .appids (never committed). Linux needs libwebkit2gtk-4.1-dev;
# Windows cross-build needs mingw-w64.
#
# Usage:
#   bash scripts/build.sh linux dev
#   bash scripts/build.sh windows dev
#   bash scripts/build.sh linux release
#   bash scripts/build.sh windows release
#
set -euo pipefail
cd "$(dirname "$0")/.."

OS="${1:?usage: build.sh <linux|windows> <dev|release>}"
CH="${2:?usage: build.sh <linux|windows> <dev|release>}"

source ./.appids
P=github.com/glow-moe/glow-collector/internal/orchestrator

if [ "$CH" = "dev" ]; then
  DEV="$(cat DEV 2>/dev/null || echo 41)"
  VER="dev$DEV"
  NAME="glow-collector-dev$DEV"
elif [ "$CH" = "release" ]; then
  VER="v$(cat VERSION)"
  NAME="glow-live-$VER-$OS-x64"
else
  echo "unknown channel: $CH (dev|release)" >&2; exit 1
fi

LD="-X main.version=$VER \
  -X $P.appGlow=$APP_GLOW -X $P.appLoL=$APP_LOL \
  -X $P.appForzaH6=$APP_FH6 -X $P.appForzaH5=$APP_FH5"

mkdir -p dist
case "$OS" in
  linux)
    PKG_CONFIG_PATH="$PWD/.pkgconfig-shim:${PKG_CONFIG_PATH:-}" CGO_ENABLED=1 \
      go build -ldflags "$LD" -o "dist/$NAME" ./cmd/gui
    ;;
  windows)
    CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
      CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
      CGO_CXXFLAGS="-I$PWD/.winshim" CGO_CPPFLAGS="-I$PWD/.winshim" \
      go build -ldflags "-H windowsgui $LD" -o "dist/$NAME.exe" ./cmd/gui
    NAME="$NAME.exe"
    ;;
  *)
    echo "unknown os: $OS (linux|windows)" >&2; exit 1
    ;;
esac

echo "▸ built  $VER  ->  dist/$NAME"

# Bump the dev counter for the next dev build (41 -> 42).
if [ "$CH" = "dev" ]; then
  echo "$((DEV + 1))" > DEV
  echo "▸ next dev -> dev$(cat DEV)"
fi
