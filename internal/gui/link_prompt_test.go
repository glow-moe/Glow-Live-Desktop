package gui

import (
	"os"
	"testing"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/orchestrator"
)

// A key glow.moe turned down brings the link screen up on its own, once,
// after any full screen app; a read that left before linking again can't
// drop the new key.
func TestDropLinkBringsUpTheLinkScreen(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir()) // config.Save stays out of the real one
	if dir, err := os.UserConfigDir(); err != nil || dir == "" {
		t.Skip("no config dir")
	}
	cfg := config.Default()
	cfg.Token = "new"
	s := &Server{cfg: cfg, orch: orchestrator.New(cfg)}
	peeks, busy := 0, true
	s.SetPeekWindow(func() bool {
		if busy {
			return false
		}
		peeks++
		return true
	})

	s.dropLink("old") // turned down before linking again
	if s.cfg.Token != "new" || s.wasUnlinked {
		t.Fatal("a stale key dropped the new link")
	}

	s.dropLink("new") // full screen: the link screen waits
	if s.cfg.Token != "" || !s.wasUnlinked || peeks != 0 || !s.linkPrompt {
		t.Fatalf("token %q, unlinked %v, peeks %d, pending %v", s.cfg.Token, s.wasUnlinked, peeks, s.linkPrompt)
	}
	busy = false
	s.maybePromptLink()
	s.maybePromptLink()
	if peeks != 1 || s.linkPrompt {
		t.Fatalf("peeks = %d, want 1", peeks)
	}
}
