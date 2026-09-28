package snapshot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/glow-moe/glow-collector/internal/live"
)

// A game with nobody on a side (Practice Tool) and no events yet must still
// send arrays: the web renderer maps over them.
func TestEmptyTeamsAndFeedEncodeAsArrays(t *testing.T) {
	s := Build(&live.AllGameData{}, "", 0)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{`"blue":[]`, `"red":[]`, `"feed":[]`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}
