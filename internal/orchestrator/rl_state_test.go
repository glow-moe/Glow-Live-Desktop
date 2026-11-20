package orchestrator

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/rocketleague"
)

func TestRlState(t *testing.T) {
	m := &rocketleague.Match{Arena: "DFH Stadium", Mine: 3, Theirs: 2, Clock: 74}
	if got := rlState(m); got != "DFH Stadium · 3–2 · 1:14" {
		t.Fatalf("live: %q", got)
	}
	m.Ended, m.Won = true, true
	if got := rlState(m); got != "DFH Stadium · won 3–2" {
		t.Fatalf("ended: %q", got)
	}
	if rlState(nil) != "" {
		t.Fatal("no match should be empty")
	}
	if !rlListed(map[string]string{"rocketleague.exe": "Rocket League"}) || rlListed(map[string]string{"osu!.exe": "osu!"}) {
		t.Fatal("rlListed")
	}
}
