#!/usr/bin/env bash
#
# Build both platforms and refresh the single rolling `dev` pre-release IN PLACE,
# so the releases list never fills up with dev83, dev84, ... The internal build
# number (dev<N>) still increments so builds stay distinguishable in the app and
# the release title.
#
# The release notes are UNRELEASED.md, an accumulating changelog. Pass a line to
# append it first:
#
#   bash scripts/release-dev.sh                       # just rebuild + republish
#   bash scripts/release-dev.sh "Fixed the avatar"    # append, then rebuild
#
set -euo pipefail
cd "$(dirname "$0")/.."

# Optional changelog line -> append as a bullet before building.
if [ "${1:-}" != "" ]; then
  printf -- '- %s\n' "$1" >> UNRELEASED.md
  git add UNRELEASED.md && git commit -q -m "changelog: $1" || true
fi

N="$(cat DEV 2>/dev/null || echo 41)"

# Build both platforms at the SAME dev number (build.sh bumps DEV after each, so
# reset it before the second build).
echo "$N" > DEV; bash scripts/build.sh linux dev
echo "$N" > DEV; bash scripts/build.sh windows dev

cp "dist/glow-collector-dev$N"     /tmp/glow-collector-linux-x64
cp "dist/glow-collector-dev$N.exe" /tmp/glow-collector-windows-x64.exe

# Point the `dev` tag at the current commit so the source zip matches, then swap
# the binaries and set the notes from the accumulating changelog + a download
# table.
git tag -f dev >/dev/null
git push -f origin dev >/dev/null

NOTES="$(cat UNRELEASED.md)
| file | platform |
|---|---|
| glow-collector-linux-x64 | Linux x64 |
| glow-collector-windows-x64.exe | Windows x64 |"

gh release upload dev \
  /tmp/glow-collector-linux-x64 /tmp/glow-collector-windows-x64.exe --clobber
gh release edit dev --title "glow L!VE — dev (build dev$N)" --notes "$NOTES" >/dev/null

echo "▸ dev release refreshed -> dev$N"
echo "  https://github.com/glow-moe/Glow-Live-Desktop/releases/tag/dev"
