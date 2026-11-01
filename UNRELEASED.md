# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.

- Dota 2: when Dota is open but sends no game state, the status line says to add `-gamestateintegration` to its Steam launch options (Dota only shares the live match with it).
- macOS (beta): a universal glow L!VE.app (Apple Silicon and Intel, macOS 12+) reading League, Dota 2, Steam games and Roblox, starting with the Mac and updating itself. Not notarized yet: the first launch needs Open Anyway in System Settings.
- The app tells glow.moe when it starts and quits (and how: with the PC, by hand, after an update, tray Quit, Windows shutting down), so a member whose app vanished can be told apart from one who closed it.
- Windows: updates work like Discord's now. Each release gets its own folder next to a small launcher, an update is a new folder (never a file written over the running one), and the launcher starts the newest. If a new build dies before it has run a minute (an antivirus removed it, a crash), the launcher goes back to the previous one and skips that release. Opening an old downloaded copy just starts the installed app instead of replacing it. The first start after this update moves the install over by itself.
