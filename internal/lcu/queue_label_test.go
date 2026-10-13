package lcu

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A queue id the table doesn't know is named by the client's own queue list,
// so a new mode (ARAM: Mayhem, Classic Rift) never shows up as "Custom".
func TestQueueLabelAsksTheClientForNewQueues(t *testing.T) {
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.URL.Path == "/lol-game-queues/v1/queues/9999" {
			_, _ = w.Write([]byte(`{"name":"","description":"Brand New Mode"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := &conn{base: srv.URL, http: srv.Client()}

	if got := c.queueLabel(2400); got != "ARAM: Mayhem" {
		t.Fatalf("2400 = %q", got)
	}
	if got := c.queueLabel(9999); got != "Brand New Mode" {
		t.Fatalf("9999 = %q", got)
	}
	c.queueLabel(9999)
	if asked != 1 {
		t.Fatalf("asked the client %d times, want 1 (cached)", asked)
	}
	if got := c.queueLabel(8888); got != "Custom" {
		t.Fatalf("unknown everywhere = %q", got)
	}
}
