# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; run `bash scripts/release-dev.sh` and it becomes the
dev release notes. At release time this whole list becomes the release notes and
gets cleared.

- OBS overlay: the card now sits at the bottom-right of the browser source (where the minimap is), stays on screen between polls instead of blinking off, and rides out short client hiccups.
- League: chroma skins now resolve to their parent skin's art, so Discord, the overlay and your live page show the right splash instead of a blank.
- Tray: a status line (last push, live, problem, update required) in the menu and tooltip, an "Open my profile" item, and a red-dot icon when glow.moe is not receiving anything.
- Steam: the session timer restarts when you relaunch a game instead of counting from the first launch of the day.
- Steam: the built-in game table refreshed (19,342 games) so new releases get their own "Playing …" headline offline too.
- Linux: League of Legends is no longer looked for (it does not work under Wine); the chip and the mention are gone from the Linux build. Windows is unchanged.
- Stream-snipe delay now actually works: the app holds each snapshot for the delay you set in the dashboard and sends it that many seconds later, so your public live page runs behind your stream. The OBS overlay stays real-time (it is part of your stream). Before this the setting was read but nothing delayed.
