//go:build linux || darwin

// Package single keeps the app to one running copy per user. The lock is the
// OS's own (flock here, a named mutex on Windows), so a crash releases it by
// itself and no stale pid file can wedge the next start.
package single

import (
	"os"
	"path/filepath"
	"syscall"
)

// held keeps the descriptor referenced for the process lifetime; the lock
// lives exactly as long as the process does.
var held *os.File

// Acquire reports whether this process is the only copy. false means another
// copy already holds the lock.
func Acquire() bool {
	base, err := os.UserConfigDir()
	if err != nil {
		return true // no config dir to lock in; better to run than to refuse
	}
	dir := filepath.Join(base, "glow-collector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return true
	}
	f, err := os.OpenFile(filepath.Join(dir, "app.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	held = f
	return true
}
