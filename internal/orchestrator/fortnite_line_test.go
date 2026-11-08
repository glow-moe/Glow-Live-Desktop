package orchestrator

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/fortnite"
)

func TestFortniteThirdLine(t *testing.T) {
	lobby := fortnite.State{Last: &fortnite.Final{Left: 12}}
	if got := fortniteLine(lobby, "Hatsune Miku", "melocet"); got != "Last match: out with 12 left" {
		t.Fatalf("lobby after a match: %q", got)
	}
	if got := fortniteLine(fortnite.State{}, "Hatsune Miku", "melocet"); got != "Hatsune Miku" {
		t.Fatalf("lobby, no match yet: %q", got)
	}
	inMatch := fortnite.State{InMatch: true, Last: &fortnite.Final{Left: 12}}
	if got := fortniteLine(inMatch, "Hatsune Miku", "melocet"); got != "Hatsune Miku" {
		t.Fatalf("in a match: %q", got)
	}
	if got := fortniteLine(fortnite.State{Last: &fortnite.Final{}}, "", "melocet"); got != "glow.moe/melocet" {
		t.Fatalf("nothing known: %q", got)
	}
}
