# glow L!VE desktop app

Native desktop app for glow.moe L!VE. While you play, it reads your local game
data and pushes a live snapshot to your glow.moe profile, and shows Discord Rich
Presence. Windows and Linux.

It reads:

- **League** from Riot's Live Client Data API (`127.0.0.1:2999`, up only while
  you're in a game) and, outside a match, the League client's local API (lobby,
  champion select, rank). Windows only: Riot's client APIs don't run under Wine
- **Forza** from the game's Data Out UDP telemetry
- **Steam** from the client's own local record of the running game (registry on
  Windows, the launcher's process arguments on Linux), plus your public Steam
  profile card for the status line, avatar and level
- **Roblox** from the Roblox client's own log (which experience you joined; the
  website installer and the Microsoft Store build), named through Roblox's
  public web API. Windows only. The server you're on is never sent anywhere

When no game is running it mirrors to Discord what glow.moe already knows about
you: the anime, manga or song the browser extension reports, and your SplitCraft
session.

Only your own live data is sent, only while the app is running. Pairing is done
in the browser (the app opens glow.moe and you approve the device), so there is
no key to copy. Updates come from this repo's releases: the app checks on start
and every few hours, and asks before it installs one.

Discord Rich Presence runs over Discord's local IPC pipe. The catalog that maps
Steam games to their Discord titles is embedded in the binary, so the app makes
no requests to Discord's servers.

## Layout

```
cmd/gui/               native window (WebView2 on Windows, WebKitGTK on Linux) + tray
internal/gui/          local HTTP server + embedded UI and OBS overlay
internal/orchestrator  ties the game readers, the push and Discord RPC together
internal/live          Riot Live Client Data API reader
internal/lcu           League client API (lobby, champion select, rank)
internal/forza         Forza Data Out UDP listener
internal/steam         running-game detection + public profile card + Discord catalog
internal/roblox        Roblox client log reader + public experience lookup
internal/snapshot      Riot data -> the glow.moe page schema
internal/ddragon       patch version, champion + spell icons
internal/poster        authenticated push to the ingest endpoint
internal/discord       Discord Rich Presence over the local IPC pipe
internal/pair          browser pairing with glow.moe
internal/update        release check + verified self-update
internal/config        local settings file
internal/autostart     start with the computer
internal/single        one running copy per user
```

## Build

Go 1.22+. Discord app ids are kept out of git: put them in a `.appids` file
(`APP_GLOW`, `APP_LOL`, `APP_FH6`, `APP_FH5`, `APP_SPLITCRAFT`, one `NAME=id` per
line). The build injects them with `-ldflags`, so they never land in source.
Roblox uses Roblox's own public Discord application, set in the code.

### Linux

Needs `libwebkit2gtk-4.1-dev`. `.pkgconfig-shim` maps `webkit2gtk-4.0` to `4.1`.

```sh
source ./.appids
P=github.com/glow-moe/glow-collector/internal/orchestrator
PKG_CONFIG_PATH="$PWD/.pkgconfig-shim:$PKG_CONFIG_PATH" CGO_ENABLED=1 \
  go build -ldflags "-X main.version=v$(cat VERSION) \
    -X $P.appGlow=$APP_GLOW -X $P.appLoL=$APP_LOL \
    -X $P.appForzaH6=$APP_FH6 -X $P.appForzaH5=$APP_FH5 \
    -X $P.appSplitcraft=$APP_SPLITCRAFT" \
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
    -X $P.appForzaH6=$APP_FH6 -X $P.appForzaH5=$APP_FH5 \
    -X $P.appSplitcraft=$APP_SPLITCRAFT" \
  -o glow-collector.exe ./cmd/gui
```

### Windows resources

`cmd/gui/icon.rc` holds the icon and the version resource (publisher, product,
version). Go links the compiled `rsrc_windows_amd64.syso` automatically. After
editing `icon.rc` or bumping `VERSION`, regenerate it with mingw-w64 windres:

```sh
x86_64-w64-mingw32-windres -O coff -o cmd/gui/rsrc_windows_amd64.syso cmd/gui/icon.rc
```

### Steam game table

`internal/steam/steam-apps.bin.gz` is the offline fallback for the Steam appid →
Discord application id table. glow.moe rebuilds the same table weekly from
Discord's detectable-games list and the app pulls it on start and once a week,
so the embedded copy only matters on a first launch without internet. Refresh it
with `curl -fsS https://glow.moe/api/steam-map -o internal/steam/steam-apps.bin.gz`
(the release script does this automatically).

## License

Mozilla Public License 2.0. See [LICENSE](LICENSE).
