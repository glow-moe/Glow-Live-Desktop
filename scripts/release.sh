#!/usr/bin/env bash
#
# Cut a numbered release (v1.3, v1.4, ...) from the accumulated dev changelog.
# The UNRELEASED.md list becomes the release notes and is then cleared, so the
# next dev cycle starts fresh.
#
#   bash scripts/release.sh 1.3
#
set -euo pipefail
cd "$(dirname "$0")/.."

VER="${1:?usage: release.sh <version, e.g. 1.3>}"
echo "$VER" > VERSION

bash scripts/build.sh linux release
bash scripts/build.sh windows release
cp "dist/glow-live-v$VER-linux-x64"     /tmp/glow-live-v$VER-linux-x64
cp "dist/glow-live-v$VER-windows-x64.exe" /tmp/glow-live-v$VER-windows-x64.exe

git tag "v$VER"
git push origin "v$VER"

gh release create "v$VER" \
  "dist/glow-live-v$VER-linux-x64" "dist/glow-live-v$VER-windows-x64.exe" \
  --title "glow L!VE v$VER" --notes "$(cat UNRELEASED.md)"

# Start the next dev cycle from a clean slate.
cat > UNRELEASED.md <<'MD'
# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; run `bash scripts/release-dev.sh` and it becomes the
dev release notes. At release time this whole list becomes the release notes and
gets cleared.

MD
git add UNRELEASED.md VERSION && git commit -q -m "release v$VER; reset dev changelog"
git push origin main

echo "▸ released v$VER; UNRELEASED.md reset for the next cycle"
