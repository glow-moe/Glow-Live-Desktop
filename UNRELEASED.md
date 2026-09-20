# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.

- Discord: a session on the SplitCraft Minecraft server now shows as "Playing SplitCraft" Rich Presence: your own Minecraft face as the picture (served by splitcraft.net, Bedrock players included), the world and rank on one line (AFK when you step away), what you play on and how many are online on the next ("on Java", "on PlayStation", "12 online"), the session timer, and "View my Glow profile" / "Visit SplitCraft" buttons. The server plugin reports it to glow.moe; the app reads it back the same way it does anime. Every server rank reads as its badge (VIP, VIP+, MVP, MVP+, INSANE, HARDCORE, Glow+). New "Show SplitCraft on Discord" toggle in Settings, on by default.
- The row of game chips under the status line is gone; the status line already says what is running.
- While SplitCraft or anime is mirrored the tray reads "Live on Discord" instead of warning that nothing reaches glow.moe (those sources are never pushed from the app), and a closed Discord is mentioned in the status line rather than shown as a problem.
- Far fewer requests to glow.moe: the anime and SplitCraft mirrors are read together once every 10 seconds instead of on every 1.5 second poll tick (about one request per second per app before).
