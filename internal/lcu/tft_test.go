package lcu

import "testing"

func TestCompanionIconURL(t *testing.T) {
	got := companionIconURL("/lol-game-data/assets/ASSETS/Loadouts/Companions/Tooltip_AkaliDragon_Ossia_Tier1.png")
	want := "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/loadouts/companions/tooltip_akalidragon_ossia_tier1.png"
	if got != want {
		t.Fatalf("got %s", got)
	}
	if companionIconURL("no-art.png") != "" {
		t.Fatal("a path outside the assets folder has no art")
	}
}

func TestTFTRankKey(t *testing.T) {
	for queue, want := range map[int]string{1090: "RANKED_TFT", 1100: "RANKED_TFT", 1160: "RANKED_TFT_DOUBLE_UP", 1130: ""} {
		if got := tftRankKey(queue); got != want {
			t.Errorf("queue %d: got %q, want %q", queue, got, want)
		}
	}
}
