package orchestrator

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/cs2"
)

func TestCS2KeepsOwnNumbersWhileSpectating(t *testing.T) {
	o := &Orchestrator{}
	o.cs2Own(cs2.Summary{Playing: true, Map: "de_dust2", Self: true, Side: "T", Mine: 7, Theirs: 9, Kills: 12, Deaths: 8})
	// Dead, watching a teammate: CT 9, T 8 now, no side of its own.
	got := o.cs2Own(cs2.Summary{Playing: true, Map: "de_dust2", Mine: 9, Theirs: 8})
	if got.Side != "T" || got.Mine != 8 || got.Theirs != 9 || got.Kills != 12 {
		t.Errorf("got %+v", got)
	}
}

func TestCS2Names(t *testing.T) {
	if n := cs2MapName("de_dust2"); n != "Dust II" {
		t.Error(n)
	}
	if n := cs2MapName("workshop/1/de_cbble_2"); n != "Cbble 2" {
		t.Error(n)
	}
	a := cs2Activity(cs2.Summary{Map: "de_mirage", Mode: "competitive", Phase: "gameover", Mine: 13, Theirs: 9}, "melocet")
	if a.Details != "Mirage · Won 13 – 9" || len(a.Buttons) != 2 {
		t.Errorf("%+v", a)
	}
}
