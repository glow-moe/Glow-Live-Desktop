//go:build !windows

package install

// Relocate is a no-op off Windows: Linux runs from wherever the user put the
// binary, and its autostart entry names the app itself.
func Relocate() bool { return false }
