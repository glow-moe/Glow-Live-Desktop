package install

import (
	"os"
	"path/filepath"
	"testing"
)

func put(t *testing.T, dir, ver string, marks ...string) {
	t.Helper()
	if err := os.MkdirAll(versionDir(dir, ver), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionExe(dir, ver), []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, m := range marks {
		mark(dir, ver, m)
	}
}

func TestPickNewestNotBad(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "v26.9.1", healthyMark)
	put(t, dir, "v26.11", healthyMark)
	put(t, dir, "v26.12", badMark)
	// A folder whose exe is gone (an antivirus took it) doesn't count.
	if err := os.MkdirAll(versionDir(dir, "v26.13"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := pick(dir); got != "v26.11" {
		t.Fatalf("pick = %q, want v26.11", got)
	}
	if got := versions(dir); len(got) != 3 || got[0] != "v26.12" || got[2] != "v26.9.1" {
		t.Fatalf("versions = %v", got)
	}
}

func TestPruneKeepsRunningPreviousAndNewerBad(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "v26.9", healthyMark)
	put(t, dir, "v26.10", healthyMark)
	put(t, dir, "v26.11")
	put(t, dir, "v26.12", badMark)
	prune(dir, "v26.11")
	for ver, want := range map[string]bool{"v26.9": false, "v26.10": true, "v26.11": true, "v26.12": true} {
		if got := exists(versionDir(dir, ver)); got != want {
			t.Errorf("%s kept = %v, want %v", ver, got, want)
		}
	}
}

func TestReplaceMovesARunningFileAside(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "a.exe")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "b.exe")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyExe(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "new" {
		t.Fatalf("dst = %q", b)
	}
}
