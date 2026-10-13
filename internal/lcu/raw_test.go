package lcu

import "testing"

// The site's list can only reach read-only client areas.
func TestRawAllowed(t *testing.T) {
	ok := []string{
		"/lol-challenges/v1/summary-player-data/local-player",
		"/lol-lobby/v2/lobby",
		"/lol-champ-select/v1/session",
		"/lol-ranked/v1/current-ranked-stats",
	}
	for _, p := range ok {
		if !RawAllowed(p) {
			t.Errorf("%s should be allowed", p)
		}
	}
	bad := []string{
		"/lol-chat/v1/me",
		"/lol-login/v1/session",
		"/lol-store/v1/catalog",
		"/riotclient/kill-and-restart-ux",
		"/lol-lobby/../lol-chat/v1/me",
		"/lol-lobby/v2/lobby?x=1",
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
