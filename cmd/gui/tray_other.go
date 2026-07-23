//go:build !windows

package main

import "unsafe"

// enableCloseToTray is Windows-only for now; elsewhere the close button behaves
// normally.
func enableCloseToTray(win unsafe.Pointer) {}

// hideToTray is a no-op off Windows (no tray to hide into).
func hideToTray(win unsafe.Pointer) {}
