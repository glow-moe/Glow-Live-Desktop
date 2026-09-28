package main

import (
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/gui"
	"github.com/glow-moe/glow-collector/internal/orchestrator"
)

func TestTrayLine(t *testing.T) {
	now := time.Now()
	ms := func(d time.Duration) int64 { return now.Add(-d).UnixMilli() }
	cases := []struct {
		name  string
		info  gui.TrayInfo
		want  string
		alert bool
	}{
		{"unlinked", gui.TrayInfo{}, "Not linked", false},
		{"outdated", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{Outdated: true}}, "Update required · open the app", true},
		{"update available", gui.TrayInfo{Linked: true, Running: true, UpdateVer: "v26.6"}, "Update available (v26.6) · open the app", false},
		{"stopped", gui.TrayInfo{Linked: true}, "Stopped", false},
		{"error", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{Err: "boom"}}, "Problem: boom", true},
		{"live fresh", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{InGame: true, LastPushAt: ms(2 * time.Second)}}, "Live · pushed just now", false},
		{"live stale", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{InGame: true, LastPushAt: ms(3 * time.Minute)}}, "Not reaching glow.moe (3m ago)", true},
		// Mirrored sources never push, so an old push clock is not an outage.
		{"splitcraft mirrored", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{Game: "splitcraft", InGame: true, LastPushAt: ms(3 * time.Minute)}}, "Live · on Discord", false},
		{"anime mirrored", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{Game: "anime", InGame: true}}, "Live · on Discord", false},
		{"idle after push", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{LastPushAt: ms(40 * time.Second)}}, "Watching · last push 40s ago", false},
		{"idle never", gui.TrayInfo{Linked: true, Running: true}, "Watching for a game", false},
		// Hidden on Discord from the tray: says so and how long, never an alert.
		{"hidden for an hour", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{InGame: true, DiscordHidden: true, DiscordHiddenUntil: now.Add(42 * time.Minute).UnixMilli()}}, "Hidden on Discord · 42m left", false},
		{"hidden for the game", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{InGame: true, DiscordHidden: true}}, "Hidden on Discord until this game ends", false},
		// A real problem still wins over the hide note.
		{"hidden with an error", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{Err: "boom", DiscordHidden: true}}, "Problem: boom", true},
	}
	for _, c := range cases {
		got, alert := trayLine(c.info, now)
		if got != c.want || alert != c.alert {
			t.Errorf("%s: got %q/%v, want %q/%v", c.name, got, alert, c.want, c.alert)
		}
	}
}
