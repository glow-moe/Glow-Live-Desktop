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
