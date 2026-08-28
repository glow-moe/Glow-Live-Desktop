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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/autostart"
	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/orchestrator"
	"github.com/glow-moe/glow-collector/internal/pair"
	"github.com/glow-moe/glow-collector/internal/poster"
	"github.com/glow-moe/glow-collector/internal/update"
)

//go:embed web
var webFS embed.FS

// Server bundles the orchestrator + config behind an HTTP API for the frontend.
type Server struct {
	mu         sync.Mutex
	cfg        config.Config
	orch       *orchestrator.Orchestrator
	username   string
	avatar     string
	version    string
	linking    bool
	refreshing bool   // a whoami refresh is in flight (avatar/username recovery)
	hideToTray func() // set by main: tuck the window into the tray
	showWindow func() // set by main: bring the window back on screen
	updateVer  string // latest release when this build is behind it, else ""
}

// SetShowWindow registers the callback a second launch (or the tray) uses to
// bring the window back on screen.
func (s *Server) SetShowWindow(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.showWindow = fn
}

// SetHideToTray registers the callback that parks the window in the system tray
// (wired by main to the native window). Called once the collector starts pushing
// so the widget auto-tucks away instead of sitting open on screen.
func (s *Server) SetHideToTray(fn func()) {
	s.mu.Lock()
	s.hideToTray = fn
	s.mu.Unlock()
}

// refreshIdentity re-fetches the profile name + avatar when they're missing
// (e.g. the startup whoami raced a cold network). Runs at most once at a time.
func (s *Server) refreshIdentity() {
	s.mu.Lock()
	if s.refreshing || s.cfg.Token == "" {
		s.mu.Unlock()
		return
	}
	s.refreshing = true
	endpoint, tok := s.cfg.Endpoint, s.cfg.Token
	s.mu.Unlock()

	name, av, uid := whoami(endpoint, tok)

	s.mu.Lock()
	if name != "" {
		s.username = name
	}
	if av != "" {
		s.avatar = av
	}
	s.refreshing = false
	s.mu.Unlock()
	if name != "" {
		s.orch.SetUsername(name)
	}
	if uid != "" {
		s.orch.SetUserID(uid)
	}
}

// NewServer wires the server to the saved config.
func NewServer(cfg config.Config, version string) *Server {
	// Report this build to the server on every push so it can turn away versions
	// that are too old (forced update).
	poster.SetVersion(version)
	s := &Server{cfg: cfg, version: version, orch: orchestrator.New(cfg)}
	s.orch.OnSeenGame(s.recordSeenGame)
	go s.watchUpdates()
	if cfg.Token != "" {
		var uid string
		s.username, s.avatar, uid = whoami(cfg.Endpoint, cfg.Token)
		s.orch.SetUsername(s.username)
		s.orch.SetUserID(uid)
		s.orch.Start() // already linked → auto-start collecting
	}
	return s
}

// watchUpdates checks for a newer release on startup and every few hours after,
// from the user's own machine. A dev build never reports an update.
func (s *Server) watchUpdates() {
	check := func() {
		if v, ok := update.Check(s.version); ok {
			s.mu.Lock()
			s.updateVer = v
			s.mu.Unlock()
		}
	}
	check()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for range t.C {
		check()
	}
}

// hOpenDownload opens the releases page in the user's browser.
func (s *Server) hOpenDownload(w http.ResponseWriter, _ *http.Request) {
	openURL(update.ReleasesPage)
	writeJSON(w, map[string]any{"ok": true})
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
	mux.HandleFunc("/api/minimize", s.hMinimize)
	mux.HandleFunc("/api/show", s.hShow)
	mux.HandleFunc("/api/open-download", s.hOpenDownload)
	// Local OBS overlay data: the overlay page (below, or the glow.moe page with
	// ?src=local) reads the current masked League snapshot from here instead of
	// polling glow.moe, so the ~2s live poll never touches our server.
	mux.HandleFunc("/live.json", s.hLive)
	// Self-contained OBS overlay page. Point an OBS Browser Source at
	// http://127.0.0.1:47100/overlay: it renders the League card entirely from
	// /live.json on this loopback server, so nothing touches glow.moe (works
	// offline). Rank crests + fonts are bundled; ddragon art is Riot's CDN.
	mux.HandleFunc("/overlay", s.hOverlay)
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

// hLive serves the current masked League snapshot ({live,snapshot}) for a
// localhost OBS overlay. CORS-open so the glow.moe overlay page (?src=local) can
// read it cross-origin; never cached so OBS always gets the latest frame.
func (s *Server) hLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = w.Write(s.orch.OverlayJSON())
}

// hOverlay serves the self-contained OBS overlay page (web/overlay.html): a
// transparent card that polls /live.json on this same localhost server and
// renders the League snapshot entirely offline (no glow.moe). Served from an
// explicit route so the OBS URL is a clean /overlay with no trailing slash.
func (s *Server) hOverlay(w http.ResponseWriter, _ *http.Request) {
	b, err := webFS.ReadFile("web/overlay.html")
	if err != nil {
		http.Error(w, "overlay not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
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
	// The CDN transcodes avatars and picks the format from Accept, so ask for one
	// the webview can paint: an AVIF reply renders as nothing and the picture
	// silently falls back to the initials.
	//
	// Asking is not enough on its own. A cached AVIF copy from before this header
	// existed is served straight back on a HIT, Accept and all, so the URL is
	// varied per launch to force the edge to negotiate again. That costs one
	// origin fetch each time the app starts, which is what it does anyway.
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	req, err := http.NewRequest(http.MethodGet, url+sep+"fresh="+strconv.FormatInt(time.Now().UnixNano(), 36), nil)
	if err != nil {
		http.Error(w, "bad avatar url", http.StatusBadGateway)
		return
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
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
	needIdentity := linked && (s.username == "" || s.avatar == "")
	out := map[string]any{
		"version":  s.version,
		"linked":   linked,
		"username": s.username,
		"avatar":   s.avatar,
		"linking":  s.linking,
		"running":  s.orch.Running(),
		"status":   s.orch.Status(),
		"update":   map[string]any{"version": s.updateVer, "url": update.ReleasesPage},
	}
	s.mu.Unlock()
	// Recover the name/avatar if the startup whoami came back empty (cold network).
	if needIdentity {
		go s.refreshIdentity()
	}
	writeJSON(w, out)
}

func (s *Server) hConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct {
			DelaySec      *int    `json:"delaySec"`
			PollMs        *int    `json:"pollMs"`
			Endpoint      *string `json:"endpoint"`
			AnimePresence *bool   `json:"animePresence"`
			SteamPresence *bool   `json:"steamPresence"`
			AutoStart     *bool   `json:"autoStart"`
			StartHidden   *bool   `json:"startHidden"`
			HideOnGame    *bool   `json:"hideOnGame"`
			HideGame      *struct {
				AppID  int  `json:"appId"`
				Hidden bool `json:"hidden"`
			} `json:"hideGame"`
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
		if body.AnimePresence != nil {
			s.cfg.AnimePresence = *body.AnimePresence
		}
		if body.SteamPresence != nil {
			s.cfg.SteamPresence = *body.SteamPresence
		}
		if body.AutoStart != nil {
			// The OS artifact is the real switch; the flag only survives when
			// the artifact could actually be written.
			if err := autostart.Set(*body.AutoStart); err == nil {
				s.cfg.AutoStart = *body.AutoStart
			}
		}
		if body.StartHidden != nil {
			s.cfg.StartHidden = *body.StartHidden
		}
		if body.HideOnGame != nil {
			s.cfg.HideOnGame = *body.HideOnGame
		}
		// Per-game hide: one appid on/off. Purely local, never sent to glow.
		if body.HideGame != nil {
			s.setGameHiddenLocked(body.HideGame.AppID, body.HideGame.Hidden)
		}
		s.cfg.Normalize()
		cfg := s.cfg
		s.mu.Unlock()
		_ = config.Save(cfg)
		s.orch.SetConfig(cfg)
	}
	s.mu.Lock()
	out := map[string]any{
		"delaySec":      s.cfg.DelaySec,
		"pollMs":        s.cfg.PollMs,
		"endpoint":      s.cfg.Endpoint,
		"animePresence": s.cfg.AnimePresence,
		"steamPresence": s.cfg.SteamPresence,
		"autoStart":     s.cfg.AutoStart,
		"startHidden":   s.cfg.StartHidden,
		"hideOnGame":    s.cfg.HideOnGame,
		"seenGames":     s.cfg.SeenGames,
		"hiddenGames":   s.cfg.HiddenGames,
	}
	s.mu.Unlock()
	writeJSON(w, out)
}

// setGameHiddenLocked adds or removes an appid from HiddenGames. Caller holds
// s.mu.
func (s *Server) setGameHiddenLocked(appID int, hidden bool) {
	if appID <= 0 {
		return
	}
	out := s.cfg.HiddenGames[:0]
	for _, id := range s.cfg.HiddenGames {
		if id != appID {
			out = append(out, id)
		}
	}
	if hidden {
		out = append(out, appID)
	}
	s.cfg.HiddenGames = out
}

// recordSeenGame remembers a Steam game the collector saw, so the settings list
// can offer a toggle for it. Deduped, saved only when new. Wired as the
// orchestrator's OnSeenGame callback.
func (s *Server) recordSeenGame(appID int, name string) {
	if appID <= 0 || name == "" {
		return
	}
	s.mu.Lock()
	for _, g := range s.cfg.SeenGames {
		if g.AppID == appID {
			s.mu.Unlock()
			return // already known
		}
	}
	s.cfg.SeenGames = append(s.cfg.SeenGames, config.SeenGame{AppID: appID, Name: name})
	cfg := s.cfg
	s.mu.Unlock()
	_ = config.Save(cfg)
	s.orch.SetConfig(cfg)
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
				name, av, uid := whoami(cfg.Endpoint, tok)
				s.mu.Lock()
				s.username = name
				s.avatar = av
				s.orch.SetUsername(name)
				s.orch.SetUserID(uid)
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
	name, av, uid := whoami(cfg.Endpoint, tok)
	s.mu.Lock()
	s.username = name
	s.avatar = av
	s.orch.SetUsername(name)
	s.orch.SetUserID(uid)
	s.orch.Start()
	s.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "username": name})
}

// hMinimize tucks the window into the system tray. The frontend calls it once,
// the moment the collector starts delivering (a game/anime is live), so the app
// doesn't stay open on screen. Honors the hideOnGame setting, and no-ops when
// the native hook isn't wired.
func (s *Server) hMinimize(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	fn := s.hideToTray
	allowed := s.cfg.HideOnGame
	s.mu.Unlock()
	if fn != nil && allowed {
		fn()
	}
	writeJSON(w, map[string]any{"ok": true})
}

// hShow brings the window back; a duplicate launch posts here and exits.
func (s *Server) hShow(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	fn := s.showWindow
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
	writeJSON(w, map[string]any{"ok": true})
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
func whoami(endpoint, token string) (username, avatar, id string) {
	if token == "" {
		return "", "", ""
	}
	// /api/live/me returns the profile id + avatar (or the account photo) - the
	// same endpoint the browser extension uses, so a user with no uploaded avatar
	// still gets their OAuth photo. The id lets us read our own anime snapshot.
	req, err := http.NewRequest(http.MethodGet, pair.BaseFrom(endpoint)+"/api/live/me", nil)
	if err != nil {
		return "", "", ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return "", "", ""
	}
	defer resp.Body.Close()
	var out struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		AvatarURL string `json:"avatarUrl"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Username, out.AvatarURL, out.ID
}
