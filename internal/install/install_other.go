//go:build !windows

package install

import "github.com/glow-moe/glow-collector/internal/update"

// Off Windows the app runs from wherever the user put it (Linux) or from its
// .app bundle (macOS), and updates swap the binary in place: no launcher.

// Launch is a no-op off Windows.
func Launch(string) bool { return false }

// Relocate is a no-op off Windows: Linux runs from wherever the user put the
// binary, and its autostart entry names the app itself.
func Relocate(string) bool { return false }

// Installed is always true off Windows: wherever the app runs is its home.
func Installed() bool { return true }

// Supervised is always false off Windows (no launcher).
func Supervised() bool { return false }

// FellBack is always "" off Windows.
func FellBack() string { return "" }

// MarkHealthy is a no-op off Windows.
func MarkHealthy(string) {}

// Describe is "" off Windows (no release folders).
func Describe() string { return "" }

// NoteShutdown is a no-op off Windows.
func NoteShutdown() {}

// IsBad is always false off Windows.
func IsBad(string) bool { return false }

// ApplyUpdate swaps the running binary for release `tag`.
func ApplyUpdate(tag string) error { return update.Apply(tag) }

// Restart starts the swapped binary. Does not return on success.
func Restart(hidden bool) {
	if hidden {
		update.Relaunch("--hidden")
	}
	update.Relaunch()
}
