#!/usr/bin/env bash
# Builds the macOS app on a Mac (GitHub Actions' macOS runner or a real one):
# one universal binary (Apple Silicon + Intel), the glow L!VE.app around it,
# an ad-hoc signature, and the .dmg people download. Not notarized: there is
# no Apple Developer account, so the first launch needs "Open Anyway".
#
#   packaging/macos/build.sh <version> [outdir]
#
# The Discord application ids come from the same APP_* environment variables
# the Linux and Windows builds use (empty = that game skips Rich Presence).
#
# Out: <outdir>/glow-live-macos            the bare binary (the in-app updater swaps this)
#      <outdir>/glow-live-macos.dmg        the .app on a disk image (first install)
#      <outdir>/glow-live-macos.zip        the .app zipped (same, for people who prefer it)
set -euo pipefail

VERSION="${1:?usage: packaging/macos/build.sh <version> [outdir]}"
OUT="${2:-dist}"
P=github.com/glow-moe/glow-collector/internal/orchestrator
APPNAME="glow L!VE"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
mkdir -p "$OUT"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# macOS 12 is the floor: CI builds with Go 1.25, the last Go whose runtime runs
# there (Go 1.27 needs 13). Keep the C side (webview) at the same floor.
export MACOSX_DEPLOYMENT_TARGET=12.0
LDFLAGS="-s -w -X main.version=$VERSION \
  -X $P.appGlow=${APP_GLOW:-} -X $P.appLoL=${APP_LOL:-} \
  -X $P.appForzaH6=${APP_FH6:-} -X $P.appForzaH5=${APP_FH5:-} \
  -X $P.appSplitcraft=${APP_SPLITCRAFT:-}"

for arch in arm64 amd64; do
  clangarch=$arch; [ "$arch" = amd64 ] && clangarch=x86_64
  echo "> go build darwin/$arch"
  CGO_ENABLED=1 GOOS=darwin GOARCH=$arch CC="clang -arch $clangarch" CXX="clang++ -arch $clangarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$WORK/glow-live-$arch" ./cmd/gui
done
lipo -create -output "$WORK/glow-live" "$WORK/glow-live-arm64" "$WORK/glow-live-amd64"
lipo -info "$WORK/glow-live"

echo "> $APPNAME.app"
APP="$WORK/$APPNAME.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$WORK/glow-live" "$APP/Contents/MacOS/glow-live"
sed "s/__VERSION__/${VERSION#v}/g" packaging/macos/Info.plist > "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist"

# Icon: the same "G" as the Windows exe (packaging/macos/icon-256.png, taken
# from cmd/gui/icon.ico), scaled into every size an .icns carries.
ICONSET="$WORK/AppIcon.iconset"
mkdir -p "$ICONSET"
for s in 16 32 128 256 512; do
  sips -z $s $s packaging/macos/icon-256.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  d=$((s * 2))
  sips -z $d $d packaging/macos/icon-256.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"

# Ad-hoc signature: Apple Silicon refuses to run unsigned code at all, and a
# sealed bundle is what "Open Anyway" approves once.
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict "$APP"

echo "> artifacts"
cp "$APP/Contents/MacOS/glow-live" "$OUT/glow-live-macos"
(cd "$WORK" && ditto -c -k --keepParent "$APPNAME.app" "glow-live-macos.zip")
cp "$WORK/glow-live-macos.zip" "$OUT/glow-live-macos.zip"
# The disk image: the app next to an Applications shortcut, to drag across.
STAGE="$WORK/dmg"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "$APPNAME" -srcfolder "$STAGE" -ov -format UDZO "$OUT/glow-live-macos.dmg"
ls -la "$OUT"
