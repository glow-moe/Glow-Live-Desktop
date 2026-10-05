//go:build linux

package main

import "unsafe"

// showWindow brings the hidden window back; same path the tray menu uses.
func showWindow(win unsafe.Pointer) {
	trayWin = win
	showFromTray()
}

// peekWindow is the update prompt's show; GTK has no portable no-focus show.
func peekWindow(win unsafe.Pointer) { showWindow(win) }

// userBusy: no portable full-screen check here.
func userBusy() bool { return false }
