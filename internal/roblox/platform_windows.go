//go:build windows

package roblox

import (
	"os"
	"path/filepath"

	"github.com/glow-moe/glow-collector/internal/proc"
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
func playerRunning() bool { return proc.Running("RobloxPlayerBeta.exe") }
