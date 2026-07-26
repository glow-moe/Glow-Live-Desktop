#!/usr/bin/env bash
#
# Build both platforms and refresh the single rolling `dev` pre-release IN PLACE,
# so the releases list never fills up with dev83, dev84, ... The internal build
# number (dev<N>) still increments so builds stay distinguishable in the app and
# the release title.
#
# Usage: bash scripts/release-dev.sh
set -euo pipefail
cd "$(dirname "$0")/.."

N="$(cat DEV 2>/dev/null || echo 41)"

# Build both platforms at the SAME dev number (build.sh bumps DEV after each, so
# reset it before the second build).
echo "$N" > DEV; bash scripts/build.sh linux dev
echo "$N" > DEV; bash scripts/build.sh windows dev

cp "dist/glow-collector-dev$N"     /tmp/glow-collector-linux-x64
cp "dist/glow-collector-dev$N.exe" /tmp/glow-collector-windows-x64.exe

# Point the `dev` tag at the current commit so the release's source zip matches,
# then swap the binaries and stamp the build number in the title.
git tag -f dev >/dev/null
git push -f origin dev >/dev/null
gh release upload dev \
  /tmp/glow-collector-linux-x64 /tmp/glow-collector-windows-x64.exe --clobber
gh release edit dev --title "glow L!VE — dev (build dev$N)" >/dev/null

echo "▸ dev release refreshed -> dev$N"
echo "  https://github.com/glow-moe/Glow-Live-Desktop/releases/tag/dev"
