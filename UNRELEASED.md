# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.

- Dota 2: when Dota is open but sends no game state, the status line says to add `-gamestateintegration` to its Steam launch options (Dota only shares the live match with it).
- macOS (beta): a universal glow L!VE.app (Apple Silicon and Intel, macOS 12+) reading League, Dota 2, Steam games and Roblox, starting with the Mac and updating itself. Not notarized yet: the first launch needs Open Anyway in System Settings.
- The app tells glow.moe when it starts and quits (and how: with the PC, by hand, after an update, tray Quit, Windows shutting down), so a member whose app vanished can be told apart from one who closed it.
- Opening an old downloaded copy no longer overwrites a newer installed app: it just starts the installed one.
