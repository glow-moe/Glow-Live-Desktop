package orchestrator

import (
	"testing"
	"time"
)

// Hide on Discord: for a while, or until the game it was set in ends.
func TestHideOnDiscord(t *testing.T) {
	o := &Orchestrator{}

	// Nothing running: "until I leave this game" has nothing to hold on to.
	if o.HideOnDiscordForGame() {
		t.Fatal("hide-for-game with no game must refuse")
	}

	o.HideOnDiscord(time.Hour)
	st := o.Status()
	if !st.DiscordHidden || st.DiscordHiddenUntil == 0 {
		t.Fatalf("timed hide: %+v", st)
	}
	o.ShowOnDiscord()
	if o.Status().DiscordHidden {
		t.Fatal("show must end the hide")
	}

	// A timed hide that ran out ends on its own.
	o.mu.Lock()
	o.dcHideUntil = time.Now().Add(-time.Second)
	o.mu.Unlock()
	if o.Status().DiscordHidden {
		t.Fatal("expired hide still on")
	}

	// Hide for this game: holds while that game runs, ends when it stops or
	// another one starts.
	o.mu.Lock()
	o.status = Status{Game: "league", InGame: true}
	o.mu.Unlock()
	if !o.HideOnDiscordForGame() {
		t.Fatal("hide-for-game refused while in a game")
	}
	if st := o.Status(); !st.DiscordHidden || st.DiscordHiddenUntil != 0 {
		t.Fatalf("game hide: %+v", st)
	}
	o.mu.Lock()
	o.status = Status{Game: "forza", InGame: true}
	o.mu.Unlock()
	if o.Status().DiscordHidden {
		t.Fatal("a different game must end a game hide")
	}

	// presence() stays quiet while hidden and reports no error.
	o.HideOnDiscord(time.Minute)
	if err := o.presence("123", splitcraftActivity(splitSnap{}, "")); err != nil {
		t.Fatalf("presence while hidden returned %v", err)
	}
}
