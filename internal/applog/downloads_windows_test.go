//go:build windows

package applog

import (
	"os"
	"testing"
)

func TestDownloadsDirIsAFolder(t *testing.T) {
	d := downloadsDir()
	if d == "" {
		t.Skip("no Downloads folder in the registry here")
	}
	if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
		t.Fatalf("downloadsDir = %q", d)
	}
	t.Logf("Downloads is %s", d)
}
