package orchestrator

import (
	"strings"

	"github.com/glow-moe/glow-collector/internal/discord"
)

// tagGlowButtons marks every glow.moe button on the Discord status with
// ?s=rpc, so glow.moe can tell a visit (and a signup) that came from someone's
// status apart from any other Discord link. Other sites' buttons (Listen
// along, Roblox, SplitCraft) are left as they are. The buttons are copied, so
// a template's own slice is never changed.
func tagGlowButtons(a discord.Activity) discord.Activity {
	if len(a.Buttons) == 0 {
		return a
	}
	btns := make([]discord.Button, len(a.Buttons))
	for i, b := range a.Buttons {
		if strings.HasPrefix(b.URL, "https://glow.moe/") && !strings.Contains(b.URL, "s=rpc") {
			if strings.Contains(b.URL, "?") {
				b.URL += "&s=rpc"
			} else {
				b.URL += "?s=rpc"
			}
		}
		btns[i] = b
	}
	a.Buttons = btns
	return a
}
