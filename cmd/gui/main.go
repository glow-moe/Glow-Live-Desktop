// Command gui is the glow L!VE desktop app. It renders the embedded kawaii UI in
// a real native window (webview - WebView2 on Windows, WebKitGTK on Linux), no
// browser and no tabs, driving the same League + Forza collectors as the console.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/gui"
	"github.com/glow-moe/glow-collector/internal/single"
)

// version is the build label shown on the UI badge. Overridden at build time
// via -ldflags "-X main.version=v1.0" (see build-*.sh + the VERSION file).
var version = "dev"

// trayTerminate is the app's real exit, wired to the webview's Terminate; the
// tray menu's Quit calls it from whichever platform's tray is active.
var trayTerminate func()

func main() {
	runtime.LockOSThread() // the GUI must own the main OS thread

	// One copy per user. A second launch is treated as "bring it back": the
	// running copy is asked to show its window, and this one leaves.
	if !single.Acquire() {
		wakeRunningCopy()
		return
	}

	cfg, _ := config.Load()
	srv := gui.NewServer(cfg, version)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("glow L!VE: can't start local server:", err)
		return
	}
	go func() { _ = http.Serve(ln, srv.Handler()) }()
	url := "http://" + ln.Addr().String()
	writeAddrFile(url)

	const winW, winH = 340, 440 // small desktop-widget size

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("glow L!VE")
	w.SetSize(winW, winH, webview.HintFixed)
	w.Navigate(url)
	trayTerminate = w.Terminate
	// Park it in the bottom-right corner once the GTK loop is up.
	w.Dispatch(func() {
		moveBottomRight(w.Window(), winW, winH)
		enableCloseToTray(w.Window()) // close button hides to tray
		if cfg.StartHidden {
			hideToTray(w.Window())
		}
	})
	// Auto-tuck into the tray once the collector starts pushing. The server fires
	// this (from an HTTP handler goroutine); marshal it onto the GUI thread.
	srv.SetHideToTray(func() { w.Dispatch(func() { hideToTray(w.Window()) }) })
	srv.SetShowWindow(func() { w.Dispatch(func() { showWindow(w.Window()) }) })
	w.Run()
}

// addrFilePath is where the running copy leaves its local GUI address.
func addrFilePath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "glow-collector", "gui.addr")
}

func writeAddrFile(url string) {
	if p := addrFilePath(); p != "" {
		_ = os.WriteFile(p, []byte(url), 0o600)
	}
}

// wakeRunningCopy asks the copy that holds the lock to show its window, so a
// double-click on the launcher behaves like "open it", not like an error.
func wakeRunningCopy() {
	p := addrFilePath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	c := &http.Client{Timeout: 2 * time.Second}
	_, _ = c.Post(strings.TrimSpace(string(b))+"/api/show", "", nil)
}
