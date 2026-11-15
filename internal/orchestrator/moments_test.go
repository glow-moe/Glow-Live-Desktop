package orchestrator

import (
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/cs2"
	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/snapshot"
)

func TestMomentFlash(t *testing.T) {
	o := &Orchestrator{}
	now := time.Unix(1000, 0)
	base := discord.Activity{Details: "Mirage · 9 – 7", State: "18 / 9 / 3"}

	ace := cs2.Summary{Self: true, Mode: "competitive", Map: "de_mirage", Kills: 18, RoundKills: 5}
	o.cs2Ace(ace, now)
	if got := o.withFlash(base, now.Add(10*time.Second)).State; got != "ACE on Mirage" {
		t.Fatalf("flash = %q", got)
	}
	if got := o.withFlash(base, now.Add(31*time.Second)).State; got != "18 / 9 / 3" {
		t.Fatalf("after 30s = %q", got)
	}
	// The round-end frames are the same ace: no second flash.
	o.cs2Ace(ace, now.Add(40*time.Second))
	if got := o.withFlash(base, now.Add(45*time.Second)).State; got != "18 / 9 / 3" {
		t.Fatalf("repeat = %q", got)
	}
	// Casual has 10 a side: five kills there is no ace.
	o2 := &Orchestrator{}
	o2.cs2Ace(cs2.Summary{Self: true, Mode: "casual", Map: "de_mirage", Kills: 5, RoundKills: 5}, now)
	if o2.flash.text != "" {
		t.Fatal("casual flashed")
	}
}

func TestLeaguePentaFlash(t *testing.T) {
	o := &Orchestrator{}
	now := time.Unix(1000, 0)
	var s snapshot.Snapshot
	s.Me.RiotName, s.Me.ChampName, s.Me.ChampKey = "Melocet", "Ahri", "Ahri"
	s.Feed = []snapshot.FeedEvent{{Who: "Someone", Text: "Penta Kill", Time: "20:00"}}
	o.leaguePenta(s, now)
	if o.flash.text != "" {
		t.Fatal("someone else's penta flashed")
	}
	s.Feed = append(s.Feed, snapshot.FeedEvent{Who: "melocet", Text: "Penta Kill", Time: "24:10"})
	o.leaguePenta(s, now)
	if got := o.withFlash(discord.Activity{State: "x"}, now).State; got != "PENTA KILL as Ahri" {
		t.Fatalf("penta = %q", got)
	}
}
