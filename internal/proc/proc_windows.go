//go:build windows

// Package proc answers "is this game's client open?" from the process list
// (Roblox, Fortnite), without opening or touching the process.
package proc

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Running reports whether a process with one of these exe names is running.
func Running(exes ...string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := windows.UTF16ToString(e.ExeFile[:])
		for _, x := range exes {
			if strings.EqualFold(name, x) {
				return true
			}
		}
	}
	return false
}
