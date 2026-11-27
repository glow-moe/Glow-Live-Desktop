# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.

- Dota 2 keeps the same Discord status name in the menu and in a match.
- League matches send each player's Riot tag, so live pages can mark players who are on Glow.
- Teamfight Tactics works with its new standalone client: your Little Legend, queue, set, game clock and the champions you buy with their stars, on Discord and its own live page (glow.moe/<you>/tft). In the menus and queue too.
- VALORANT on Discord shows your agent in a match, your player card in the menus, lobby and queue with party size, and a match and queue clock. Deathmatch no longer shows a team score.
- Fortnite on Discord shows your outfit's name, and how your last match ended while you're in the lobby.
- League on Riot's new Classic Rift: the map reads Summoner's Rift, bots go by their champion ("Ryze bot"), and the match feed names turrets and inhibitors instead of their object ids.
- L!VE talks to glow.moe less: it reads your settings every few minutes (instantly when you change them during a game) and sends a frame every 20s when nobody is watching your match.
- Counter-Strike 2: your match on glow.moe/<you>/cs2 (map, score, health, money, K/D/A, your weapons) and on Discord, from CS2's own game state. Nothing to set up: the app adds its config file to CS2 by itself.
- Games outside Steam (Genshin Impact, Honkai: Star Rail, osu!, Overwatch, Minecraft for Windows and more) show on your Glow page while you play them. glow.moe keeps the list, so new ones come without an update.
- An ace in CS2 or a penta kill in League shows on your Discord status for 30 seconds.
- If CS2 was already open when L!VE set it up, L!VE tells you to restart CS2 once.
- League in Practice Tool keeps working after you spawn a target dummy (the match stopped showing before).
- League: stealing Baron or a Dragon, and finishing your team's Ace, show as big plays on your Glow page.
- Rocket League (Steam or Epic): your match (score, clock, goals, saves) shows on your Glow page and on Discord from the game's own Stats API. L!VE switches it on in the game's settings by itself; restart Rocket League once after.
- One odd value from a game no longer stops L!VE from reading the whole match (League, TFT, VALORANT, Dota, CS2).
- A stolen Baron or Dragon, and an Ace you finish, show on your Discord status for 30 seconds too.
- Fortnite Save the World on Discord: the zone and whether you're on a mission (with its power level) or at Homebase, with your outfit under that zone's sky as the picture.
- Fortnite Save the World on your Glow page: your three mission medals as they fill, the map, the wave in a Storm Shield, and after a mission everyone's score with the XP you earned and your commander level.
- glow.moe can pick a status card's verb (Playing, Competing, Watching, Listening) per game, starting with Fortnite.
- Fortnite Save the World on Discord: in a Storm Shield the card says which wave you're on.
- A test build of L!VE now updates to the real release of the same month (a 26.12 test build took itself for 26.12 and never updated).
- Fortnite on Discord no longer puts your glow.moe link on the card's third line before your outfit is known (the button already opens your page).
- Teamfight Tactics in the menus: your Little Legend from your last lobby (the home screen doesn't say which it is), a clock since you opened the game instead of 0:00, and no second glow badge.
