package gui

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/pair"
)

// The OBS overlay page comes from glow.moe (public/live/overlay-app.html there),
// so its look ships with a site update; the copy built into the app is the
// fallback when glow.moe can't be reached. Either way it is served from this
// loopback server and reads /live.json here, so OBS never polls glow.moe.
//
// It runs sandboxed (an opaque origin): it can read /live.json (CORS-open) and
// load images, but can't call this app's /api.

const overlayMarker = "glow-live-overlay"

var overlayCache struct {
	sync.Mutex
	page []byte
	at   time.Time
	try  time.Time
}

// remoteOverlay returns glow.moe's overlay page, re-fetched at most every 10
// minutes (a failed fetch is retried after a minute). nil = use the built-in one.
func remoteOverlay(endpoint string) []byte {
	c := &overlayCache
	c.Lock()
	defer c.Unlock()
	if c.page != nil && time.Since(c.at) < 10*time.Minute {
		return c.page
	}
	if time.Since(c.try) < time.Minute {
		return c.page
	}
	c.try = time.Now()
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(pair.BaseFrom(endpoint) + "/live/overlay-app.html")
	if err != nil {
		return c.page
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10+1))
	if err != nil || resp.StatusCode != http.StatusOK || len(b) > 512<<10 || !bytes.Contains(b, []byte(overlayMarker)) {
		return c.page
	}
	c.page, c.at = b, time.Now()
	return c.page
}

// localOnly turns away anything that isn't this app's own window: a Host that
// isn't loopback (DNS rebinding), and /api calls from another origin (a web
// page, or the sandboxed overlay) - /api can link, unlink and stop the app.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if s := r.Header.Get("Sec-Fetch-Site"); s == "cross-site" || s == "same-site" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
