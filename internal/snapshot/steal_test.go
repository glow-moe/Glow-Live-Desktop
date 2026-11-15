package snapshot

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/live"
)

func TestFeedStealAndAce(t *testing.T) {
	teams := map[string]string{"Melocet": "ORDER"}
	e, _ := feedLine(live.Event{EventName: "BaronKill", KillerName: "Melocet", Stolen: "True", EventTime: 1500}, "Melocet", "ORDER", teams, nil)
	if e.Text != "stole Baron Nashor" {
		t.Fatalf("baron = %q", e.Text)
	}
	e, _ = feedLine(live.Event{EventName: "DragonKill", KillerName: "Melocet", DragonType: "Elder", Stolen: "False"}, "Melocet", "ORDER", teams, nil)
	if e.Text != "slayed the Elder Dragon" {
		t.Fatalf("dragon = %q", e.Text)
	}
	e, _ = feedLine(live.Event{EventName: "Ace", Acer: "Melocet", AcingTeam: "ORDER"}, "Melocet", "ORDER", teams, nil)
	if e.Who != "Melocet" || e.Text != "scored an Ace" {
		t.Fatalf("ace = %+v", e)
	}
}
