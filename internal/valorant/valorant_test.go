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

func TestReadParty(t *testing.T) {
	var s State
	s.read([]byte(`{"partyPresenceData":{"partyState":"MATCHMAKING","partySize":2,"maxPartySize":5},"playerPresenceData":{"playerCardId":"abc"}}`))
	if s.PartyState != "MATCHMAKING" || s.PartySize != 2 || s.PartyMax != 5 || s.Card != "abc" {
		t.Fatalf("nested: %+v", s)
	}
	var f State
	f.read([]byte(`{"partyState":"DEFAULT","partySize":1,"maxPartySize":5,"playerCardId":"xyz"}`))
	if f.PartyState != "DEFAULT" || f.PartySize != 1 || f.PartyMax != 5 || f.Card != "xyz" {
		t.Fatalf("flat: %+v", f)
	}
}

func TestSelfAgent(t *testing.T) {
	var tr Tracker
	if tr.SelfAgent() != "" {
		t.Fatal("no match, no agent")
	}
	tr.match = &Match{Players: []Player{{Agent: "other", Locked: true}, {Agent: "hover", Self: true}}}
	if tr.SelfAgent() != "" {
		t.Fatal("an agent you only hover isn't yours yet")
	}
	tr.match.Players[1].Locked = true
	if tr.SelfAgent() != "hover" {
		t.Fatal("locked agent not read")
	}
}

func TestRankName(t *testing.T) {
	for n, want := range map[int]string{0: "", 3: "Iron 1", 13: "Gold 2", 24: "Immortal 1", 26: "Immortal 3", 27: "Radiant"} {
		if got := RankName(n); got != want {
			t.Errorf("%d: %q want %q", n, got, want)
		}
	}
}
