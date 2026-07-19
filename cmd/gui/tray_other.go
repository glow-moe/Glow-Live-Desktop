//go:build !windows

package main

import "unsafe"

// enableCloseToTray is Windows-only for now; elsewhere the close button behaves
// normally.
func enableCloseToTray(win unsafe.Pointer) {}
