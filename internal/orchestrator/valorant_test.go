package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/valorant"
)

const (
	jettID = "add6443a-41bd-e414-f6ad-e58d267f4e95"
	cardID = "9fb348bc-41a0-91ad-8a3e-818035c4e561"
)

func valNames() *valorantNames {
	n := &valorantNames{Agents: map[string]string{jettID: "Jett"}, Modes: map[string]string{"competitive": "Competitive", "deathmatch": "Deathmatch"}}
	n.Maps = map[string]struct{ Name, Splash string }{"Ascent": {"Ascent", "https://x/ascent.png"}}
	return n
}

func TestValorantStage(t *testing.T) {
	cases := []struct {
		v    valorant.State
		want string
	}{
		{valorant.State{Phase: "MENUS", PartySize: 1}, valMenus},
		{valorant.State{Phase: "MENUS", PartySize: 2}, valLobby},
		{valorant.State{Phase: "MENUS", PartyState: "MATCHMAKING", PartySize: 1}, valQueue},
		{valorant.State{Phase: "MENUS", PartyState: "CUSTOM_GAME_SETUP"}, valCustom},
		{valorant.State{Phase: "PREGAME", PartyState: "MATCHMAKING"}, valSelect},
		{valorant.State{Phase: "INGAME", PartyState: "MATCHMAKING"}, valMatch},
	}
	for _, c := range cases {
		if got := valorantStage(&c.v); got != c.want {
			t.Errorf("%+v: got %s, want %s", c.v, got, c.want)
		}
	}
}

func TestValorantActivityInAMatch(t *testing.T) {
	v := &valorant.State{Phase: "INGAME", Queue: "competitive", MapCode: "Ascent", AllyScore: 7, FoeScore: 5, Level: 40}
	since := time.Now().Add(-time.Minute)
	a := valorantActivity(v, valNames(), valMatch, jettID, since, "")
	if a.Details != "Competitive · Ascent" || a.State != "7 - 5" {
		t.Fatalf("lines: %q %q", a.Details, a.State)
	}
	if !strings.HasSuffix(a.Assets.LargeImage, "/agents/"+jettID+"/displayicon.png") || a.Assets.LargeText != "Jett" {
		t.Fatalf("art: %+v", a.Assets)
	}
	if a.Timestamps == nil || a.Timestamps.Start != since.UnixMilli() {
		t.Fatalf("clock: %+v", a.Timestamps)
	}
	if bare := valorantActivity(v, valNames(), valMatch, "", since, ""); bare.Assets.LargeImage != "https://x/ascent.png" || bare.Assets.LargeText != "Ascent" {
		t.Fatalf("no agent should keep the map: %+v", bare.Assets)
	}
}

func TestValorantDeathmatchHasNoScore(t *testing.T) {
	v := &valorant.State{Phase: "INGAME", Queue: "deathmatch", MapCode: "Ascent", AllyScore: 12, FoeScore: 15, Level: 40}
	a := valorantActivity(v, valNames(), valMatch, "", time.Time{}, "")
	if strings.Contains(a.State, "12") {
		t.Fatalf("state %q", a.State)
	}
	if vars := valorantVars(v, valNames(), valMatch, ""); vars["ally"] != "" {
		t.Fatalf("vars %+v", vars)
	}
}

func TestValorantMenus(t *testing.T) {
	v := &valorant.State{Phase: "MENUS", Queue: "competitive", PartySize: 2, PartyMax: 5, Card: cardID, MapCode: "Ascent"}
	a := valorantActivity(v, valNames(), valLobby, "", time.Time{}, "")
	if a.Details != "Competitive" || a.State != "In lobby · (2/5)" || a.Timestamps != nil {
		t.Fatalf("lobby: %+v", a)
	}
	if !strings.HasSuffix(a.Assets.LargeImage, "/playercards/"+cardID+"/smallart.png") {
		t.Fatalf("art: %+v", a.Assets)
	}
	q := valorantActivity(v, valNames(), valQueue, "", time.Now(), "")
	if q.State != "In queue · (2/5)" || q.Timestamps == nil {
		t.Fatalf("queue: %+v", q)
	}
	c := valorantActivity(&valorant.State{Phase: "MENUS", PartySize: 3, PartyMax: 10}, valNames(), valCustom, "", time.Time{}, "")
	if c.Details != "Custom game" || c.State != "In lobby · (3/10)" {
		t.Fatalf("custom: %+v", c)
	}
}

func TestValorantMediaURLRefusesAnythingButAnID(t *testing.T) {
	for _, id := range []string{"", "x", "../../evil", "00000000-0000-0000-0000-000000000000"} {
		if got := valorantMediaURL("agents", id, "displayicon.png"); got != "" {
			t.Errorf("%q gave %s", id, got)
		}
	}
}

func TestValorantClockCountsFromTheStage(t *testing.T) {
	o := &Orchestrator{}
	first := o.valorantSince(valMatch)
	if first.IsZero() || !o.valorantSince(valMatch).Equal(first) {
		t.Fatal("the clock moved inside one stage")
	}
	if !o.valorantSince(valLobby).IsZero() {
		t.Fatal("a lobby has no clock")
	}
}
