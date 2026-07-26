//go:build linux

package main

// The //export functions live apart from tray_linux.go because cgo forbids
// exports in a file whose preamble contains C definitions.

import "C"

// glowTrayOpen is the tray menu's Open item. Runs inside the GTK main loop, so
// touching the window here is safe.
//
//export glowTrayOpen
func glowTrayOpen() {
	showFromTray()
}

// glowTrayQuit is the tray menu's Quit item, the app's real exit.
//
//export glowTrayQuit
func glowTrayQuit() {
	if trayTerminate != nil {
		trayTerminate()
	}
}
