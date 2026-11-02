//go:build linux

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The install goes under XDG_DATA_HOME, else ~/.local/share.
func TestTargetFollowsXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	if got := Target(); got != "/tmp/xdg-data/glow-live/glow-live" {
		t.Fatalf("Target = %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/sam")
	if got := Target(); got != "/home/sam/.local/share/glow-live/glow-live" {
		t.Fatalf("Target = %q", got)
	}
}

// A downloaded copy installs itself executable and hands off to the launcher.
func TestInstallFromMakesItRunnable(t *testing.T) {
	out := setup(t, "")
	os.Args = []string{os.Args[0], "--hidden"}
	self, _ := os.Executable()
	if !installFrom(self, "v26.11") {
		t.Fatal("did not hand off")
	}
	for _, p := range []string{Target(), versionExe(installDir(), "v26.11")} {
		fi, err := os.Stat(p)
		if err != nil || fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s not executable: %v %v", p, fi, err)
		}
	}
	if lines := runs(t, out, 1); !strings.Contains(lines[0], Target()+"|--moved --hidden") {
		t.Fatalf("handed off to %q", lines[0])
	}
}

// The app menu entry is made once, points at the launcher and offers
// Uninstall; one deleted afterwards stays deleted.
func TestMenuEntryOnce(t *testing.T) {
	_ = setup(t, "")
	installTestExe(t, Target())
	ensureShortcut()
	b, err := os.ReadFile(menuPath())
	if err != nil {
		t.Fatal("no menu entry")
	}
	for _, want := range []string{"Name=glow L!VE", `Exec="` + Target() + `"`, `Exec="` + Target() + `" --uninstall`, "Icon=" + iconPath()} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("menu entry has no %q:\n%s", want, b)
		}
	}
	_ = os.Remove(menuPath())
	ensureShortcut()
	if exists(menuPath()) {
		t.Fatal("a deleted menu entry came back")
	}
}

// Uninstall leaves nothing of the app behind but the download.
func TestUninstallRemovesEverything(t *testing.T) {
	_ = setup(t, "")
	installTestExe(t, Target())
	installTestExe(t, versionExe(installDir(), "v26.11"))
	ensureShortcut()
	cfg, _ := os.UserConfigDir()
	auto := filepath.Join(cfg, "autostart", "glow-live.desktop")
	settings := filepath.Join(cfg, "glow-collector", "config.json")
	for _, p := range []string{auto, settings} {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}

	Uninstall(true)
	for _, p := range []string{installDir(), menuPath(), auto, filepath.Dir(settings)} {
		if exists(p) {
			t.Fatalf("%s is still there", p)
		}
	}
}
