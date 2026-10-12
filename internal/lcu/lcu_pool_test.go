package lcu

import (
	"os"
	"path/filepath"
	"testing"
)

// Every dial must reuse one HTTP client: a fresh Transport per poll leaked a
// keep-alive socket each tick until the League client refused new connections.
func TestDialSharesOneClient(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lockfile"), []byte("LeagueClient:1:51302:pw:https"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := dial(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := dial(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.http != b.http || a.http != lcuClient {
		t.Fatal("each dial built its own HTTP client")
	}
	if lcuClient.Timeout == 0 {
		t.Fatal("LCU calls need a timeout")
	}
}
