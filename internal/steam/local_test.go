package steam

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunningAppIDFromVDF(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "registry.vdf")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	reg := func(id string) string {
		return "\"Registry\"\n{\n\t\"HKCU\"\n\t{\n\t\t\"Software\"\n\t\t{\n\t\t\t\"Valve\"\n\t\t\t{\n\t\t\t\t\"Steam\"\n\t\t\t\t{\n\t\t\t\t\t\"AutoLoginUser\"\t\t\"someone\"\n\t\t\t\t\t\"RunningAppID\"\t\t\"" + id + "\"\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n}\n"
	}

	if id, ok := runningAppIDFromVDF(write(reg("570"))); !ok || id != 570 {
		t.Fatalf("running Dota: got %d %v", id, ok)
	}
	if id, ok := runningAppIDFromVDF(write(reg("0"))); !ok || id != 0 {
		t.Fatalf("nothing running: got %d %v", id, ok)
	}
	if _, ok := runningAppIDFromVDF(write("\"Registry\"\n{\n}\n")); ok {
		t.Fatal("missing key must be unknown")
	}
	if _, ok := runningAppIDFromVDF(filepath.Join(dir, "nope.vdf")); ok {
		t.Fatal("missing file must be unknown")
	}
}
