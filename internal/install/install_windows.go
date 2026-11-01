//go:build windows

// Package install gives the app one fixed home on Windows. A browser saves the
// download as glow-live-windows-x64(3).exe or similar, and Windows shows that
// file name in Task Manager's Startup apps; so the first run copies itself to
// %LOCALAPPDATA%\Programs\glow L!VE\glow L!VE.exe and hands off to that copy,
// which is the one the autostart entry and the updater then use.
package install

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/glow-moe/glow-collector/internal/update"
)

const (
	dirName = "glow L!VE"
	exeName = "glow L!VE.exe"
)

// Target is where the app lives once installed ("" if LOCALAPPDATA is unset).
func Target() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Programs", dirName, exeName)
}

// versionFile sits next to the installed exe and names the build in it, so a
// copy run from Downloads can tell whether the install is newer than itself.
func versionFile() string { return filepath.Join(filepath.Dir(Target()), "version") }

// MarkVersion records the build running from Target. Main calls it on every
// start of the installed copy, updates included. Best-effort.
func MarkVersion(version string) {
	if Installed() {
		_ = os.WriteFile(versionFile(), []byte(version), 0o644)
	}
}

// Installed reports whether this process runs from Target.
func Installed() bool {
	target := Target()
	self, err := os.Executable()
	if target == "" || err != nil {
		return false
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	return strings.EqualFold(filepath.Clean(self), filepath.Clean(target))
}

// Relocate copies the running exe to Target and starts that copy with the same
// arguments, when the app runs from anywhere else. It reports whether it
// handed off, in which case the caller should exit. Call it while holding the
// single-instance lock: the new copy waits for it (--relaunch). Any failure
// leaves the app running from where it is. An old download opened again
// (people keep the first file and double-click it) never overwrites a newer
// install: it just starts the installed copy. --moved tells that copy it was
// opened through another file.
func Relocate(version string) bool {
	target := Target()
	if target == "" || Installed() {
		return false
	}
	self, err := os.Executable()
	if err != nil {
		return false
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	installed, _ := os.ReadFile(versionFile())
	keep := update.IsRelease(version) && update.IsRelease(string(installed)) && update.Compare(version, string(installed)) < 0
	if _, err := os.Stat(target); err != nil {
		keep = false
	}
	if !keep {
		if err := copyExe(self, target); err != nil {
			return false
		}
	}
	args := []string{"--relaunch", "--moved"}
	for _, a := range os.Args[1:] {
		if a != "--relaunch" && a != "--moved" {
			args = append(args, a)
		}
	}
	if err := exec.Command(target, args...).Start(); err != nil {
		return false
	}
	return true
}

// copyExe writes src over dst through a temp file, so a half-written copy
// never replaces a working one.
func copyExe(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
