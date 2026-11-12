package snapshot

import (
	"encoding/json"
	"testing"

	"github.com/glow-moe/glow-collector/internal/live"
)

// Riot's new Classic Rift (2026-11-12): bots are "Jade_<Champ> bot" in the
// list, a nickname in events, and turrets kill under their object names.
const jadeGame = `{
 "activePlayer": {"riotId": "Melocet#SETT", "riotIdGameName": "Melocet", "summonerName": "Melocet#SETT"},
 "allPlayers": [
  {"championName": "Ahri", "rawChampionName": "game_character_displayname_Ahri", "riotId": "Melocet#SETT", "riotIdGameName": "Melocet", "summonerName": "Melocet#SETT", "team": "ORDER"},
  {"championName": "Master Yi", "isBot": true, "rawChampionName": "game_character_displayname_Jade_MasterYi", "riotId": "Jade_MasterYi bot#BOT", "riotIdGameName": "Jade_MasterYi bot", "summonerName": "Pro as Heck Bot", "team": "CHAOS"}
 ],
 "events": {"Events": [
  {"EventName": "ChampionKill", "EventTime": 287.9, "KillerName": "Turret_TChaos_L2_P3_2521511112_0", "VictimName": "Melocet"},
  {"EventName": "ChampionKill", "EventTime": 294.0, "KillerName": "Pro as Heck Bot", "VictimName": "Melocet"},
  {"EventName": "ChampionKill", "EventTime": 300.0, "KillerName": "Melocet", "VictimName": "Pro as Heck Bot"}
 ]},
 "gameData": {"gameMode": "JADE", "gameTime": 300, "mapName": "Map453", "mapNumber": 453}
}`

func TestNewClassicRiftNames(t *testing.T) {
	var d live.AllGameData
	if err := json.Unmarshal([]byte(jadeGame), &d); err != nil {
		t.Fatal(err)
	}
	s := Build(&d, "16.22.1", 0)
	if s.MapName != "Summoner's Rift" {
		t.Fatalf("map %q", s.MapName)
	}
	if len(s.Red) != 1 || s.Red[0].Summoner != "Master Yi bot" {
		t.Fatalf("red %+v", s.Red)
	}
	want := [][2]string{{"Melocet", "slew Master Yi bot"}, {"Master Yi bot", "slew Melocet"}, {"A turret", "slew Melocet"}}
	if len(s.Feed) != len(want) {
		t.Fatalf("feed %+v", s.Feed)
	}
	for i, w := range want {
		if s.Feed[i].Who != w[0] || s.Feed[i].Text != w[1] {
			t.Fatalf("feed %d = %q %q, want %q", i, s.Feed[i].Who, s.Feed[i].Text, w)
		}
	}
}
