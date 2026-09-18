# Unreleased (dev)

Changes riding the rolling dev build, waiting for the next numbered release. Add
a line here as you build; at release time this list becomes the release notes
and gets cleared.
- L!VE settings: the app now sends its build version when it reads your settings from glow.moe, the same way it does when it pushes a match. Without it the server treated 26.6 and 26.6.1 as outdated and the settings never arrived, so name hiding on the OBS overlay, the stream delay and the RP buttons set on the site did not apply. A failed read now retries every 15 seconds instead of on every tick.
