package update

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	holdURL = srv.URL
	t.Cleanup(func() {
		hostedMu.Lock()
		hosted = nil
		hostedMu.Unlock()
	})
}

func TestAHostedReleaseIsTheOneToTake(t *testing.T) {
	sha := strings.Repeat("b", 64)
	serve(t, `{"hold":[],"latest":{"tag":"v26.12","publishedAt":"2026-12-01T00:00:00Z","notes":["New thing"],
		"files":{"`+assetName()+`":{"url":"https://glow.moe/api/live/download/v26.12/`+assetName()+`","sha256":"`+sha+`"}}}}`)
	tag, _, available := Check("v26.11")
	if tag != "v26.12" || !available {
		t.Fatalf("Check = %q %v", tag, available)
	}
	if got := assetURL("v26.12"); got != "https://glow.moe/api/live/download/v26.12/"+assetName() {
		t.Fatalf("assetURL = %q", got)
	}
	if got, _ := expectedSHA("v26.12"); got != sha {
		t.Fatalf("sha = %q", got)
	}
	if n := Notes("v26.12"); len(n) != 1 || n[0] != "New thing" {
		t.Fatalf("notes = %q", n)
	}
}

func TestAHostedFileOffGlowIsIgnored(t *testing.T) {
	serve(t, `{"hold":[],"latest":{"tag":"v26.12","publishedAt":"2026-12-01T00:00:00Z",
		"files":{"`+assetName()+`":{"url":"https://evil.example.com/x.exe","sha256":"`+strings.Repeat("c", 64)+`"}}}}`)
	if _, r, ok := releaseInfo(); !ok || r != nil {
		t.Fatalf("took a release from another host: %+v", r)
	}
	if got := assetURL("v26.12"); !strings.HasPrefix(got, downloadBase) {
		t.Fatalf("not GitHub: %q", got)
	}
}

// Right after an update the new copy asks for its notes before anything has
// checked glow.moe in this run: they come from glow.moe all the same.
func TestNotesAskGlowWhenNothingHasYet(t *testing.T) {
	serve(t, `{"hold":[],"latest":{"tag":"v26.12","publishedAt":"2026-12-01T00:00:00Z","notes":["Fresh"],
		"files":{"`+assetName()+`":{"url":"https://glow.moe/x","sha256":"`+strings.Repeat("d", 64)+`"}}}}`)
	if n := Notes("v26.12"); len(n) != 1 || n[0] != "Fresh" {
		t.Fatalf("notes = %q", n)
	}
}
