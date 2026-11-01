//go:build !windows

package install

// Relocate is a no-op off Windows: Linux runs from wherever the user put the
// binary, and its autostart entry names the app itself.
func Relocate(string) bool { return false }

// MarkVersion is a no-op off Windows (no fixed install to mark).
func MarkVersion(string) {}

// Installed is always true off Windows: wherever the app runs is its home.
func Installed() bool { return true }
