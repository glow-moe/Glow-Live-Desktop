package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportHasTheLogWithoutTheAccountName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(home, "cfg"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	downloads = func() string { return "" } // not the real Downloads folder
	defer func() { downloads = downloadsDir }()
	Enable()
	Printf(`open C:\Users\sam\AppData\x.json: denied`)
	Printf("league in game")
	path, err := Export("glow L!VE v26.11\n")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(home, "Downloads") {
		t.Fatalf("saved to %s", path)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	if !strings.HasPrefix(out, "glow L!VE v26.11") || !strings.Contains(out, "[app] league in game") {
		t.Fatalf("export:\n%s", out)
	}
	if strings.Contains(out, "sam") || !strings.Contains(out, `C:\Users\…\AppData`) {
		t.Fatalf("account name not scrubbed:\n%s", out)
	}
}
