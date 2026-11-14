package orchestrator

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/poster"
)

// The settings read must carry the build version like the push does: without
// it the server's release gate treats the app as a pre-26.5 build and answers
// 426 forever (26.6 and 26.6.1 shipped that way).
func TestFetchSettingsSendsVersionHeader(t *testing.T) {
	poster.SetVersion("v26.6.2")
	var gotVersion, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Glow-Live-Version")
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/live/settings" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"hideMyName": true, "delaySec": 7})
	}))
	defer srv.Close()

	s, _, err := fetchSettings(srv.URL+"/api/live/ingest", "tok", "")
	if err != nil {
		t.Fatalf("fetchSettings failed against a 200 server: %v", err)
	}
	if gotVersion != "v26.6.2" {
		t.Fatalf("X-Glow-Live-Version = %q, want v26.6.2", gotVersion)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !s.HideMyName || s.DelaySec != 7 {
		t.Fatalf("settings not decoded: %+v", s)
	}
}

// A key glow.moe no longer knows (the PC was removed under Linked PCs while the
// app was off) has to come back as unlinked from the settings read too: with no
// game open nothing is pushed, and the app sat on "@you" instead of the link
// screen (2026-11-02).
func TestFetchSettingsUnlinkedOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	if _, _, err := fetchSettings(srv.URL+"/api/live/ingest", "gone", ""); !errors.Is(err, poster.ErrUnlinked) {
		t.Fatalf("err = %v, want poster.ErrUnlinked", err)
	}
}

func TestFetchSettingsServerErrorIsNotUnlinked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, _, err := fetchSettings(srv.URL+"/api/live/ingest", "tok", "")
	if err == nil || errors.Is(err, poster.ErrUnlinked) {
		t.Fatalf("err = %v, want a plain error", err)
	}
}

// An unchanged read is a 304 to the ETag the app sent back: no body, and the
// app keeps the settings it has.
func TestFetchSettingsSendsItsTagAndKeepsOnNotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_ = json.NewEncoder(w).Encode(map[string]any{"delaySec": 7, "sv": "abc", "refreshSec": 300, "quietSec": 20})
	}))
	defer srv.Close()

	s, tag, err := fetchSettings(srv.URL+"/api/live/ingest", "tok", "")
	if err != nil || tag != `"v1"` || s.Sv != "abc" || s.RefreshSec != 300 || s.QuietSec != 20 {
		t.Fatalf("first read: %+v %q %v", s, tag, err)
	}
	if _, tag, err = fetchSettings(srv.URL+"/api/live/ingest", "tok", tag); !errors.Is(err, errSettingsSame) || tag != `"v1"` {
		t.Fatalf("second read: %q %v", tag, err)
	}
}

func TestSettingsDue(t *testing.T) {
	now := time.Now()
	s := liveSettings{Sv: "a", RefreshSec: 300}
	if settingsDue(s, now.Add(-2*time.Minute), now.Add(-2*time.Minute), "a", now) {
		t.Fatal("read again before the site's refresh time")
	}
	if !settingsDue(s, now.Add(-2*time.Minute), now.Add(-2*time.Minute), "b", now) {
		t.Fatal("a push named newer settings: read them")
	}
	if settingsDue(s, now.Add(-2*time.Minute), now.Add(-5*time.Second), "b", now) {
		t.Fatal("never within 15s of the last try")
	}
	if !settingsDue(liveSettings{}, now.Add(-61*time.Second), now.Add(-61*time.Second), "", now) {
		t.Fatal("an older site (no refreshSec) keeps the minute")
	}
	if !settingsDue(liveSettings{RefreshSec: 99999}, now.Add(-61*time.Second), now.Add(-61*time.Second), "", now) {
		t.Fatal("an out-of-range refresh falls back to the minute")
	}
}

func TestQuietEvery(t *testing.T) {
	if quietEvery(liveSettings{QuietSec: 20}) != 20*time.Second || quietEvery(liveSettings{}) != heartbeat || quietEvery(liveSettings{QuietSec: 300}) != heartbeat {
		t.Fatal("quietEvery bounds")
	}
}
