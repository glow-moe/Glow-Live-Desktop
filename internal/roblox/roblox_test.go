package roblox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLastJoin(t *testing.T) {
	log := strings.Join([]string{
		"2026-10-05T12:00:00.100Z,1.0,ab,6 [FLog::Output] ! Joining game 'aaaa-bbbb' place 920587237 at 128.116.1.2",
		"2026-10-05T12:30:00.000Z,2.0,ab,6 [FLog::Network] Time to disconnect replication data: 1",
		"2026-10-05T12:31:00.500Z,3.0,ab,6 [FLog::Output] ! Joining game 'cccc' place 2753915549 at 128.116.9.9",
		"2026-10-05T12:31:05.000Z,3.1,ab,6 [FLog::Output] some other line",
	}, "\n")
	id, at, ok := lastJoin(strings.NewReader(log))
	if !ok || id != 2753915549 || at.IsZero() || at.Minute() != 31 {
		t.Fatalf("got %d %v %v", id, at, ok)
	}
	left := log + "\n2026-10-05T13:00:00.000Z,4,ab,6 [FLog::Network] Client:Disconnect 1"
	if _, _, ok := lastJoin(strings.NewReader(left)); ok {
		t.Fatal("a disconnect after the join means not in a game")
	}
	if _, _, ok := lastJoin(strings.NewReader("nothing here")); ok {
		t.Fatal("no join line")
	}
}

func TestNewestLogSkipsCrashHandler(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	player := filepath.Join(b, "2.741_Player_3F309_last.log")
	crash := filepath.Join(a, "2.741_Player_52EB4_CrashHandler_last.log")
	if err := os.WriteFile(player, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crash, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(crash, later, later) // newer, but not the player's log
	if p, ok := newestLog([]string{a, b}); !ok || p != player {
		t.Fatalf("got %q %v", p, ok)
	}
}
