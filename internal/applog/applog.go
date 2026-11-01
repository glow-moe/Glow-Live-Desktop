// Package applog keeps what the app did lately (starts, status changes,
// errors, updates, the launcher's picks) in a small file on this PC, so a
// member can export it from Settings and send it to glow when something is
// off. Nothing leaves the PC on its own. Tokens never go in.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// maxSize is when app.log rolls over to app.old.log (one old file kept).
const maxSize = 512 << 10

var mu sync.Mutex

// Dir is where the log files live ("" if there's no config dir).
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "glow-collector", "logs")
}

func current() string  { return filepath.Join(Dir(), "app.log") }
func previous() string { return filepath.Join(Dir(), "app.old.log") }

// Printf adds one timestamped line, tagged with the process (launcher or app)
// so the two writing the same file can be told apart.
func Printf(format string, args ...any) {
	if Dir() == "" {
		return
	}
	line := time.Now().Format("2006-01-02 15:04:05") + " [" + role + "] " + fmt.Sprintf(format, args...)
	line = strings.ReplaceAll(line, "\n", " ") + "\n"
	mu.Lock()
	defer mu.Unlock()
	_ = os.MkdirAll(Dir(), 0o755)
	if fi, err := os.Stat(current()); err == nil && fi.Size() > maxSize {
		_ = os.Remove(previous())
		_ = os.Rename(current(), previous())
	}
	f, err := os.OpenFile(current(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(scrub(line))
	_ = f.Close()
}

var role = "app"

// SetRole names this process in the lines it writes ("launcher" or "app").
func SetRole(r string) { role = r }

// CatchCrashes sends a fatal Go error's trace to crash.log, which the export
// includes. A GUI app has no console, so otherwise it's lost.
func CatchCrashes() {
	if Dir() == "" {
		return
	}
	_ = os.MkdirAll(Dir(), 0o755)
	p := filepath.Join(Dir(), "crash.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > maxSize {
		_ = os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	_ = f.Close()
}

var userPathRE = []*regexp.Regexp{
	regexp.MustCompile(`(?i)([A-Z]:[\\/]+Users[\\/]+)[^\\/\s"']+`),
	regexp.MustCompile(`(/(?:home|Users)/)[^/\s"']+`),
}

// scrub drops the account name from file paths (C:\Users\<name>\...).
func scrub(s string) string {
	for _, re := range userPathRE {
		s = re.ReplaceAllString(s, "${1}…")
	}
	return s
}

// Export writes header plus the logs to a text file in Downloads (the config
// dir when there's none) and returns its path.
func Export(header string) (string, error) {
	var b strings.Builder
	b.WriteString(scrub(header))
	for _, p := range []string{filepath.Join(Dir(), "crash.log"), previous(), current()} {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			continue
		}
		b.WriteString("\n===== " + filepath.Base(p) + " =====\n")
		b.WriteString(scrub(string(data)))
	}
	dir := Dir()
	if d := downloadsDir(); d != "" {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			dir = d
		}
	}
	if home, err := os.UserHomeDir(); err == nil && dir == Dir() {
		if fi, err := os.Stat(filepath.Join(home, "Downloads")); err == nil && fi.IsDir() {
			dir = filepath.Join(home, "Downloads")
		}
	}
	out := filepath.Join(dir, "glow-live-logs-"+time.Now().Format("2006-01-02-1504")+".txt")
	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return out, nil
}
