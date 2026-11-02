//go:build windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The install lives in %LOCALAPPDATA%\Programs\glow L!VE\.
const (
	dirName = "glow L!VE"
	exeName = "glow L!VE.exe"
)

// Target is the launcher ("" if LOCALAPPDATA is unset).
func Target() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Programs", dirName, exeName)
}

func samePath(a, b string) bool {
	return a != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// detach is a no-op here: a started copy outlives this one anyway.
func detach(*exec.Cmd) {}

// shutdownMark is touched by the app when Windows signs out or shuts down, so
// the launcher doesn't take the app being ended for a crash.
func shutdownMark() string { return filepath.Join(installDir(), "shutdown") }

// NoteShutdown records that Windows is ending the session (WM_ENDSESSION).
func NoteShutdown() {
	if Target() != "" {
		_ = os.WriteFile(shutdownMark(), []byte(time.Now().Format(time.RFC3339)), 0o644)
	}
}

// shuttingDown reports a shutdown noted in the last minute.
func shuttingDown() bool {
	fi, err := os.Stat(shutdownMark())
	return err == nil && time.Since(fi.ModTime()) < time.Minute
}
