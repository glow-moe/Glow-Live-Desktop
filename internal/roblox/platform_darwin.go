//go:build darwin

package roblox

import (
	"os"
	"path/filepath"

	"github.com/glow-moe/glow-collector/internal/proc"
)

// logDirs: the Mac client logs to ~/Library/Logs/Roblox, in the same format
// as the Windows one.
func logDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, "Library", "Logs", "Roblox")}
}

// playerRunning: is the Roblox game client (RobloxPlayer.app, not Studio) open?
func playerRunning() bool { return proc.Running("RobloxPlayer") }
