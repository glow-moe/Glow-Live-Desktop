//go:build linux

package main

import "testing"

// trayIconPNG is embedded by tray_linux.go, so this test only exists on Linux;
// the platform-neutral tray line test lives in tray_status_test.go.
func TestAlertIconPNG(t *testing.T) {
	out, err := alertIconPNG(trayIconPNG)
	if err != nil || len(out) == 0 {
		t.Fatalf("alert icon: %v (%d bytes)", err, len(out))
	}
}
