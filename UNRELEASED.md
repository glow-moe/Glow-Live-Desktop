# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.

- Dota 2: when Dota is open but sends no game state, the status line says to add `-gamestateintegration` to its Steam launch options (Dota only shares the live match with it).
- Dota 2: the app also passes on the cosmetics your hero has on, so glow.moe can show your skin. Dota picks this up the next time it starts.
- macOS (beta): a universal glow L!VE.app (Apple Silicon and Intel, macOS 12+) reading League, Dota 2, Steam games and Roblox, starting with the Mac and updating itself. Not notarized yet: the first launch needs Open Anyway in System Settings.
- The app tells glow.moe when it starts and quits (and how: with the PC, by hand, after an update, tray Quit, Windows shutting down), so a member whose app vanished can be told apart from one who closed it.
- Windows: updates work like Discord's now. Each release gets its own folder next to a small launcher, an update is a new folder (never a file written over the running one), and the launcher starts the newest. If a new build dies before it has run a minute (an antivirus removed it, a crash), the launcher goes back to the previous one and skips that release. Opening an old downloaded copy just starts the installed app instead of replacing it. The first start after this update moves the install over by itself.
- When glow.moe can't be reached at all, the status line says so in plain words (your internet, or your provider blocking it) instead of a raw network error, and the app tries a second glow.moe address that skips Cloudflare when glow.moe hands one out.
- glow.moe can put a bad release on hold: no app updates itself to it until it's taken off.
- A linked account with no username yet gets a Pick a username button instead of asking glow.moe every second.
- Settings > Help: Test connection (glow.moe, the backup address, this PC's link, Discord, updates, what's being read) and Export logs, which saves a log file to Downloads to send us. Nothing is sent on its own and the key is never in it.
- Pushes slow down to one every 10 seconds while nobody has your live view open on glow.moe (glow.moe says so in its answer), and go back to every tick the moment someone does. Discord and the OBS overlay don't change.
- Hide from my profile for 1 hour, in the tray menu and the window: the game stays off glow.moe while Discord keeps showing it.
- After an update, a What's new card shows that release's notes once.
- The window names the game you're in (VALORANT, Fortnite and Dota 2 showed as League).
- Windows: glow L!VE shows up in the Start menu (and its search). Made once; if you remove it, it stays removed.
- Windows: glow L!VE is listed in Settings > Apps > Installed apps, with its version, and Uninstall there removes it cleanly (the app, its autostart, Start menu entry, settings and log).
- Updates ask glow.moe first and come from there once glow.moe hosts the releases (GitHub until then), so where releases live can change without a new app. The download button opens glow.moe's L!VE page.
- If glow.moe unlinks this PC (removed under Linked PCs, or the password changed), the app goes straight back to its Link screen, with a line saying why.
- The app can show a pairing code for glow.moe's link page to ask for; glow.moe has it switched off for now.
- How long a new release waits before installing itself (it never installs mid-game) is set by glow.moe; 24 hours if the app can't ask.
