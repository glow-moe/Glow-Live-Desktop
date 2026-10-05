//go:build !windows

package roblox

// The Roblox player is Windows-only for this app; nothing to read elsewhere.
func logDir() string      { return "" }
func playerRunning() bool { return false }
