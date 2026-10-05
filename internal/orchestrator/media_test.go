package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/roblox"
)

func TestProgressBar(t *testing.T) {
	at := time.UnixMilli(1_800_000_000_000)
	ts := progressBar(83, 213, false, at)
	if ts == nil {
		t.Fatal("expected a bar")
	}
	if ts.Start != at.Add(-83*time.Second).UnixMilli() || ts.End-ts.Start != 213_000 {
		t.Fatalf("bar = %+v", ts)
	}
	for _, c := range []struct {
		pos, length float64
		paused      bool
		at          time.Time
	}{
		{83, 213, true, at},           // paused: no bar
		{83, 0, false, at},            // unknown length
		{300, 213, false, at},         // past the end
		{83, 213, false, time.Time{}}, // no frame time
	} {
		if progressBar(c.pos, c.length, c.paused, c.at) != nil {
			t.Fatalf("expected no bar for %+v", c)
		}
	}
}

func TestMediaActivities(t *testing.T) {
	at := time.Now()
	a := animeActivityAt(animeSnap{Title: "Frieren", Episode: 12, CurrentTime: 600, Duration: 1440}, "sam", at)
	if a.Type != 3 || a.State != "Episode 12" || a.Timestamps == nil || a.Timestamps.End == 0 {
		t.Fatalf("anime = %+v", a)
	}
	p := animeActivityAt(animeSnap{Title: "Frieren", Episode: 12, CurrentTime: 600, Duration: 1440, Paused: true}, "sam", at)
	if p.Timestamps != nil || p.State != "Episode 12 · paused" {
		t.Fatalf("paused anime = %+v", p)
	}
	m := musicActivity(musicSnap{Title: "Idol", Artist: "YOASOBI", URL: "https://open.spotify.com/track/1", CurrentTime: 10, Duration: 200}, "sam", at)
	if m.Type != 2 || m.Details != "Idol" || m.State != "YOASOBI" || m.Timestamps == nil || len(m.Buttons) != 2 {
		t.Fatalf("music = %+v", m)
	}
	if bad := musicActivity(musicSnap{Title: "Idol", URL: "javascript:alert(1)"}, "", at); len(bad.Buttons) != 0 {
		t.Fatalf("unsafe button kept: %+v", bad.Buttons)
	}
	since := at.Add(-5 * time.Minute)
	r := readingActivity(mangaSnap{Title: "Frieren", Chapter: 128}, "sam", since)
	if r.Details != "Reading Frieren" || r.State != "Chapter 128" || r.Timestamps == nil || r.Timestamps.Start != since.UnixMilli() {
		t.Fatalf("reading = %+v", r)
	}
	if h := readingActivity(mangaSnap{Title: "Frieren", Chapter: 12.5}, "sam", since); h.State != "Chapter 12.5" {
		t.Fatalf("half chapter = %q", h.State)
	}
}

func TestRobloxActivity(t *testing.T) {
	at := time.Now().Add(-5 * time.Minute)
	g := roblox.Game{PlaceID: 920587237, Name: "Adopt Me!", Creator: "Uplift Games", Icon: "https://tr.rbxcdn.com/x.png", JoinedAt: at}
	a := robloxActivity(g, "melocet", true)
	if a.Details != "Adopt Me!" || a.State != "by Uplift Games" || a.Timestamps == nil || a.Timestamps.Start != at.UnixMilli() {
		t.Fatalf("named: %+v", a)
	}
	if a.Assets.LargeImage != g.Icon || len(a.Buttons) != 2 || a.Buttons[0].URL != "https://www.roblox.com/games/920587237" {
		t.Fatalf("assets/buttons: %+v %+v", a.Assets, a.Buttons)
	}
	if b := robloxActivity(roblox.Game{Name: "Adopt Me!"}, "", false); b.State != "on Roblox" || len(b.Buttons) != 0 || b.Timestamps != nil || b.Assets.LargeImage != glowIcon {
		t.Fatalf("glow app: %+v", b)
	}
}

func TestRobloxPayloadKeepsTheServerPrivate(t *testing.T) {
	at := time.Now().Add(-time.Minute)
	b, _ := json.Marshal(robloxPayload(roblox.Game{PlaceID: 13822889, Name: "Lumber Tycoon 2", Creator: "Defaultio", JoinedAt: at}))
	s := string(b)
	if !strings.Contains(s, `"game":"roblox"`) || !strings.Contains(s, `"placeId":13822889`) || !strings.Contains(s, fmt.Sprintf(`"startedAt":%d`, at.UnixMilli())) {
		t.Fatalf("payload %s", s)
	}
}

func TestMusicOnDiscordSkipsSpotifyAndYouTubeMusic(t *testing.T) {
	for svc, want := range map[string]bool{"spotify": false, "ytmusic": false, "youtube": true, "applemusic": true, "soundcloud": true, "": true} {
		if got := musicOnDiscord(svc); got != want {
			t.Errorf("%q: got %v, want %v", svc, got, want)
		}
	}
}
