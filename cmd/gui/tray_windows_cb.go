//go:build windows

package main

// The //export lives apart from tray_windows.go because cgo forbids exports in
// a file whose preamble defines C functions.

import "C"

// glowTrayProfile is the tray menu's "Open my profile" item.
//
//export glowTrayProfile
func glowTrayProfile() {
	openProfile()
}
