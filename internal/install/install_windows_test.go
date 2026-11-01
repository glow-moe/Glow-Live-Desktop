//go:build windows

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The started copies are this test binary itself: run with GLOW_INSTALL_CHILD
// set, it appends where it ran and with which arguments to that file, then
// exits with the code GLOW_INSTALL_EXIT names for its folder ("app-26.12=1").
func TestMain(m *testing.M) {
	if out := os.Getenv("GLOW_INSTALL_CHILD"); out != "" {
		self := selfPath()
		f, _ := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		f.WriteString(self + "|" + strings.Join(os.Args[1:], " ") + "\n")
		f.Close()
		if v := os.Getenv("GLOW_INSTALL_FETCH"); v != "" && !strings.Contains(self, "app-"+strings.TrimPrefix(v, "v")) {
			_ = copyExe(self, versionExe(installDir(), v))
		}
		for _, rule := range strings.Split(os.Getenv("GLOW_INSTALL_EXIT"), ",") {
			if k, v, ok := strings.Cut(rule, "="); ok && strings.Contains(self, k) {
				code := 0
				for _, c := range v {
					code = code*10 + int(c-'0')
				}
				os.Exit(code)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// setup points LOCALAPPDATA at a temp dir and returns the child log path.
func setup(t *testing.T, exit string) string {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	// The launcher logs (internal/applog); keep that out of the real config dir.
	t.Setenv("APPDATA", t.TempDir())
	out := filepath.Join(t.TempDir(), "child.txt")
	t.Setenv("GLOW_INSTALL_CHILD", out)
	t.Setenv("GLOW_INSTALL_EXIT", exit)
	return out
}

func installTestExe(t *testing.T, dst string) {
	t.Helper()
	self, _ := os.Executable()
	if err := copyExe(self, dst); err != nil {
		t.Fatal(err)
	}
}

func runs(t *testing.T, out string, want int) []string {
	t.Helper()
	var lines []string
	for i := 0; i < 100; i++ {
		b, _ := os.ReadFile(out)
		lines = strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(b) > 0 && len(lines) >= want {
			return lines
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("wanted %d runs, got %q", want, lines)
	return nil
}

// A new build that dies on start (an antivirus removed it, a crash) is marked
// bad and the launcher starts the previous one, telling it what happened.
func TestSuperviseFallsBackFromABuildThatDiesOnStart(t *testing.T) {
	out := setup(t, "app-26.12=1")
	os.Args = []string{os.Args[0], "--hidden"}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.11"))
	mark(dir, "v26.11", healthyMark)
	installTestExe(t, versionExe(dir, "v26.12"))

	if !supervise("v26.12") {
		t.Fatal("supervise gave up")
	}
	lines := runs(t, out, 2)
	if !strings.Contains(lines[0], `app-26.12\glow L!VE.exe|--hidden --launched --launcher=v26.12`) {
		t.Fatalf("first run %q", lines[0])
	}
	if !strings.Contains(lines[1], `app-26.11\glow L!VE.exe|--restarted --hidden --launched --launcher=v26.12 --fellback=v26.12`) {
		t.Fatalf("fallback run %q", lines[1])
	}
	if !isBad(dir, "v26.12") || isBad(dir, "v26.11") {
		t.Fatal("bad marks wrong")
	}
}

// An update lands in a new folder and the app exits with restartHidden: the
// launcher starts the new folder, back in the tray.
func TestSuperviseStartsTheNewFolderAfterAnUpdate(t *testing.T) {
	out := setup(t, "app-26.11=3")
	t.Setenv("GLOW_INSTALL_FETCH", "v26.12") // what the 26.11 run "downloads"
	os.Args = []string{os.Args[0]}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.11"))

	if !supervise("v26.11") {
		t.Fatal("supervise gave up")
	}
	lines := runs(t, out, 2)
	if !strings.Contains(lines[0], `app-26.11\glow L!VE.exe|--launched --launcher=v26.11`) {
		t.Fatalf("first run %q", lines[0])
	}
	if !strings.Contains(lines[1], `app-26.12\glow L!VE.exe|--relaunch --hidden --launched --launcher=v26.11`) {
		t.Fatalf("after the update %q", lines[1])
	}
}

// The first start of a launcher from an install before release folders puts
// its own build in one and runs it from there.
func TestSuperviseMigratesAnOldInstall(t *testing.T) {
	out := setup(t, "")
	os.Args = []string{os.Args[0], "--relaunch", "--hidden"}
	installTestExe(t, Target())

	if !supervise("v26.11") {
		t.Fatal("supervise gave up")
	}
	lines := runs(t, out, 1)
	if !strings.Contains(lines[0], `app-26.11\glow L!VE.exe|--hidden --relaunch --launched --launcher=v26.11`) {
		t.Fatalf("run %q", lines[0])
	}
}

// An old download opened again starts the install and leaves the newer build
// alone; a newer download installs itself.
func TestInstallFromNeverGoesOverANewerInstall(t *testing.T) {
	out := setup(t, "")
	os.Args = []string{os.Args[0]}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.12"))
	before, _ := os.Stat(Target())

	self, _ := os.Executable()
	if !installFrom(self, "v26.10") {
		t.Fatal("did not hand off")
	}
	if exists(versionDir(dir, "v26.10")) {
		t.Fatal("an older download made a release folder")
	}
	if after, _ := os.Stat(Target()); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an older download replaced the launcher")
	}
	lines := runs(t, out, 1)
	if !strings.Contains(lines[0], `glow L!VE\glow L!VE.exe|--moved`) {
		t.Fatalf("handed off to %q", lines[0])
	}

	if !installFrom(self, "v26.13") || !exists(versionExe(dir, "v26.13")) {
		t.Fatal("a newer download did not install itself")
	}
}

// An install from before release folders (the launcher file is the old app,
// no app-* folder): a newer download puts itself in a folder AND over the
// launcher, so the launcher that starts is the new one.
func TestInstallFromReplacesAnOldStyleInstall(t *testing.T) {
	_ = setup(t, "")
	os.Args = []string{os.Args[0]}
	if err := os.MkdirAll(installDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Target(), []byte("old v26.10 app"), 0o755); err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	if !installFrom(self, "v26.11") {
		t.Fatal("did not hand off")
	}
	if b, _ := os.ReadFile(Target()); string(b) == "old v26.10 app" {
		t.Fatal("the old app is still the launcher")
	}
	if !exists(versionExe(installDir(), "v26.11")) {
		t.Fatal("no release folder")
	}
}

// A dev build still copies itself over the launcher and runs from there.
func TestRelocateDevBuild(t *testing.T) {
	out := setup(t, "")
	os.Args = []string{os.Args[0], "--hidden"}
	if Relocate("v26.11") {
		t.Fatal("a release build took the dev path")
	}
	if !Relocate("dev-abc1234") {
		t.Fatal("Relocate did not hand off")
	}
	lines := runs(t, out, 1)
	if !strings.HasPrefix(strings.ToLower(lines[0]), strings.ToLower(Target())+"|--relaunch --hidden") {
		t.Fatalf("child %q", lines[0])
	}
}

// A build that had been running fine and stops (a crash, or ended in Task
// Manager) is started once more, not over and over.
func TestSuperviseRevivesAHealthyBuildOnce(t *testing.T) {
	out := setup(t, "app-26.11=1")
	os.Args = []string{os.Args[0]}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.11"))
	mark(dir, "v26.11", healthyMark)

	if !supervise("v26.11") {
		t.Fatal("supervise gave up")
	}
	if lines := runs(t, out, 2); len(lines) != 2 || !strings.Contains(lines[1], "--restarted --hidden") {
		t.Fatalf("runs %q", lines)
	}
	if isBad(dir, "v26.11") {
		t.Fatal("a healthy build was marked bad")
	}
}

// The app ended because Windows is shutting down: no restart, no bad mark,
// even for a build that hasn't run a minute yet.
func TestSuperviseLeavesAShutdownAlone(t *testing.T) {
	out := setup(t, "app-26.12=1")
	os.Args = []string{os.Args[0]}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.11"))
	mark(dir, "v26.11", healthyMark)
	installTestExe(t, versionExe(dir, "v26.12"))
	NoteShutdown()

	if !supervise("v26.12") {
		t.Fatal("supervise gave up")
	}
	if lines := runs(t, out, 1); len(lines) != 1 {
		t.Fatalf("started again: %q", lines)
	}
	if isBad(dir, "v26.12") {
		t.Fatal("a shutdown marked the build bad")
	}
}

// The Start menu entry is made once and points at the launcher; one deleted
// afterwards stays deleted.
func TestShortcutOnce(t *testing.T) {
	_ = setup(t, "")
	installTestExe(t, Target())
	ensureShortcut()
	if !exists(shortcutPath()) {
		t.Fatal("no Start menu shortcut")
	}
	_ = os.Remove(shortcutPath())
	ensureShortcut()
	if exists(shortcutPath()) {
		t.Fatal("a deleted shortcut came back")
	}
}

// A build that keeps asking to be restarted (exit code 3) is not restarted
// forever: four restarts in ten minutes and the launcher stops.
func TestSuperviseStopsARestartLoop(t *testing.T) {
	out := setup(t, "app-26.11=3")
	os.Args = []string{os.Args[0]}
	dir := installDir()
	installTestExe(t, Target())
	installTestExe(t, versionExe(dir, "v26.11"))

	if !supervise("v26.11") {
		t.Fatal("supervise gave up")
	}
	if lines := runs(t, out, 4); len(lines) != 4 {
		t.Fatalf("ran %d times, want 4: %q", len(lines), lines)
	}
}
