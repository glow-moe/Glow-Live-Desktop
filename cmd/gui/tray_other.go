//go:build !windows && !linux

package main

import "unsafe"

// enableCloseToTray has native implementations on Windows and Linux; elsewhere
// the close button behaves normally.
func enableCloseToTray(win unsafe.Pointer) {}

// hideToTray is a no-op off Windows (no tray to hide into).
func hideToTray(win unsafe.Pointer) {}
