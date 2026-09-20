# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.

- Discord: a session on the SplitCraft Minecraft server now shows as Rich Presence (world and rank, "Join SplitCraft" button). The server plugin reports it to glow.moe; the app reads it back the same way it does anime. New "Show SplitCraft on Discord" toggle in Settings, on by default.
- Far fewer requests to glow.moe: the anime and SplitCraft mirrors are read together once every 10 seconds instead of on every 1.5 second poll tick (about one request per second per app before).
