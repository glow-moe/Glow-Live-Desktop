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
  "splitcraft":{"live":true,"snapshot":{"game":"splitcraft","name":"Melocet","uuid":"cd0967da-b198-4c8b-bcf3-f73172c7bd78","group":"vipplus","world":"world_nether","platform":"java","sessionStartedAt":1758300000,"afk":true,"server":{"name":"SplitCraft","online":12}}}
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
	if !m.splitOK || m.split.World != "world_nether" || m.split.Group != "vipplus" || m.split.Server.Online != 12 || !m.split.AFK || m.split.UUID == "" {
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
	s.Name, s.World, s.Group, s.SessionStartedAt = "Melocet", "world_nether", "vipplus", 1758300000
	s.Server.Name, s.Server.Online = "SplitCraft", 12

	// Shared glow app: the first line has to name the server.
	appSplitcraft = ""
	a := splitcraftActivity(s, "melocet")
	if a.Details != "Playing on SplitCraft" || a.State != "Nether · VIP+" {
		t.Fatalf("details=%q state=%q", a.Details, a.State)
	}
	if a.Timestamps == nil || a.Timestamps.Start != 1758300000000 {
		t.Fatalf("seconds must become milliseconds: %+v", a.Timestamps)
	}
	if a.Assets == nil || a.Assets.LargeImage != "https://splitcraft.net/api/skin/Melocet.png?size=256" || a.Assets.LargeText != "Melocet" || a.Assets.SmallImage != splitcraftImage {
		t.Fatalf("assets = %+v", a.Assets)
	}
	if len(a.Buttons) != 2 || a.Buttons[0].Label != "View my Glow profile" || a.Buttons[0].URL != "https://glow.moe/melocet" || a.Buttons[1].Label != "Visit SplitCraft" {
		t.Fatalf("buttons = %+v", a.Buttons)
	}

	// Dedicated SplitCraft app: the headline is the app, the lines are place + crowd.
	appSplitcraft = "123"
	defer func() { appSplitcraft = "" }()
	b := splitcraftActivity(s, "")
	if b.Details != "Nether · VIP+" || b.State != "12 online" {
		t.Fatalf("dedicated app: details=%q state=%q", b.Details, b.State)
	}
	if len(b.Buttons) != 1 || b.Buttons[0].Label != "Visit SplitCraft" {
		t.Fatalf("buttons without a username = %+v", b.Buttons)
	}

	// The uuid wins over the name for the skin; Bedrock gets the server mark.
	s.UUID = "CD0967DA-B198-4C8B-BCF3-F73172C7BD78"
	if img := splitcraftSkin(s); img != "https://splitcraft.net/api/skin/cd0967dab1984c8bbcf3f73172c7bd78.png?size=256" {
		t.Fatalf("skin by uuid = %q", img)
	}
	s.Platform = "bedrock"
	if img := splitcraftSkin(s); img != splitcraftImage {
		t.Fatalf("bedrock skin = %q", img)
	}
	if img := splitcraftSkin(splitSnap{Name: "not a name!"}); img != splitcraftImage {
		t.Fatalf("odd name must not reach a URL: %q", img)
	}

	if d := splitcraftDetail(s); d != "SplitCraft · Nether · VIP+" {
		t.Fatalf("detail = %q", d)
	}
	s.AFK = true
	if st := splitcraftState(s); st != "AFK · VIP+" {
		t.Fatalf("afk state = %q", st)
	}
	s.AFK = false
	// No rank, no world, no server name: still a sane line.
	if d := splitcraftDetail(splitSnap{}); d != "SplitCraft" {
		t.Fatalf("empty detail = %q", d)
	}
}

// Every server rank reads as its badge, including ones added after this build.
func TestSplitcraftGroupLabels(t *testing.T) {
	cases := map[string]string{
		"": "", "vip": "VIP", "vipplus": "VIP+", "mvp": "MVP", "mvpplus": "MVP+", "MVP+": "MVP+",
		"insane": "INSANE", "hardcore": "HARDCORE", "glowplus": "Glow+", "legend_plus": "LEGEND+",
	}
	for in, want := range cases {
		if got := splitcraftGroup(in); got != want {
			t.Errorf("splitcraftGroup(%q) = %q, want %q", in, got, want)
		}
	}
}
