package snapshot

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/live"
)

func TestMapLabelNamesTheNewRift(t *testing.T) {
	if got := mapLabel(live.GameData{MapNumber: 453, GameMode: "JADE"}); got != "Summoner's Rift" {
		t.Fatalf("got %q", got)
	}
	if got := mapLabel(live.GameData{MapNumber: 12, GameMode: "ARAM"}); got != "Howling Abyss" {
		t.Fatalf("got %q", got)
	}
}
