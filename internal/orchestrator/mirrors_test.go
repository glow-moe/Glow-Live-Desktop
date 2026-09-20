package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const mirrorsBody = `{"games":{
  "anime":{"live":true,"snapshot":{"title":"Frieren","episode":7,"poster":"https://x/p.jpg"}},
  "splitcraft":{"live":true,"snapshot":{"game":"splitcraft","name":"Melocet","group":"vipplus","world":"world_nether","platform":"java","sessionStartedAt":1758300000,"server":{"name":"SplitCraft","online":12}}}
}}`

// Both mirrors ride one request, and each side parses into its own snapshot.
func TestFetchMirrorsParsesBothSources(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mirrorsBody))
	}))
	defer srv.Close()

	m, ok := fetchMirrors(srv.URL+"/api/live/ingest", "cuid123")
	if !ok {
		t.Fatal("fetchMirrors failed against a 200 server")
	}
	if path != "/api/live/read?u=cuid123&games=anime,splitcraft" {
		t.Fatalf("request path = %q", path)
	}
	if !m.animeOK || m.anime.Title != "Frieren" || m.anime.Episode != 7 {
		t.Fatalf("anime = %+v ok=%v", m.anime, m.animeOK)
	}
	if !m.splitOK || m.split.World != "world_nether" || m.split.Group != "vipplus" || m.split.Server.Online != 12 {
		t.Fatalf("splitcraft = %+v ok=%v", m.split, m.splitOK)
	}
}

// An idle source is just ok=false; a failed request is nothing live at all.
func TestFetchMirrorsIdleAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"games":{"anime":{"live":false,"snapshot":null},"splitcraft":{"live":false,"snapshot":null}}}`))
	}))
	defer srv.Close()
	m, ok := fetchMirrors(srv.URL+"/api/live/ingest", "u")
	if !ok || m.animeOK || m.splitOK {
		t.Fatalf("idle: ok=%v anime=%v split=%v", ok, m.animeOK, m.splitOK)
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	if _, ok := fetchMirrors(down.URL+"/api/live/ingest", "u"); ok {
		t.Fatal("a 503 must not count as a successful read")
	}
}

// The orchestrator reads the mirrors once per window, not once per tick: the
// 1.5s tick used to turn into a request per second on glow.moe.
func TestReadMirrorsIsThrottled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(mirrorsBody))
	}))
	defer srv.Close()

	o := &Orchestrator{}
	endpoint := srv.URL + "/api/live/ingest"
	for i := 0; i < 20; i++ {
		m := o.readMirrors(endpoint, "u")
		if !m.splitOK {
			t.Fatalf("tick %d lost the cached SplitCraft session", i)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("20 ticks inside one window made %d requests, want 1", n)
	}

	// Age the cache past the window: exactly one more read.
	o.mu.Lock()
	o.mirrorsAt = time.Now().Add(-mirrorEvery - time.Second)
	o.mu.Unlock()
	o.readMirrors(endpoint, "u")
	o.readMirrors(endpoint, "u")
	if n := hits.Load(); n != 2 {
		t.Fatalf("after the window expired: %d requests, want 2", n)
	}
}

func TestSplitcraftActivityText(t *testing.T) {
	var s splitSnap
	s.World, s.Group, s.SessionStartedAt = "world_nether", "vipplus", 1758300000
	s.Server.Name, s.Server.Online = "SplitCraft", 12
	a := splitcraftActivity(s, "melocet")
	if a.Details != "Playing on SplitCraft" || a.State != "Nether · VIP+" {
		t.Fatalf("details=%q state=%q", a.Details, a.State)
	}
	if a.Timestamps == nil || a.Timestamps.Start != 1758300000000 {
		t.Fatalf("seconds must become milliseconds: %+v", a.Timestamps)
	}
	if a.Assets == nil || a.Assets.LargeImage != splitcraftImage || a.Assets.LargeText != "SplitCraft · 12 online" {
		t.Fatalf("assets = %+v", a.Assets)
	}
	if len(a.Buttons) != 2 || a.Buttons[1].URL != "https://glow.moe/melocet" {
		t.Fatalf("buttons = %+v", a.Buttons)
	}
	if d := splitcraftDetail(s); d != "SplitCraft · Nether · VIP+" {
		t.Fatalf("detail = %q", d)
	}
	// No rank, no world, no server name: still a sane line.
	if d := splitcraftDetail(splitSnap{}); d != "SplitCraft" {
		t.Fatalf("empty detail = %q", d)
	}
}
