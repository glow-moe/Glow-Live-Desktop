package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/config"
)

// Status errors reach glow.moe once per message per hour; states that are not
// failures (not linked, outdated) never do.
func TestReportErrDedupesAndSkipsStates(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/live/report" || r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-Glow-Live-Version") == "" {
			t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		got = append(got, body)
		mu.Unlock()
	}))
	defer srv.Close()

	o := &Orchestrator{cfg: config.Config{Token: "tok", Endpoint: srv.URL + "/api/live/ingest"}}
	o.set(Status{Game: "league", InGame: true, Err: "lcu: connection refused"})
	o.set(Status{Game: "league", InGame: true, Err: "lcu: connection refused"}) // same message: skipped
	o.set(Status{Err: "Discord: pipe closed"})
	o.set(Status{Err: "not linked"})
	o.set(Status{Err: "Update required: this version is no longer supported."})
	o.set(Status{}) // no error: nothing to send

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // let any stray third report land
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("got %d reports, want 2: %+v", len(got), got)
	}
	kinds := map[string]map[string]string{}
	for _, g := range got {
		kinds[g["kind"]] = g
	}
	if l := kinds["league"]; l == nil || l["game"] != "lol" || l["message"] != "lcu: connection refused" {
		t.Errorf("league report = %+v", l)
	}
	if kinds["discord"] == nil {
		t.Errorf("discord report missing: %+v", got)
	}
}

func TestErrorKind(t *testing.T) {
	cases := map[[2]string]string{
		{"Discord not running", ""}:     "discord",
		{"read: timeout", "league"}:     "league",
		{"udp: bind failed", "forza"}:   "forza",
		{"steam: 503", "steam"}:         "steam",
		{"update check failed", ""}:     "update",
		{"something odd", "splitcraft"}: "other",
	}
	for in, want := range cases {
		if got := errorKind(in[0], in[1]); got != want {
			t.Errorf("errorKind(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
