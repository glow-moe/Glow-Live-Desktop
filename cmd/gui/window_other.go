//go:build !linux && !windows

package main

import "unsafe"

// moveBottomRight is a no-op on the remaining platforms (macOS corner-parking
// would use its own window APIs). Linux and Windows have real implementations.
func moveBottomRight(win unsafe.Pointer, w, h int) {}
