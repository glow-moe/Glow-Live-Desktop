package cs2

import (
	"encoding/json"
	"strings"
	"testing"
)

const tok = "0123456789abcdef0123456789abcdef"

func frame(playerID string) []byte {
	return []byte(`{
	"provider": {"name": "Counter-Strike: Global Offensive", "appid": 730, "steamid": "76561198000000001"},
	"map": {"mode": "competitive", "name": "de_mirage", "phase": "live", "round": 12, "team_ct": {"score": 5}, "team_t": {"score": 6}},
	"round": {"phase": "live"},
	"player": {"steamid": "` + playerID + `", "name": "melo", "clan": "GLOW", "team": "T", "activity": "playing",
		"state": {"health": 100}, "match_stats": {"kills": 9, "deaths": 4, "assists": 2}},
	"previously": {"map": {"round": 11}},
	"auth": {"token": "` + tok + `"}
}`)
}

func TestAcceptDropsIDsAndMarksSelf(t *testing.T) {
	r := &Receiver{token: tok}
	out, err := r.accept(frame("76561198000000001"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, gone := range []string{"7656119", "melo", "GLOW", "previously", "provider", tok} {
		if strings.Contains(s, gone) {
			t.Errorf("%q left in %s", gone, s)
		}
	}
	var got struct {
		Self bool `json:"self"`
	}
	_ = json.Unmarshal(out, &got)
	if !got.Self {
		t.Error("own player block not marked self")
	}
	sum := Summarize(out)
	if !sum.Playing || sum.Side != "T" || sum.Mine != 6 || sum.Theirs != 5 || sum.Kills != 9 {
		t.Errorf("summary %+v", sum)
	}
}

func TestSpectatedPlayerIsNotSelf(t *testing.T) {
	r := &Receiver{token: tok}
	out, _ := r.accept(frame("76561198000000002"))
	sum := Summarize(out)
	if sum.Self || sum.Side != "" || sum.Kills != 0 {
		t.Errorf("teammate taken for self: %+v", sum)
	}
}

func TestBadToken(t *testing.T) {
	r := &Receiver{token: "ffffffffffffffffffffffffffffffff"}
	if _, err := r.accept(frame("1")); err == nil {
		t.Error("accepted a frame with the wrong token")
	}
}

func TestMenusAreNotPlaying(t *testing.T) {
	if Summarize(json.RawMessage(`{"player":{"activity":"menu"},"self":true}`)).Playing {
		t.Error("menu read as a match")
	}
}
