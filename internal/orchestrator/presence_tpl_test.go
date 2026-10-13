package orchestrator

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/discord"
)

func TestPresenceTemplate(t *testing.T) {
	base := discord.Activity{
		Details:    "In a lobby",
		State:      "Custom",
		Assets:     &discord.Assets{LargeImage: "https://x/icon.png", LargeText: "me", SmallImage: glowIcon, SmallText: "glow.moe"},
		Timestamps: &discord.Timestamps{Start: 1},
		Buttons:    []discord.Button{{Label: "old", URL: "https://glow.moe/"}},
	}
	tpl := presenceTpl{
		State:     []string{"Locked in {champ}", "{queue} · {mode}", "{queue}"},
		LargeText: []string{"{summoner} (Lv {level})"},
		Buttons: []tplButton{
			{Label: "Watch {user}", URL: "https://glow.moe/{user}/live"},
			{Label: "Bad", URL: "javascript:alert(1)"},
			{Label: "Missing {nope}", URL: "https://glow.moe/"},
		},
		NoTime: true,
	}
	got := applyTpl(base, tpl, map[string]string{"queue": "ARAM: Mayhem", "summoner": "Hidden", "level": "484", "user": "melocet"})
	if got.Details != "In a lobby" {
		t.Errorf("details kept: %q", got.Details)
	}
	if got.State != "ARAM: Mayhem" {
		t.Errorf("state: %q", got.State)
	}
	if got.Assets.LargeText != "Hidden (Lv 484)" || got.Assets.LargeImage != "https://x/icon.png" {
		t.Errorf("assets: %+v", got.Assets)
	}
	if len(got.Buttons) != 1 || got.Buttons[0].URL != "https://glow.moe/melocet/live" {
		t.Errorf("buttons: %+v", got.Buttons)
	}
	if got.Timestamps != nil {
		t.Error("NoTime should drop the timer")
	}
	// No alternative fits: the built-in value stays.
	got = applyTpl(base, presenceTpl{State: []string{"{champ}"}}, nil)
	if got.State != "Custom" {
		t.Errorf("fallback state: %q", got.State)
	}
	// "" clears on purpose.
	got = applyTpl(base, presenceTpl{State: []string{""}}, nil)
	if got.State != "" {
		t.Errorf("cleared state: %q", got.State)
	}
	// One-character results are skipped (Discord drops the activity).
	got = applyTpl(base, presenceTpl{State: []string{"{x}", "{queue}"}}, map[string]string{"x": "a", "queue": "Flex"})
	if got.State != "Flex" {
		t.Errorf("short skip: %q", got.State)
	}
}

func TestInPhase(t *testing.T) {
	if !inPhase(nil, "Lobby") || !inPhase([]string{"None"}, "") || inPhase([]string{"ChampSelect"}, "Lobby") {
		t.Fatal("inPhase")
	}
}
