package orchestrator

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/discord"
)

func TestTagGlowButtons(t *testing.T) {
	orig := []discord.Button{
		{Label: "Live game", URL: "https://glow.moe/sam/league"},
		{Label: "Listen along", URL: "https://open.spotify.com/track/x"},
	}
	a := tagGlowButtons(discord.Activity{Buttons: orig})
	if a.Buttons[0].URL != "https://glow.moe/sam/league?s=rpc" {
		t.Errorf("glow.moe button: %q", a.Buttons[0].URL)
	}
	if a.Buttons[1].URL != "https://open.spotify.com/track/x" {
		t.Errorf("other sites stay as they are: %q", a.Buttons[1].URL)
	}
	if orig[0].URL != "https://glow.moe/sam/league" {
		t.Error("the original buttons were changed")
	}
	again := tagGlowButtons(discord.Activity{Buttons: []discord.Button{{Label: "x", URL: "https://glow.moe/sam?tab=a"}}})
	if again.Buttons[0].URL != "https://glow.moe/sam?tab=a&s=rpc" {
		t.Errorf("with a query: %q", again.Buttons[0].URL)
	}
	if twice := tagGlowButtons(again); twice.Buttons[0].URL != again.Buttons[0].URL {
		t.Errorf("tagged twice: %q", twice.Buttons[0].URL)
	}
}
