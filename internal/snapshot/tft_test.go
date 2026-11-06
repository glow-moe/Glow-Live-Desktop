package snapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/glow-moe/glow-collector/internal/lcu"
	"github.com/glow-moe/glow-collector/internal/live"
)

func tftGame() *live.AllGameData {
	d := &live.AllGameData{}
	d.GameData.GameMode = "TFT"
	d.GameData.MapNumber = 22
	d.GameData.GameTime = 754
	d.ActivePlayer.RiotID = "Melisa#TR1"
	d.ActivePlayer.Level = 7
	return d
}

func TestIsTFT(t *testing.T) {
	if !IsTFT(tftGame()) {
		t.Fatal("TFT not recognized")
	}
	if IsTFT(&live.AllGameData{GameData: live.GameData{GameMode: "CLASSIC"}}) {
		t.Fatal("a Rift game read as TFT")
	}
}

func TestBuildTFT(t *testing.T) {
	s := BuildTFT(tftGame(), &lcu.TFTInfo{Queue: "Ranked TFT", Companion: "Ossia", Tier: "GOLD", Division: "II", LP: 45}, "26.22.1", 1)
	if s.Clock != "12:34" || s.Me.Level != 7 || s.Me.RiotName != "Melisa" || s.Me.RiotTag != "TR1" {
		t.Fatalf("unexpected me: %+v clock %q", s.Me, s.Clock)
	}
	if s.MapName != "Convergence" {
		t.Fatalf("map %q", s.MapName)
	}
	if s.Tft == nil || s.Tft.Tier != "GOLD" || s.Tft.Companion != "Ossia" {
		t.Fatalf("tft %+v", s.Tft)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"blue":[]`) || !strings.Contains(string(b), `"tft":{`) {
		t.Fatalf("json %s", b)
	}
}

func TestBuildTFTWithoutTheClient(t *testing.T) {
	if s := BuildTFT(tftGame(), nil, "", 0); s.Tft == nil {
		t.Fatal("a TFT game must still read as TFT")
	}
}
