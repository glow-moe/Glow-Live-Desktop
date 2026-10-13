package dota

import (
	"strings"
	"testing"
)

const frame = `{
 "provider": {"name": "Dota 2", "appid": 570},
 "map": {"matchid": "7842119045", "game_state": "DOTA_GAMERULES_STATE_GAME_IN_PROGRESS", "clock_time": 1884, "daytime": false},
 "player": {"steamid": "76561190000000000", "accountid": "123", "activity": "playing", "kills": 9, "deaths": 2, "assists": 7},
 "hero": {"name": "npc_dota_hero_juggernaut", "level": 18},
 "auth": {"token": "0123456789abcdef0123456789abcdef"},
 "previously": {"hero": {"level": 17}},
 "wearables": {"wearable0": 1}
}`

func TestAccept(t *testing.T) {
	r := &Receiver{token: "0123456789abcdef0123456789abcdef"}
	out, err := r.accept([]byte(frame))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, gone := range []string{"auth", "0123456789abcdef", "previously", "wearables"} {
		if strings.Contains(s, gone) {
			t.Errorf("%q should not be forwarded: %s", gone, s)
		}
	}
	if !strings.Contains(s, "npc_dota_hero_juggernaut") {
		t.Errorf("hero missing: %s", s)
	}
	if _, err := (&Receiver{token: "ffffffffffffffffffffffffffffffff"}).accept([]byte(frame)); err == nil {
		t.Error("a wrong token must be refused")
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize([]byte(frame))
	if !s.Playing || s.Draft || s.Hero != "juggernaut" || s.Kills != 9 || s.Level != 18 || s.Clock != 1884 {
		t.Fatalf("%+v", s)
	}
	spec := strings.Replace(frame, `"activity": "playing"`, `"activity": "menu"`, 1)
	if Summarize([]byte(spec)).Playing {
		t.Error("menus / spectating are not playing")
	}
}

func TestCfg(t *testing.T) {
	c := cfgText("0123456789abcdef0123456789abcdef")
	if !strings.Contains(c, `"uri"		"http://127.0.0.1:47101/"`) || tokenRE.FindStringSubmatch(c)[1] != "0123456789abcdef0123456789abcdef" {
		t.Fatal(c)
	}
}
