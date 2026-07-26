//go:build linux

package main

import "unsafe"

// showWindow brings the hidden window back; same path the tray menu uses.
func showWindow(win unsafe.Pointer) {
	trayWin = win
	showFromTray()
}
