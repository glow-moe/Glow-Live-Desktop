package valorant

import "testing"

func TestRead(t *testing.T) {
	var s State
	s.read([]byte(`{"matchPresenceData":{"sessionLoopState":"INGAME","queueId":"competitive","matchMap":"/Game/Maps/Ascent/Ascent","provisioningFlow":"Matchmaking"},
		"partyPresenceData":{"partyOwnerMatchMap":"/Game/Maps/Duality/Duality","partyOwnerMatchScoreAllyTeam":7,"partyOwnerMatchScoreEnemyTeam":5},
		"playerPresenceData":{"accountLevel":212,"competitiveTier":13}}`))
	if s.Phase != "INGAME" || s.Queue != "competitive" || s.MapCode != "Duality" || s.AllyScore != 7 || s.FoeScore != 5 || s.Tier != 13 || s.Level != 212 {
		t.Fatalf("nested: %+v", s)
	}
	var f State
	f.read([]byte(`{"sessionLoopState":"MENUS","queueId":"unrated","accountLevel":40,"competitiveTier":0}`))
	if f.Phase != "MENUS" || f.Queue != "unrated" || f.Level != 40 || f.MapCode != "" {
		t.Fatalf("flat: %+v", f)
	}
}

func TestRankName(t *testing.T) {
	for n, want := range map[int]string{0: "", 3: "Iron 1", 13: "Gold 2", 24: "Immortal 1", 26: "Immortal 3", 27: "Radiant"} {
		if got := RankName(n); got != want {
			t.Errorf("%d: %q want %q", n, got, want)
		}
	}
}
