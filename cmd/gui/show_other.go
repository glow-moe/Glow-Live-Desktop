//go:build !linux && !windows

package main

import "unsafe"

// showWindow is a no-op without a native implementation.
func showWindow(win unsafe.Pointer) {}

func peekWindow(win unsafe.Pointer) {}

func userBusy() bool { return false }
