//go:build windows

package main

import "unsafe"

// moveBottomRight parks the widget in the bottom-right corner (above the
// taskbar), always-on-top, and snaps it back there if the user drags it away -
// the same pinned-widget behaviour as the Linux build. The heavy lifting lives
// in the tray_windows.go cgo block so the window subclass stays a single unit.
func moveBottomRight(win unsafe.Pointer, w, h int) {
	pinBottomRight(win, w, h)
}
