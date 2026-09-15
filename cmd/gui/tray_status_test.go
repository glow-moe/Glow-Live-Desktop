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
		{"idle after push", gui.TrayInfo{Linked: true, Running: true, Status: orchestrator.Status{LastPushAt: ms(40 * time.Second)}}, "Watching · last push 40s ago", false},
		{"idle never", gui.TrayInfo{Linked: true, Running: true}, "Watching for a game", false},
	}
	for _, c := range cases {
		got, alert := trayLine(c.info, now)
		if got != c.want || alert != c.alert {
			t.Errorf("%s: got %q/%v, want %q/%v", c.name, got, alert, c.want, c.alert)
		}
	}
}
