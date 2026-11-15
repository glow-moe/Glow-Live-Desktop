package live

import "testing"

func TestParseSkipsPracticeToolDummy(t *testing.T) {
	b := []byte(`{"allPlayers":[
		{"championName":"Akali","items":[{"itemID":3020,"slot":0}]},
		{"championName":"","rawChampionName":"","skinName":"Target Dummy","items":{"error":"Unable to find player"}}
	]}`)
	d, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.AllPlayers) != 1 || d.AllPlayers[0].ChampionName != "Akali" || len(d.AllPlayers[0].Items) != 1 {
		t.Fatalf("players = %+v", d.AllPlayers)
	}
}
