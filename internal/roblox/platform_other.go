//go:build !windows

package roblox

// The Roblox player is Windows-only for this app; nothing to read elsewhere.
func logDirs() []string   { return nil }
func playerRunning() bool { return false }
