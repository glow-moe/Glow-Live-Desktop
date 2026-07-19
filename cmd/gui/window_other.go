//go:build !linux

package main

import "unsafe"

// moveBottomRight is a no-op off Linux for now (Windows/macOS corner-parking
// would use their own window APIs).
func moveBottomRight(win unsafe.Pointer, w, h int) {}
