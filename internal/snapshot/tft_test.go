package snapshot

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/tft"
)

func TestFromTFTInGame(t *testing.T) {
	start := time.Date(2026, 11, 8, 12, 8, 37, 0, time.UTC)
	s := tft.State{Name: "Melisa", Tag: "TR1", Phase: tft.PhaseInGame, QueueID: 1100, Set: 18, StartedAt: start, Companion: "StyleTwoAhri_Base_Classic_Tier1",
		Units: []tft.Unit{{Champ: "Xayah", Stars: 1}, {Champ: "Varus", Stars: 2}}}
	snap := FromTFT(s, start.Add(12*time.Minute+34*time.Second))
	if snap.Game != "tft" || !snap.Live || snap.Phase != "inGame" || snap.Clock != "12:34" || snap.Me.RiotName != "Melisa" {
		t.Fatalf("got %+v", snap)
	}
	if snap.Tft.Queue != "Ranked" || snap.Tft.Set != 18 || snap.Tft.Units[0].ChampKey != "Varus" || snap.Tft.Units[0].Stars != 2 {
		t.Fatalf("got %+v", snap.Tft)
	}
	if snap.Tft.CompanionIcon != "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/loadouts/companions/tooltip_styletwoahri_base_classic_tier1.png" {
		t.Fatalf("icon %s", snap.Tft.CompanionIcon)
	}
	b, _ := json.Marshal(snap)
	if !strings.Contains(string(b), `"tft":{"state":"inGame","queue":"Ranked","queueId":1100,"set":18,"units":[{"champKey":"Varus","stars":2}`) {
		t.Fatalf("json %s", b)
	}
}

func TestFromTFTMenusIsNotTimePlayed(t *testing.T) {
	snap := FromTFT(tft.State{Phase: tft.PhaseMenus, QueueID: 4242}, time.Now())
	if snap.Live || snap.Phase != "lobby" || snap.Clock != "" || snap.Tft.State != "menus" || snap.Tft.Queue != "" || snap.Tft.Units != nil {
		t.Fatalf("got %+v %+v", snap, snap.Tft)
	}
}
