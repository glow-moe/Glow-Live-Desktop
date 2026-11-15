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

func TestCS2MapArt(t *testing.T) {
	if got := cs2MapArt("de_dust2"); got != "https://glow.moe/cs2/maps/de_dust2.jpg" {
		t.Fatalf("dust2 = %q", got)
	}
	if got := cs2MapArt("workshop/123/aim_map"); got != "" {
		t.Fatalf("workshop = %q", got)
	}
	a := cs2Activity(cs2.Summary{Map: "de_mirage", Phase: "live"}, "")
	if a.Assets == nil || a.Assets.LargeImage != "https://glow.moe/cs2/maps/de_mirage.jpg" {
		t.Fatalf("activity image = %+v", a.Assets)
	}
}

func TestCS2DeathmatchHasNoTeamScore(t *testing.T) {
	s := cs2.Summary{Map: "de_mirage", Mode: "deathmatch", Phase: "live", Kills: 14, Deaths: 6, Assists: 2, Mine: 0, Theirs: 0}
	a := cs2Activity(s, "")
	if a.Details != "Mirage · Deathmatch" || a.State != "14 / 6 / 2" {
		t.Fatalf("deathmatch = %q / %q", a.Details, a.State)
	}
	s.Phase = "gameover"
	if a := cs2Activity(s, ""); a.Details != "Mirage · Deathmatch" {
		t.Fatalf("deathmatch end = %q", a.Details)
	}
	if d := cs2Detail(s); d != "CS2 · Mirage · Deathmatch · 14/6/2" {
		t.Fatalf("detail = %q", d)
	}
}
