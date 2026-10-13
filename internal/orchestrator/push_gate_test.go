package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/snapshot"
)

func lobbyFrame(label string, raw map[string]string) snapshot.Snapshot {
	s := snapshot.Snapshot{Game: "lol", Phase: "Lobby", UpdatedAt: time.Now().UnixMilli(), Lobby: &snapshot.Lobby{Phase: "Lobby", Label: label}}
	if raw != nil {
		s.Raw = map[string]json.RawMessage{}
		for k, v := range raw {
			s.Raw[k] = json.RawMessage(v)
		}
	}
	return s
}

func TestPushGate(t *testing.T) {
	o := &Orchestrator{}
	t0 := time.Unix(1000, 0)

	b, ok := o.gate(lobbyFrame("In a lobby", map[string]string{"lobby": `{"a":1}`, "honor": `{"h":1}`}), false, t0)
	if !ok || !strings.Contains(string(b), `"_full":true`) || !strings.Contains(string(b), `"honor"`) {
		t.Fatalf("first frame goes out in full: %s", b)
	}
	o.sent(b, t0)

	// Same frame (new timestamp) inside the heartbeat: skipped.
	if _, ok := o.gate(lobbyFrame("In a lobby", map[string]string{"lobby": `{"a":1}`, "honor": `{"h":1}`}), false, t0.Add(4*time.Second)); ok {
		t.Fatal("an unchanged frame should be skipped")
	}
	// A change within 3s of the last push waits.
	if _, ok := o.gate(lobbyFrame("In queue", nil), false, t0.Add(2*time.Second)); ok {
		t.Fatal("out of game: at most one frame every 3s")
	}
	// A change after 3s goes out, with only the raw key that changed and null for the gone one.
	b, ok = o.gate(lobbyFrame("In a lobby", map[string]string{"lobby": `{"a":2}`}), false, t0.Add(4*time.Second))
	if !ok {
		t.Fatal("a changed frame goes out")
	}
	s := string(b)
	if !strings.Contains(s, `"lobby":{"a":2}`) || !strings.Contains(s, `"honor":null`) || strings.Contains(s, "_full") {
		t.Fatalf("delta: %s", s)
	}
	o.sent(b, t0.Add(4*time.Second))

	// Unchanged raw: no raw at all, and the heartbeat resends after 10s.
	b, ok = o.gate(lobbyFrame("In a lobby", map[string]string{"lobby": `{"a":2}`}), false, t0.Add(15*time.Second))
	if !ok || strings.Contains(string(b), `"raw"`) {
		t.Fatalf("heartbeat without raw: %v %s", ok, b)
	}
	o.sent(b, t0.Add(15*time.Second))

	// Every 60s the raw set goes out in full again.
	b, ok = o.gate(lobbyFrame("In a lobby", map[string]string{"lobby": `{"a":2}`}), false, t0.Add(70*time.Second))
	if !ok || !strings.Contains(string(b), `"_full":true`) {
		t.Fatalf("full resend: %s", b)
	}

	// A failed push forgets everything: the next frame is full and immediate.
	o.failed()
	b, ok = o.gate(lobbyFrame("In a lobby", map[string]string{"lobby": `{"a":2}`}), false, t0.Add(71*time.Second))
	if !ok || !strings.Contains(string(b), `"_full":true`) {
		t.Fatalf("after a failure: %s", b)
	}

	// In a match every changed frame goes out at once.
	o.sent(b, t0.Add(71*time.Second))
	live := snapshot.Snapshot{Game: "lol", Live: true, Clock: "01:00"}
	if _, ok := o.gate(live, true, t0.Add(71500*time.Millisecond)); !ok {
		t.Fatal("in-game frames are not held back")
	}
}
