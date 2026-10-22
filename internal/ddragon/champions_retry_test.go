package ddragon

import (
	"testing"
	"time"
)

// An empty champion table (no network when the app started) must not stick
// for the session: once the table loads, lookups and chroma mapping work.
func TestChampionsRetryAfterEmptyLoad(t *testing.T) {
	t.Cleanup(func() {
		SetChampionsForTest(nil)
		skinVidMu.Lock()
		champIDByKey = nil
		skinVidMu.Unlock()
		champsFailAt = time.Time{}
	})
	SetChampionsForTest(map[int]string{})
	champsFailAt = time.Now() // the first load came back empty
	skinVidMu.Lock()
	champIDByKey = nil
	if id := champIDLocked("Akali"); id != 0 {
		t.Fatalf("empty table resolved Akali to %d", id)
	}
	skinVidMu.Unlock()

	SetChampionsForTest(map[int]string{84: "Akali"}) // the retry loaded it
	skinVidMu.Lock()
	id := champIDLocked("Akali")
	skinVidMu.Unlock()
	if id != 84 {
		t.Fatalf("after the table loaded, Akali resolved to %d, want 84", id)
	}
	if !IsChampKey("Akali") {
		t.Fatal("IsChampKey(Akali) false after the table loaded")
	}
}
