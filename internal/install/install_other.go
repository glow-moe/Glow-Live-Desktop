//go:build !windows && !linux

package install

import "github.com/glow-moe/glow-collector/internal/update"

// On macOS the app runs from its .app bundle, and updates swap the binary
// inside it in place: no launcher.

// exeName is unused here (no release folders); versions.go names it.
const exeName = "glow-live"

// Launch is a no-op on macOS.
func Launch(string) bool { return false }

// Relocate is a no-op on macOS.
func Relocate(string) bool { return false }

// Installed is always true on macOS: the bundle is its home.
func Installed() bool { return true }

// Supervised is always false on macOS (no launcher).
func Supervised() bool { return false }

// FellBack is always "" on macOS.
func FellBack() string { return "" }

// MarkHealthy is a no-op on macOS.
func MarkHealthy(string) {}

// Describe is "" on macOS (no release folders).
func Describe() string { return "" }

// NoteShutdown is a no-op on macOS.
func NoteShutdown() {}

// Register is a no-op on macOS.
func Register(string) {}

// Uninstall is a no-op on macOS (the bundle goes to the Trash).
func Uninstall(bool) {}

// IsBad is always false on macOS.
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
