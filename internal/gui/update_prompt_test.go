package gui

import (
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/orchestrator"
)

func TestUpdatePromptOncePerRelease(t *testing.T) {
	cfg := config.Default()
	cfg.AutoUpdate = false // the pop-up is for people who update by hand
	s := &Server{cfg: cfg, orch: orchestrator.New(cfg)}
	peeks, busy := 0, false
	s.SetPeekWindow(func() bool {
		if busy {
			return false
		}
		peeks++
		return true
	})

	s.maybePromptUpdate() // no update out
	if peeks != 0 {
		t.Fatal("prompted without an update")
	}
	s.updateVer = "v26.8"
	busy = true
	s.maybePromptUpdate() // full screen: try again later
	busy = false
	s.maybePromptUpdate()
	s.maybePromptUpdate()
	if peeks != 1 || !s.peeked {
		t.Fatalf("peeks = %d, want 1", peeks)
	}

	// "Later" quiets this release for a day, even in a new session.
	s.cfg.UpdateLaterVer, s.cfg.UpdateLaterUntil = "v26.8", time.Now().Add(time.Hour).UnixMilli()
	s.prompted = ""
	s.maybePromptUpdate()
	if peeks != 1 {
		t.Fatal("prompted during Later")
	}
	// A newer release asks again.
	s.updateVer = "v26.9"
	s.maybePromptUpdate()
	if peeks != 2 {
		t.Fatalf("peeks = %d, want 2", peeks)
	}
}

// With automatic updates on the app installs the release itself, so it doesn't
// pop the window for it.
func TestUpdatePromptQuietWithAutoUpdate(t *testing.T) {
	s := &Server{cfg: config.Default(), orch: orchestrator.New(config.Default())}
	peeks := 0
	s.SetPeekWindow(func() bool { peeks++; return true })
	s.updateVer = "v26.10"
	s.maybePromptUpdate()
	if peeks != 0 {
		t.Fatal("prompted although automatic updates are on")
	}
}
