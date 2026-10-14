# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.
- League: ARAM: Mayhem, Classic Rift and other new queues show their real names instead of Custom; a queue the app doesn't know yet is named by the League client itself.
- League: your party lobby shows everyone's banner, level frame, title and challenge tokens, and their Riot IDs again (the client stopped putting names in the lobby).
- Lobby, champ select, the loading screen and your client profile are now read on glow.moe, so fixes and new League details arrive with a site update, no app update needed. Queue and mode names come from glow.moe too.
- Discord status wording can now be updated from glow.moe without a new app version.
- The OBS overlay page loads its look from glow.moe (it still reads your game from this PC, never from glow.moe), with the built-in one as a backup.
- Dota 2 (early): the app sets up Dota's Game State Integration and sends your live match to glow.moe/<you>/dota. Add -gamestateintegration to Dota 2's Steam launch options to turn it on.
- The app no longer writes League client dumps (lcu-*.json) next to itself; you can delete any old ones.
- The app's local page only answers its own window now, so other websites can't reach it.
- Much lighter on glow.moe: when nothing changed (sitting in a lobby, the client, a Steam game) the app stays quiet and checks in every 10 seconds; outside a match it sends at most every 3 seconds, and League client details only when they change. Matches still update live.
- VALORANT (early): menus, agent select and your match (map, mode, score, rank, level) on glow.moe/<you>/valorant and in Discord, read from the Riot Client on your PC only.
- VALORANT: Discord reads "Playing VALORANT" (its own Discord app), and glow.moe can pick which Discord app each status runs under.
- Windows: the app now lives in one place, %LOCALAPPDATA%\Programs\glow L!VE, and shows up as "glow L!VE.exe" in Startup apps instead of whatever name your browser gave the download. Running a downloaded copy moves it there on its own; you can delete the old one from Downloads.
- Steam: the built-in game table refreshed (19710 games) so new releases get their own "Playing <game>" headline offline too.
- Settings: a "Check for updates" button, and "Update on its own" (on by default) that installs a new version in the background a day after it comes out, never while you are in a game, and brings the app back in the tray.
