package snapshot

import (
	"testing"

	"github.com/glow-moe/glow-collector/internal/ddragon"
)

func TestChampKeyShapes(t *testing.T) {
	ddragon.SetChampionsForTest(map[int]string{64: "LeeSin", 147: "Seraphine", 145: "Kaisa"})
	cases := []struct{ raw, display, want string }{
		{"game_character_displayname_LeeSin", "Lee Sin", "LeeSin"},
		{"game_character_displayname_Seraphine", "Seraphine", "Seraphine"},
		// The shape Seraphine's raw name came through in: the display name wins.
		{"weird_shape_Name", "Seraphine", "Seraphine"},
		// Nothing known: the raw guess goes out, as before.
		{"weird_shape_Name", "Unknown Thing", "Name"},
		{"", "Unknown Thing", "UnknownThing"},
	}
	for _, c := range cases {
		if got := champKey(c.raw, c.display); got != c.want {
			t.Errorf("champKey(%q, %q) = %q, want %q", c.raw, c.display, got, c.want)
		}
	}
}
