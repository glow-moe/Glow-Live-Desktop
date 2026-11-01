package poster

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDoTriesTheFallbackWhenGlowCantBeReached(t *testing.T) {
	var got []string
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, r.URL.Path+" "+string(b))
	}))
	defer fb.Close()
	// "glow.moe" for the test: a port nothing listens on.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().String()
	ln.Close()
	primaryHost = dead
	fbMu.Lock()
	fallback, fbUntil, fbLoaded = fb.URL, time.Time{}, true
	fbMu.Unlock()
	defer func() { primaryHost = "glow.moe" }()

	req, _ := http.NewRequest(http.MethodPost, "http://"+dead+"/api/live/ingest", bytes.NewReader([]byte(`{"a":1}`)))
	resp, err := Do(&http.Client{Timeout: 2 * time.Second}, req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(got) != 1 || got[0] != `/api/live/ingest {"a":1}` {
		t.Fatalf("fallback got %q", got)
	}
	if !time.Now().Before(fbUntil) {
		t.Fatal("a fallback that worked isn't tried first next time")
	}
}

func TestSetFallbackTakesOnlyAGlowSubdomain(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fbMu.Lock()
	fallback, fbLoaded = "", true
	fbMu.Unlock()
	SetFallback("https://evil.example.com")
	SetFallback("http://direct.glow.moe")
	if fallback != "" {
		t.Fatalf("took %q", fallback)
	}
	SetFallback("https://direct.glow.moe/")
	if fallback != "https://direct.glow.moe" {
		t.Fatalf("fallback = %q", fallback)
	}
}
