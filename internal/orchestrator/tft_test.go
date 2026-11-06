package orchestrator

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/snapshot"
)

func TestTFTActivity(t *testing.T) {
	s := snapshot.Snapshot{Me: snapshot.Me{Level: 7}, Tft: &snapshot.TFT{Queue: "Ranked TFT", Companion: "Ossia", CompanionIcon: "https://x/o.png", Tier: "GOLD", Division: "II", LP: 45}}
	a := tftActivity(s, 60, "melocet")
	if a.Details != "Ranked TFT" || a.State != "Level 7  |  Gold II 45 LP" || a.Assets.LargeImage != "https://x/o.png" || a.Assets.LargeText != "Ossia" {
		t.Fatalf("got %+v %+v", a, a.Assets)
	}
	bare := tftActivity(snapshot.Snapshot{Me: snapshot.Me{Level: 3}, Tft: &snapshot.TFT{}}, 0, "")
	if bare.Details != "Teamfight Tactics" || bare.State != "Level 3" || bare.Assets.LargeImage != glowIcon {
		t.Fatalf("got %+v %+v", bare, bare.Assets)
	}
}
