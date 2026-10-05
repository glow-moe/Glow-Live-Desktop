//go:build windows

package roblox

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// logDirs: the website installer's client logs to Roblox\logs, the Microsoft
// Store build to RobloxPCGDK\logs. Same log format in both.
func logDirs() []string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return nil
	}
	return []string{filepath.Join(base, "Roblox", "logs"), filepath.Join(base, "RobloxPCGDK", "logs")}
}

// playerRunning: is the Roblox game client (not the website, not Studio) open?
func playerRunning() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "RobloxPlayerBeta.exe") {
			return true
		}
	}
	return false
}
