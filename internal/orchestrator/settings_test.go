package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	s, ok := fetchSettings(srv.URL+"/api/live/ingest", "tok")
	if !ok {
		t.Fatal("fetchSettings failed against a 200 server")
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
