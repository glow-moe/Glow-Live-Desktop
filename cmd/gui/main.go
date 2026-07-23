// Command gui is the glow L!VE desktop app. It renders the embedded kawaii UI in
// a real native window (webview - WebView2 on Windows, WebKitGTK on Linux), no
// browser and no tabs, driving the same League + Forza collectors as the console.
package main

import (
	"fmt"
	"net"
	"net/http"
	"runtime"

	webview "github.com/webview/webview_go"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/gui"
)

// version is the build label shown on the UI badge. Overridden at build time
// via -ldflags "-X main.version=v1.0" (see build-*.sh + the VERSION file).
var version = "dev"

func main() {
	runtime.LockOSThread() // the GUI must own the main OS thread

	cfg, _ := config.Load()
	srv := gui.NewServer(cfg, version)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("glow L!VE: can't start local server:", err)
		return
	}
	go func() { _ = http.Serve(ln, srv.Handler()) }()
	url := "http://" + ln.Addr().String()

	const winW, winH = 340, 440 // small desktop-widget size

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("glow L!VE")
	w.SetSize(winW, winH, webview.HintFixed)
	w.Navigate(url)
	// Park it in the bottom-right corner once the GTK loop is up.
	w.Dispatch(func() {
		moveBottomRight(w.Window(), winW, winH)
		enableCloseToTray(w.Window()) // Windows: close button hides to tray
	})
	// Auto-tuck into the tray once the collector starts pushing. The server fires
	// this (from an HTTP handler goroutine); marshal it onto the GUI thread.
	srv.SetHideToTray(func() { w.Dispatch(func() { hideToTray(w.Window()) }) })
	w.Run()
}
