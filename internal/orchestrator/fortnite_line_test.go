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

func TestFortniteSaveTheWorldCard(t *testing.T) {
	mission := fortnite.State{InMatch: true, Status: "Save the World: Canny Valley (15)", Loadout: fortnite.Loadout{Character: "Character_CovertShadow"}}
	a := fortniteActivity(mission, "melocet")
	if a.Details != "Save the World · Canny Valley" || a.State != "Power level 15" {
		t.Fatalf("mission: %q / %q", a.Details, a.State)
	}
	if a.Assets.LargeImage != "https://glow.moe/api/live/rpc-art/stw?zone=canny&outfit=character_covertshadow" {
		t.Fatalf("mission art: %q", a.Assets.LargeImage)
	}
	if a.Assets.SmallImage != glowIcon || len(a.Buttons) != 2 {
		t.Fatalf("corner or buttons: %+v", a)
	}

	home := fortniteActivity(fortnite.State{InMatch: true, Status: "Save the World: Homebase (1 - Normal)"}, "melocet")
	if home.Details != "Save the World" || home.State != "At Homebase" || fortniteVars(fortnite.State{InMatch: true, Status: "Save the World: Homebase (1 - Normal)"})["stwLevel"] != "" || home.Assets.LargeImage != "https://glow.moe/api/live/rpc-art/stw?zone=homebase" || home.Assets.LargeText != "At Homebase" {
		t.Fatalf("homebase: %+v %+v", home, home.Assets)
	}

	party := fortnite.State{InMatch: true, ID: "a", Status: "Save the World: Plankerton (19)", Party: []fortnite.Member{{Name: "me", ID: "a"}, {Name: "you", ID: "b"}}}
	if p := fortniteActivity(party, ""); p.State != "Power level 19 · party of 2" {
		t.Fatalf("party: %q", p.State)
	}

	shield := party
	shield.Stw = fortnite.StwRun{Mission: "Storm Shield Defense", Wave: 3}
	if p := fortniteActivity(shield, ""); p.State != "Storm Shield · Wave 3 · party of 2" || fortniteVars(shield)["stwWave"] != "3" {
		t.Fatalf("storm shield: %q", p.State)
	}

	odd := fortniteActivity(fortnite.State{InMatch: true, Status: "Save the World: Ventures"}, "")
	if odd.Details != "Save the World · Ventures" || odd.State != "On a mission" || odd.Assets.LargeText != "On a mission" || odd.Assets.LargeImage != "https://glow.moe/api/live/rpc-art/stw?zone=other" || odd.Buttons != nil {
		t.Fatalf("unknown place: %+v", odd)
	}

	// Battle Royale and the lobby keep their card.
	if br := fortniteActivity(fortnite.State{InMatch: true, Status: "Solo - 73 Left", Phase: "SafeZones"}, ""); br.Details != "Solo" {
		t.Fatalf("battle royale: %q", br.Details)
	}
	if lobby := fortniteActivity(fortnite.State{Status: "Save the World: Stonewood (15)"}, ""); lobby.State != "In the lobby" {
		t.Fatalf("lobby: %q", lobby.State)
	}

	v := fortniteVars(mission)
	if v["stwPlace"] != "Canny Valley" || v["stwZone"] != "canny" || v["stwHome"] != "" || v["stwLevel"] != "15" {
		t.Fatalf("vars: %v", v)
	}
	if v := fortniteVars(fortnite.State{InMatch: true, Status: "Solo - 73 Left"}); v["stwZone"] != "" {
		t.Fatalf("br vars: %v", v)
	}
}
