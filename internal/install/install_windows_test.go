//go:build windows

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The handed-off copy is this test binary itself: started from Target with
// GLOW_INSTALL_CHILD set, it records where it ran and with which arguments.
func TestMain(m *testing.M) {
	if out := os.Getenv("GLOW_INSTALL_CHILD"); out != "" {
		self, _ := os.Executable()
		_ = os.WriteFile(out, []byte(self+"\n"+strings.Join(os.Args[1:], " ")), 0o600)
		// Run from the target already: nothing to do.
		if Relocate() {
			_ = os.WriteFile(out+".again", nil, 0o600)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRelocateCopiesAndHandsOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	out := filepath.Join(dir, "child.txt")
	t.Setenv("GLOW_INSTALL_CHILD", out)
	os.Args = []string{os.Args[0], "--hidden"}

	if !Relocate() {
		t.Fatal("Relocate did not hand off")
	}
	want := filepath.Join(dir, "Programs", "glow L!VE", "glow L!VE.exe")
	if Target() != want {
		t.Fatalf("Target = %q, want %q", Target(), want)
	}
	if _, err := os.Stat(want + ".new"); !os.IsNotExist(err) {
		t.Fatal("temp copy left behind")
	}

	var b []byte
	for i := 0; i < 100; i++ {
		if b, _ = os.ReadFile(out); len(b) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	lines := strings.SplitN(string(b), "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("child never reported: %q", b)
	}
	if !strings.EqualFold(lines[0], want) {
		t.Fatalf("child ran from %q, want %q", lines[0], want)
	}
	if lines[1] != "--relaunch --hidden" {
		t.Fatalf("child args %q", lines[1])
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(out + ".again"); err == nil {
		t.Fatal("the installed copy tried to relocate again")
	}
}
