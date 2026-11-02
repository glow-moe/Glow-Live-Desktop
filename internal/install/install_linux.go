//go:build linux

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The install lives in ~/.local/share/glow-live/ (XDG_DATA_HOME), like other
// per-user programs: the launcher glow-live, and app-26.11/glow-live etc.
const (
	dirName = "glow-live"
	exeName = "glow-live"
)

// dataHome is XDG_DATA_HOME, or ~/.local/share ("" with no home).
func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

// Target is the launcher ("" with no home folder).
func Target() string {
	base := dataHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, dirName, exeName)
}

func samePath(a, b string) bool {
	return a != "" && filepath.Clean(a) == filepath.Clean(b)
}

// detach starts the launcher in a session of its own, so closing the terminal
// the download was opened from doesn't take the app with it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// NoteShutdown is a no-op on Linux: logging out ends the launcher too.
func NoteShutdown() {}

func shuttingDown() bool { return false }

// menuPath is the app menu entry for this user.
func menuPath() string {
	base := dataHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "applications", "glow-live.desktop")
}

// iconPath is the icon the app writes for its tray on first run.
func iconPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "glow-collector", "glow-live.png")
}

// shortcutMark says the menu entry was made once, so one the person deleted
// on purpose doesn't come back on every start.
func shortcutMark() string { return filepath.Join(installDir(), "shortcut") }

// ensureShortcut adds glow L!VE to the app menu (it then shows in search and
// the app grid), pointing at the launcher, with Uninstall on its right-click.
// Once per install.
func ensureShortcut() {
	p, target := menuPath(), Target()
	if p == "" || target == "" || exists(shortcutMark()) {
		return
	}
	if !exists(p) {
		if os.MkdirAll(filepath.Dir(p), 0o755) != nil || os.WriteFile(p, []byte(menuEntry(target, iconPath())), 0o644) != nil {
			return
		}
	}
	_ = os.WriteFile(shortcutMark(), nil, 0o644)
}

func menuEntry(target, icon string) string {
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=glow L!VE
Comment=Your games, live on glow.moe
Exec="%[1]s"
Icon=%[2]s
Terminal=false
Categories=Game;Utility;
StartupWMClass=glow-live
Actions=uninstall;

[Desktop Action uninstall]
Name=Uninstall glow L!VE
Exec="%[1]s" --uninstall
`, target, icon)
}

// Register is a no-op on Linux: the menu entry is made by the launcher.
func Register(string) {}

// Uninstall is "Uninstall glow L!VE" on the menu entry's right-click: it
// closes the running app, removes the autostart and menu entries, the app's
// settings and log, and the install folder. The key on glow.moe stays until
// it's removed from the L!VE page.
func Uninstall(quiet bool) {
	quitRunningCopy()
	if base, err := os.UserConfigDir(); err == nil {
		_ = os.Remove(filepath.Join(base, "autostart", "glow-live.desktop"))
		_ = os.RemoveAll(filepath.Join(base, "glow-collector"))
	}
	if p := menuPath(); p != "" {
		_ = os.Remove(p)
	}
	if dir := installDir(); Target() != "" && strings.HasSuffix(dir, string(filepath.Separator)+dirName) {
		_ = os.RemoveAll(dir)
	}
	if !quiet {
		const msg = "glow L!VE was removed from this PC."
		fmt.Println(msg)
		if p, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command(p, "glow L!VE", msg).Run()
		}
	}
}
