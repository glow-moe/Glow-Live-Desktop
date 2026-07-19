# glow L!VE desktop app

Native desktop app for glow.moe L!VE. While you play, it reads your local game
data and pushes a live snapshot to your glow.moe profile, and shows Discord Rich
Presence. Windows and Linux.

It reads:

- **League** from Riot's Live Client Data API (`127.0.0.1:2999`, up only while
  you're in a game)
- **Forza** from the game's Data Out UDP telemetry

Only your own live data is sent, only while the app is running. Pairing is done
in the browser (the app opens glow.moe and you approve the device), so there is
no key to copy.

## Layout

```
cmd/gui/               native window (WebView2 on Windows, WebKitGTK on Linux)
internal/gui/          local HTTP server + embedded UI
internal/orchestrator  ties League + Forza detection, push and Discord RPC together
internal/live          Riot Live Client Data API reader
internal/forza         Forza Data Out UDP listener
internal/snapshot      Riot data -> the glow.moe page schema
internal/ddragon       patch version, champion + spell icons
internal/poster        authenticated push to the ingest endpoint
internal/discord       Discord Rich Presence over the local IPC pipe
```

## Build

Go 1.22+. Discord app ids are kept out of git: copy `.appids.example` to
`.appids` and fill in your ids. The build injects them with `-ldflags`, so they
never land in source.

### Linux

Needs `libwebkit2gtk-4.1-dev`. `.pkgconfig-shim` maps `webkit2gtk-4.0` to `4.1`.

```sh
source ./.appids
P=github.com/glow-moe/glow-collector/internal/orchestrator
PKG_CONFIG_PATH="$PWD/.pkgconfig-shim:$PKG_CONFIG_PATH" CGO_ENABLED=1 \
  go build -ldflags "-X main.version=v$(cat VERSION) \
    -X $P.appGlow=$APP_GLOW -X $P.appLoL=$APP_LOL \
    -X $P.appForzaH6=$APP_FH6 -X $P.appForzaH5=$APP_FH5" \
  -o glow-collector ./cmd/gui
```

### Windows (cross-build)

Needs `mingw-w64`. WebView2 ships with Windows 10/11, so users install nothing.
`.winshim` provides `EventToken.h`.

```sh
source ./.appids
P=github.com/glow-moe/glow-collector/internal/orchestrator
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
  CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
  CGO_CXXFLAGS="-I$PWD/.winshim" CGO_CPPFLAGS="-I$PWD/.winshim" \
  go build -ldflags "-H windowsgui -X main.version=v$(cat VERSION) \
    -X $P.appGlow=$APP_GLOW -X $P.appLoL=$APP_LOL \
    -X $P.appForzaH6=$APP_FH6 -X $P.appForzaH5=$APP_FH5" \
  -o glow-collector.exe ./cmd/gui
```

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE).
