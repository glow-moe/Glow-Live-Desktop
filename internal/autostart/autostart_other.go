//go:build !linux && !windows

package autostart

// Set is a no-op on platforms without an autostart implementation.
func Set(bool) error { return nil }
