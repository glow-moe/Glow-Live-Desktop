//go:build windows || linux

package install

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// quitRunningCopy asks the app that's running (if any) to quit, through the
// local address it leaves in its config folder, and waits for it to go.
func quitRunningCopy() {
	base, err := os.UserConfigDir()
	if err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(base, "glow-collector", "gui.addr"))
	if err != nil {
		return
	}
	addr := strings.TrimSpace(string(b))
	c := &http.Client{Timeout: 2 * time.Second}
	if resp, err := c.Post(addr+"/api/quit", "application/json", strings.NewReader("{}")); err == nil {
		resp.Body.Close()
	} else {
		return // nothing answering: not running
	}
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		resp, err := c.Get(addr + "/api/status")
		if err != nil {
			break
		}
		resp.Body.Close()
	}
	// The launcher ends right after the app does.
	time.Sleep(1500 * time.Millisecond)
}
