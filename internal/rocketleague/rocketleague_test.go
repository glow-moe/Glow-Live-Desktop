package rocketleague

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWithStatsAPI(t *testing.T) {
	got := withStatsAPI("")
	if !strings.Contains(got, "[TAGame.MatchStatsExporter_TA]\nPacketSendRate=10\nPort=49123") {
		t.Fatalf("empty file: %q", got)
	}
	in := "[Other]\r\nA=1\r\n[TAGame.MatchStatsExporter_TA]\r\nPacketSendRate=0\r\n[Next]\r\nB=2"
	got = withStatsAPI(in)
	sec := got[strings.Index(got, "[TAGame"):strings.Index(got, "[Next]")]
	if !strings.Contains(sec, "PacketSendRate=10\r\n") || !strings.Contains(sec, "Port=49123\r\n") || strings.Contains(got, "PacketSendRate=0") || !strings.Contains(got, "[Other]\r\nA=1") {
		t.Fatalf("existing section: %q", got)
	}
	if withStatsAPI(got) != got {
		t.Fatal("not stable: a second run changed the file")
	}
}

func TestHandleUpdateState(t *testing.T) {
	r := &Reader{}
	frame := `{"MatchGuid":"g1","Players":[
		{"Name":"Melocet","TeamNum":1,"Goals":3,"Shots":5,"Saves":2,"Assists":1,"Demos":1,"Score":620},
		{"Name":"Other","TeamNum":0,"Goals":1}],
		"Game":{"Teams":[{"TeamNum":0,"Score":1},{"TeamNum":1,"Score":4}],"TimeSeconds":74,"Arena":"Stadium_P",
		"bHasTarget":true,"Target":{"Name":"Melocet","TeamNum":1}}}`
	r.handle("UpdateState", json.RawMessage(frame))
	m, ok := r.Latest(10e9)
	if !ok || m.Mine != 4 || m.Theirs != 1 || m.Goals != 3 || m.Saves != 2 || m.Clock != 74 || m.Arena != "Stadium_P" {
		t.Fatalf("match = %+v", m)
	}
	// A replay following someone else doesn't change who the member is.
	replay := strings.Replace(strings.Replace(frame, `"Target":{"Name":"Melocet"`, `"Target":{"Name":"Other"`, 1), `"bHasTarget":true`, `"bHasTarget":true,"bReplay":true`, 1)
	r.handle("UpdateState", json.RawMessage(replay))
	if m, _ := r.Latest(10e9); m.Goals != 3 {
		t.Fatalf("after replay = %+v", m)
	}
	r.handle("MatchDestroyed", nil)
	if _, ok := r.Latest(10e9); ok {
		t.Fatal("still a match after MatchDestroyed")
	}
}
