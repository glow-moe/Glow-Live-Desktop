//go:build windows

package install

import (
	"os"
	"testing"
)

// The app ended because Windows is shutting down: no restart, no bad mark,
// even for a build that hasn't run a minute yet.
func TestSuperviseLeavesAShutdownAlone(t *testing.T) {
	out := setup(t, "app-26.12=1")
	os.Args = []string{os.Args[0]}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.11"))
	mark(dir, "v26.11", healthyMark)
	installTestExe(t, versionExe(dir, "v26.12"))
	NoteShutdown()

	if !supervise("v26.12") {
		t.Fatal("supervise gave up")
	}
	if lines := runs(t, out, 1); len(lines) != 1 {
		t.Fatalf("started again: %q", lines)
	}
	if isBad(dir, "v26.12") {
		t.Fatal("a shutdown marked the build bad")
	}
}

// The Start menu entry is made once and points at the launcher; one deleted
// afterwards stays deleted.
func TestShortcutOnce(t *testing.T) {
	_ = setup(t, "")
	installTestExe(t, Target())
	ensureShortcut()
	if !exists(shortcutPath()) {
		t.Fatal("no Start menu shortcut")
	}
	_ = os.Remove(shortcutPath())
	ensureShortcut()
	if exists(shortcutPath()) {
		t.Fatal("a deleted shortcut came back")
	}
}
