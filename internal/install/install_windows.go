//go:build windows

// Package install gives the app one fixed home on Windows. A browser saves the
// download as glow-live-windows-x64(3).exe or similar, and Windows shows that
// file name in Task Manager's Startup apps; so the first run installs itself to
// %LOCALAPPDATA%\Programs\glow L!VE\ and hands off to the launcher there,
// which the autostart entry points at. The launcher starts the newest release
// folder (see versions.go), waits on it, and falls back to the previous one
// when a new build dies before it has run a minute.
package install

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/applog"
	"github.com/glow-moe/glow-collector/internal/update"
)

const dirName = "glow L!VE"

// Exit codes the app hands the launcher to be started again: after an update
// landed in a new folder, back in the tray (automatic) or with the window.
const (
	restartHidden = 3
	restartShown  = 4
)

// Flags between the launcher and the app it starts.
const (
	flagLaunched  = "--launched"  // started by the launcher, which waits on it
	flagLauncher  = "--launcher=" // the launcher's own build
	flagFellBack  = "--fellback=" // this build replaced one that died on start
	flagRestarted = "--restarted" // started again after the last one stopped
	flagMoved     = "--moved"     // opened through a downloaded copy
	flagRelaunch  = "--relaunch"
)

// healthyAfter is how long a build must run before it counts as working here.
const healthyAfter = time.Minute

// Target is the launcher ("" if LOCALAPPDATA is unset).
func Target() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Programs", dirName, exeName)
}

func installDir() string { return filepath.Dir(Target()) }

func selfPath() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	return filepath.Clean(self)
}

func samePath(a, b string) bool {
	return a != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// inInstall reports whether this process runs from the launcher or a release
// folder.
func inInstall() bool {
	self, target := selfPath(), Target()
	if target == "" || self == "" {
		return false
	}
	if samePath(self, target) {
		return true
	}
	rel, err := filepath.Rel(installDir(), self)
	return err == nil && strings.HasPrefix(rel, appPrefix) && !strings.Contains(filepath.Dir(rel), string(filepath.Separator))
}

// Installed reports whether this process runs from its installed home.
func Installed() bool { return inInstall() }

func hasArg(prefix string) (string, bool) {
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix), true
		}
	}
	return "", false
}

// Supervised reports whether the launcher started this copy and waits on it.
func Supervised() bool {
	_, ok := hasArg(flagLaunched)
	return ok
}

// FellBack is the build this one replaced because it died on start ("" if none).
func FellBack() string {
	v, _ := hasArg(flagFellBack)
	return v
}

// userArgs are the launch arguments without the launcher's own flags.
func userArgs() []string {
	var out []string
	for _, a := range os.Args[1:] {
		if a == flagLaunched || a == flagRestarted || a == flagMoved || a == flagRelaunch ||
			strings.HasPrefix(a, flagLauncher) || strings.HasPrefix(a, flagFellBack) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// Launch runs the launcher side of the binary and reports whether it did, in
// which case the caller exits. A release build run from Downloads installs
// itself (unless the install already has this build or a newer one) and hands
// off to the launcher. Run as the launcher, it starts the newest release
// folder and waits on it. A copy the launcher started, a release folder
// opened directly, and dev builds (Relocate) run as the app. Call before the
// single-instance lock: the app the launcher starts takes it.
func Launch(version string) bool {
	if !update.IsRelease(version) || Target() == "" || Supervised() {
		return false
	}
	self := selfPath()
	if samePath(self, Target()) {
		return supervise(version)
	}
	if inInstall() {
		return false
	}
	return installFrom(self, version)
}

// installFrom puts a downloaded copy in place and starts the launcher. An old
// download opened again (people keep the first file and double-click it)
// never goes over a newer install.
func installFrom(self, version string) bool {
	dir := installDir()
	// Decided before anything is copied: the install's newest build, or none
	// (an install from before folders, whose launcher file is the old app).
	// The same build again repairs it; only an older one leaves it alone.
	if newest := pick(dir); newest == "" || update.Compare(version, newest) >= 0 {
		if copyExe(self, versionExe(dir, version)) != nil {
			return false
		}
		if copyExe(self, Target()) != nil {
			return false
		}
	} else if !exists(Target()) {
		if copyExe(self, Target()) != nil {
			return false
		}
	}
	args := append([]string{flagMoved}, userArgs()...)
	return exec.Command(Target(), args...).Start() == nil
}

// supervise is the launcher: start the build to run, wait on it, and act on
// how it ended. A clean exit (tray Quit, or the second copy handing over to
// the first) ends the launcher too. An update's exit code starts the newest
// folder. Anything else is a crash or a kill: a build that never ran a minute
// is marked bad and the previous one starts instead. One that had been running
// fine gets one restart an hour (a crash), so ending it in Task Manager still
// ends it, and a machine where nothing runs stops trying.
func supervise(version string) bool {
	applog.SetRole("launcher")
	dir := installDir()
	_ = os.Remove(Target() + ".old")
	// The hidden copy the old in-place updater (minio selfupdate) left behind.
	_ = os.Remove(filepath.Join(dir, "."+exeName+".old"))
	// The first start of a launcher with no release folder (an install from
	// before folders) puts its own build in one.
	// An old updater that swapped this file for a newer build does the same.
	if newest := pick(dir); !exists(versionExe(dir, version)) && !isBad(dir, version) && (newest == "" || update.Compare(version, newest) > 0) {
		if copyExe(Target(), versionExe(dir, version)) != nil && newest == "" {
			return false
		}
	}
	args := userArgs()
	if _, ok := hasArg(flagMoved); ok {
		args = append(args, flagMoved)
	}
	if _, ok := hasArg(flagRelaunch); ok {
		args = append(args, flagRelaunch)
	}
	fellBack := ""
	var stops []time.Time
	var revived time.Time
	for {
		ver := pick(dir)
		if ver == "" {
			return false // nothing installed works: run this copy as the app
		}
		child := append([]string{}, args...)
		child = append(child, flagLaunched, flagLauncher+version)
		if fellBack != "" {
			child = append(child, flagFellBack+fellBack)
		}
		started := time.Now()
		applog.Printf("launcher %s starts %s %v", version, ver, child)
		cmd := exec.Command(versionExe(dir, ver), child...)
		code, err := run(cmd)
		if err != nil && code < 0 {
			// It would not even start (removed, blocked): same as dying on start.
			applog.Printf("%s would not start: %v", ver, err)
			code = 1
		}
		applog.Printf("%s ended with code %d after %s", ver, code, time.Since(started).Round(time.Second))
		switch code {
		case 0:
			return true
		case restartHidden, restartShown:
			args = []string{flagRelaunch}
			if code == restartHidden {
				args = append(args, "--hidden")
			}
			fellBack = ""
			continue
		}
		// Windows ending the session took the app down: not a crash, and
		// nothing to start again.
		if shuttingDown() {
			applog.Printf("%s ended by Windows shutting down", ver)
			return true
		}
		// A crash or a kill.
		stops = append(stops, time.Now())
		if len(stops) > 3 && time.Since(stops[len(stops)-4]) < time.Hour {
			return true
		}
		if !isHealthy(dir, ver) && time.Since(started) < healthyAfter+30*time.Second && len(versions(dir)) > 1 {
			mark(dir, ver, badMark)
			applog.Printf("%s died on start: marked bad, falling back", ver)
			fellBack = ver
		} else {
			if !revived.IsZero() && time.Since(revived) < time.Hour {
				return true
			}
			revived = time.Now()
		}
		args = []string{flagRestarted, "--hidden"}
	}
}

// run starts cmd and waits, returning its exit code (-1 if it never started).
func run(cmd *exec.Cmd) (int, error) {
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	err := cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// MarkHealthy runs once this build has been up healthyAfter: it marks the
// folder as working here, removes old ones, and brings a launcher from an
// older build up to this one.
func MarkHealthy(version string) {
	if !Supervised() || !update.IsRelease(version) {
		return
	}
	time.AfterFunc(healthyAfter, func() {
		dir := installDir()
		if !samePath(selfPath(), versionExe(dir, version)) {
			return
		}
		mark(dir, version, healthyMark)
		prune(dir, version)
		if lv, _ := hasArg(flagLauncher); !exists(Target()) || update.Compare(version, lv) > 0 {
			_ = copyExe(selfPath(), Target())
		}
	})
}

// Describe is the install for the exported log: where this copy runs and
// each release folder with its marks.
func Describe() string {
	dir := installDir()
	if dir == "" || dir == "." {
		return ""
	}
	where := "outside the install"
	if samePath(selfPath(), Target()) {
		where = "the launcher"
	} else if inInstall() {
		where = filepath.Base(filepath.Dir(selfPath()))
	}
	parts := []string{"running from " + where}
	for _, v := range versions(dir) {
		p := v
		if isHealthy(dir, v) {
			p += " healthy"
		}
		if isBad(dir, v) {
			p += " BAD"
		}
		parts = append(parts, p)
	}
	if !exists(Target()) {
		parts = append(parts, "launcher missing")
	}
	return strings.Join(parts, ", ")
}

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

// IsBad reports whether release `ver` died on this machine before, so the
// updater doesn't fetch it again.
func IsBad(ver string) bool { return Target() != "" && isBad(installDir(), ver) }

// ApplyUpdate installs release `tag`: into its own folder when the launcher
// runs this copy, else over the running exe the old way.
func ApplyUpdate(tag string) error {
	if !Supervised() {
		return update.Apply(tag)
	}
	return update.Download(tag, versionExe(installDir(), tag))
}

// Restart starts the freshly installed build. Under the launcher that's an exit
// code it acts on; otherwise the old relaunch. Does not return on success.
func Restart(hidden bool) {
	if Supervised() {
		if hidden {
			os.Exit(restartHidden)
		}
		os.Exit(restartShown)
	}
	if hidden {
		update.Relaunch("--hidden")
	}
	update.Relaunch()
}

// Relocate is the dev-build path: a test build run from anywhere copies itself
// over the launcher and runs from there, as before release folders. Release
// builds go through Launch. It reports whether it handed off, in which case
// the caller should exit. Call it while holding the single-instance lock: the
// new copy waits for it (--relaunch).
func Relocate(version string) bool {
	target := Target()
	if target == "" || update.IsRelease(version) || inInstall() {
		return false
	}
	if err := copyExe(selfPath(), target); err != nil {
		return false
	}
	args := append([]string{flagRelaunch}, userArgs()...)
	return exec.Command(target, args...).Start() == nil
}
