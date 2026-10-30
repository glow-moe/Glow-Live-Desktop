//go:build !windows

// Package proc answers "is this game's client open?" from the process list
// (Roblox, Fortnite), without opening or touching the process.
package proc

// Running is Windows only; the games it's asked about are Windows only too.
func Running(...string) bool { return false }
