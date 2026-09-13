package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/gui"
)

// The tray shows a one-line status (menu + tooltip) and flips to the alert
// icon when the site is not receiving anything: the build was turned away
// (426), the last push failed, or nothing has landed for a while.

var (
	trayMu      sync.Mutex
	trayProfile string // URL the "Open my profile" item opens
)

// trayLine renders the status line and whether the icon should alert.
func trayLine(info gui.TrayInfo, now time.Time) (line string, alert bool) {
	st := info.Status
	switch {
	case !info.Linked:
		return "Not linked", false
	case st.Outdated:
		return "Update required", true
	case !info.Running:
		return "Stopped", false
	case st.Err != "":
		return "Problem: " + trim(st.Err, 60), true
	}
	if st.LastPushAt > 0 {
		ago := now.Sub(time.UnixMilli(st.LastPushAt))
		if st.InGame && ago > 90*time.Second {
			return "Not reaching glow.moe (" + since(ago) + ")", true
		}
		if st.InGame {
			return "Live · pushed " + since(ago), false
		}
		return "Watching · last push " + since(ago), false
	}
	if st.InGame {
		return "Live · sending…", false
	}
	return "Watching for a game", false
}

func since(d time.Duration) string {
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// watchTray refreshes the tray every few seconds from the server's view of
// things. `apply` runs on the GUI thread (webview Dispatch) because both trays
// are GUI-thread objects.
func watchTray(srv *gui.Server, apply func(fn func())) {
	go func() {
		var lastLine string
		var lastAlert bool
		first := true
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for range t.C {
			info := srv.TrayInfo()
			trayMu.Lock()
			trayProfile = info.ProfileURL
			trayMu.Unlock()
			line, alert := trayLine(info, time.Now())
			if !first && line == lastLine && alert == lastAlert {
				continue
			}
			first, lastLine, lastAlert = false, line, alert
			apply(func() { trayUpdate(line, alert, info.ProfileURL != "") })
		}
	}()
}

// openProfile opens the linked profile in the default browser (tray item).
func openProfile() {
	trayMu.Lock()
	u := trayProfile
	trayMu.Unlock()
	if u == "" {
		return
	}
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		_ = exec.Command("open", u).Start()
	default:
		_ = exec.Command("xdg-open", u).Start()
	}
}
