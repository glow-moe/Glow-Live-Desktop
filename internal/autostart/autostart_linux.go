//go:build linux

// Package autostart makes the app launch at login, using whatever the OS
// already provides for that: an XDG autostart entry here, a registry Run value
// on Windows. Set is idempotent and Set(false) removes the artifact entirely.
package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func entryPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "autostart", "glow-live.desktop"), nil
}

// Set writes or removes the XDG autostart entry. Every mainstream desktop
// (KDE, GNOME, Cinnamon, XFCE) honors this directory.
func Set(on bool) error {
	p, err := entryPath()
	if err != nil {
		return err
	}
	if !on {
		err := os.Remove(p)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// The icon is the one the GUI writes on first run for the tray; a launcher
	// that reads this entry before then just shows its stand-in once.
	icon := ""
	if base, err := os.UserConfigDir(); err == nil {
		icon = filepath.Join(base, "glow-collector", "glow-live.png")
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=glow L!VE
Comment=Live game status for your glow.moe profile
Exec="%s"
Icon=%s
Terminal=false
X-GNOME-Autostart-enabled=true
`, exe, icon)
	return os.WriteFile(p, []byte(desktop), 0o644)
}
