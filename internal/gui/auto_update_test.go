package gui

import (
	"testing"
	"time"
)

func TestAutoUpdateDue(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	old := now.Add(-25 * time.Hour)
	fresh := now.Add(-2 * time.Hour)
	cases := []struct {
		name   string
		on     bool
		ver    string
		failed string
		at     time.Time
		inGame bool
		want   bool
	}{
		{"out a day, idle", true, "v26.10", "", old, false, true},
		{"setting off", false, "v26.10", "", old, false, false},
		{"no update", true, "", "", old, false, false},
		{"too fresh", true, "v26.10", "", fresh, false, false},
		{"exactly a day", true, "v26.10", "", now.Add(-autoUpdateAfter), false, true},
		{"in a game", true, "v26.10", "", old, true, false},
		{"already failed this one", true, "v26.10", "v26.10", old, false, false},
		{"failed an older one", true, "v26.10", "v26.9", old, false, true},
		{"unknown publish date", true, "v26.10", "", time.Time{}, false, false},
	}
	for _, c := range cases {
		if got := autoUpdateDue(c.on, c.ver, c.failed, c.at, now, c.inGame); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
