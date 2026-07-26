# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; run `bash scripts/release-dev.sh` and it becomes the
dev release notes. At release time this whole list becomes the release notes and
gets cleared.

- Steam Rich Presence: what you're playing on Steam shows on your profile and Discord, switching the instant you change games, carrying the game's own status line (map/mode).
- Steam game switches are instant, read from the machine itself rather than waiting on Steam's 2-minute cache.
- Discord shows a crisp square game icon (from SteamGridDB) instead of a cropped cover, with the glow badge in the corner.
- Games that publish their own Discord rich presence (Zenless Zone Zero; Contagion on Windows) are left to do it themselves, per platform, so nothing collides.
- Settings live in one panel behind a gear button: Discord, App, Games, and site links.
- Per-game toggle: turn any Steam game off to keep it off your profile and Discord entirely. Stays on your PC only.
- Start with the computer, open in the tray, and tuck into the tray when a game starts.
- Single instance: launching the app again brings the running copy back instead of opening a second one.
- Update nudge: release builds check GitHub for a newer version and show a download link when behind.
- Linux: system tray with Open / Quit, close-to-tray, and the glow icon on the window and taskbar.
