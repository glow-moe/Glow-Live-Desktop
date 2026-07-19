// Package gui serves the L!VE desktop UI (the embedded kawaii web app) over a
// loopback HTTP server and exposes a small JSON API the frontend drives:
// status, config, link (paste key or browser authorize), start/stop.
package gui

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/orchestrator"
	"github.com/glow-moe/glow-collector/internal/pair"
)

//go:embed web
var webFS embed.FS

// Server bundles the orchestrator + config behind an HTTP API for the frontend.
type Server struct {
	mu       sync.Mutex
	cfg      config.Config
	orch     *orchestrator.Orchestrator
	username string
	avatar   string
	version  string
	linking  bool
}

// NewServer wires the server to the saved config.
func NewServer(cfg config.Config, version string) *Server {
	s := &Server{cfg: cfg, version: version, orch: orchestrator.New(cfg)}
	if cfg.Token != "" {
		s.username, s.avatar = whoami(cfg.Endpoint, cfg.Token)
		s.orch.SetUsername(s.username)
		s.orch.Start() // already linked → auto-start collecting
	}
	return s
}

// Handler returns the mux serving the UI + API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/status", s.hStatus)
	mux.HandleFunc("/api/config", s.hConfig)
	mux.HandleFunc("/api/link", s.hLink)
	mux.HandleFunc("/api/start", s.hStart)
	mux.HandleFunc("/api/stop", s.hStop)
	mux.HandleFunc("/api/avatar", s.hAvatar)
	mux.HandleFunc("/api/open-settings", s.hOpenSettings)
	return mux
}

// hOpenSettings opens the site's L!VE settings (glow.moe/dashboard/live) in the
// user's default browser - the settings live there, not in this app.
func (s *Server) hOpenSettings(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	base := pair.BaseFrom(s.cfg.Endpoint)
	s.mu.Unlock()
	openURL(base + "/dashboard/live")
	writeJSON(w, map[string]any{"ok": true})
}

func openURL(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", "", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

// hAvatar proxies the user's profile photo through Go so the webview never has
// to reach a third-party CDN itself (avoids odd webview/CDN/network failures).
func (s *Server) hAvatar(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	url := s.avatar
	s.mu.Unlock()
	if url == "" {
		http.Error(w, "no avatar", http.StatusNotFound)
		return
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		http.Error(w, "fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "upstream "+resp.Status, http.StatusBadGateway)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) hStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	linked := s.cfg.Token != ""
	out := map[string]any{
		"version":  s.version,
		"linked":   linked,
		"username": s.username,
		"avatar":   s.avatar,
		"linking":  s.linking,
		"running":  s.orch.Running(),
		"status":   s.orch.Status(),
	}
	s.mu.Unlock()
	writeJSON(w, out)
}

func (s *Server) hConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct {
			DelaySec *int    `json:"delaySec"`
			PollMs   *int    `json:"pollMs"`
			Endpoint *string `json:"endpoint"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		if body.DelaySec != nil {
			s.cfg.DelaySec = *body.DelaySec
		}
		if body.PollMs != nil {
			s.cfg.PollMs = *body.PollMs
		}
		if body.Endpoint != nil && *body.Endpoint != "" {
			s.cfg.Endpoint = *body.Endpoint
		}
		s.cfg.Normalize()
		cfg := s.cfg
		s.mu.Unlock()
		_ = config.Save(cfg)
		s.orch.SetConfig(cfg)
	}
	s.mu.Lock()
	out := map[string]any{
		"delaySec": s.cfg.DelaySec,
		"pollMs":   s.cfg.PollMs,
		"endpoint": s.cfg.Endpoint,
	}
	s.mu.Unlock()
	writeJSON(w, out)
}

func (s *Server) hLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token   string `json:"token"`
		Browser bool   `json:"browser"`
		Unlink  bool   `json:"unlink"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// Unlink: forget the token + stop pushing.
	if body.Unlink {
		s.orch.Stop()
		s.mu.Lock()
		s.cfg.Token = ""
		s.username = ""
		s.avatar = ""
		cfg := s.cfg
		s.mu.Unlock()
		_ = config.Save(cfg)
		s.orch.SetConfig(cfg)
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	// Browser authorize flow: open glow.moe, poll for the token in the background.
	if body.Browser {
		s.mu.Lock()
		if s.linking {
			s.mu.Unlock()
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		s.linking = true
		base := pair.BaseFrom(s.cfg.Endpoint)
		s.mu.Unlock()
		go func() {
			tok, err := pair.Run(base)
			s.mu.Lock()
			s.linking = false
			if err == nil && tok != "" {
				s.cfg.Token = tok
				cfg := s.cfg
				s.mu.Unlock()
				_ = config.Save(cfg)
				s.orch.SetConfig(cfg)
				name, av := whoami(cfg.Endpoint, tok)
				s.mu.Lock()
				s.username = name
				s.avatar = av
				s.orch.SetUsername(name)
				s.orch.Start() // auto-start after linking
			}
			s.mu.Unlock()
		}()
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	// Paste-key flow.
	tok := trimToken(body.Token)
	if tok == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "empty key"})
		return
	}
	s.mu.Lock()
	s.cfg.Token = tok
	cfg := s.cfg
	s.mu.Unlock()
	_ = config.Save(cfg)
	s.orch.SetConfig(cfg)
	name, av := whoami(cfg.Endpoint, tok)
	s.mu.Lock()
	s.username = name
	s.avatar = av
	s.orch.SetUsername(name)
	s.orch.Start()
	s.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "username": name})
}

func (s *Server) hStart(w http.ResponseWriter, _ *http.Request) {
	s.orch.Start()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) hStop(w http.ResponseWriter, _ *http.Request) {
	s.orch.Stop()
	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func trimToken(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\r' && r != '\t' {
			out = append(out, r)
		}
	}
	return string(out)
}

// whoami resolves the linked profile's username + avatar (Bearer /api/live/whoami).
func whoami(endpoint, token string) (string, string) {
	if token == "" {
		return "", ""
	}
	// /api/live/me returns the profile avatar (or the account photo) - the same
	// endpoint the browser extension uses, so a user with no uploaded avatar
	// still gets their OAuth photo.
	req, err := http.NewRequest(http.MethodGet, pair.BaseFrom(endpoint)+"/api/live/me", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	var out struct {
		Username  string `json:"username"`
		AvatarURL string `json:"avatarUrl"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Username, out.AvatarURL
}
