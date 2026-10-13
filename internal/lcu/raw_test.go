package lcu

import "testing"

// The site's list can only reach read-only client areas.
func TestRawAllowed(t *testing.T) {
	ok := []string{
		"/lol-challenges/v1/summary-player-data/local-player",
		"/lol-lobby/v2/lobby",
		"/lol-champ-select/v1/session",
		"/lol-ranked/v1/current-ranked-stats",
		"/lol-chat/v1/me",
		"/lol-match-history/v1/products/lol/current-summoner/matches?begIndex=0&endIndex=5",
		"/lol-summoner/v1/summoner-profile?puuid=0a1b2c3d-0000-1111-2222-333344445555",
	}
	for _, p := range ok {
		if !RawAllowed(p) {
			t.Errorf("%s should be allowed", p)
		}
	}
	bad := []string{
		"/lol-chat/v1/friends",
		"/lol-chat/v1/conversations",
		"/lol-chat/v1/me/x",
		"/lol-login/v1/session",
		"/lol-lobby/v2/lobby?x=1;y",
		"/lol-lobby/v2/lobby?x=a b",
		"/lol-store/v1/catalog",
		"/riotclient/kill-and-restart-ux",
		"/lol-lobby/../lol-chat/v1/me",
		"/lol-lobby//v2",
		"lol-lobby/v2/lobby",
		"/LOL-LOBBY/v2/lobby",
		"",
	}
	for _, p := range bad {
		if RawAllowed(p) {
			t.Errorf("%s should be refused", p)
		}
	}
}

// Pick keeps only the asked-for fields, so account ids don't ride along.
func TestPickFields(t *testing.T) {
	got := string(PickFields([]byte(`{"puuid":"x","bannerId":"9","title":{"name":"Jungle Diff"}}`), []string{"bannerId", "title"}))
	if got != `{"bannerId":"9","title":{"name":"Jungle Diff"}}` {
		t.Fatalf("got %s", got)
	}
	if string(PickFields([]byte(`[1]`), []string{"a"})) != `[1]` {
		t.Fatal("non-object should pass through")
	}
}

func TestPickPaths(t *testing.T) {
	body := []byte(`{"puuid":"x","games":{"games":[{"queueId":420,"gameId":1,"participants":[{"championId":7,"stats":{"win":true,"kills":3,"gold":9}}]}]},"timer":{"phase":"BAN_PICK","x":1}}`)
	got := string(PickFields(body, []string{"games.games.queueId", "games.games.participants.championId", "games.games.participants.stats.win", "timer"}))
	want := `{"games":{"games":[{"participants":[{"championId":7,"stats":{"win":true}}],"queueId":420}]},"timer":{"phase":"BAN_PICK","x":1}}`
	if got != want {
		t.Fatalf("got  %s, want %s", got, want)
	}
	if string(PickFields([]byte(`[{"a":1,"b":2}]`), []string{"a"})) != `[{"a":1}]` {
		t.Fatal("top-level arrays are walked too")
	}
}
