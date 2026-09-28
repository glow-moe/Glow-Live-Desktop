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

// glowTrayHideHour, glowTrayHideGame and glowTrayShowDiscord are the Discord
// hide items. They only flip state in the orchestrator, so calling them from
// the window procedure is fine.
//
//export glowTrayHideHour
func glowTrayHideHour() { trayHideHour() }

//export glowTrayHideGame
func glowTrayHideGame() { trayHideGame() }

//export glowTrayShowDiscord
func glowTrayShowDiscord() { trayShowDiscord() }
