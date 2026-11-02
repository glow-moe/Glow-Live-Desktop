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

	"github.com/glow-moe/glow-collector/internal/applog"
	"github.com/glow-moe/glow-collector/internal/autostart"
	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/install"
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
	refreshing bool      // a whoami refresh is in flight (avatar/username recovery)
	hideToTray func()    // set by main: tuck the window into the tray
	showWindow func()    // set by main: bring the window back on screen
	updateVer  string    // latest release when this build is behind it, else ""
	updateAt   time.Time // when updateVer was published
	autoFailed string    // a release the automatic update couldn't install this session
	// peekWindow shows the hidden window without taking focus (main wires it;
	// false when the user is busy: full screen, presenting, quiet hours).
	peekWindow  func() bool
	prompted    string    // the update prompt already shown this session ("tag|forced")
	peeked      bool      // the window is on screen only because of that prompt
	userID      string    // the linked account's id (from /api/live/me)
	identityAt  time.Time // last whoami retry, so a missing username isn't asked every second
	linkCode    string    // the code to type on the glow.moe page while linking
	quit        func()    // the app's real exit (set by main)
	wasUnlinked bool      // glow.moe turned the key down: the link screen says so
	linkPrompt  bool      // that link screen still has to be brought up (the user was busy)
	// whatsNew is the release this build is, with its notes, shown once after
	// an update until closed.
	whatsNew      string
	whatsNewNotes []string
}

// SetPeekWindow registers the no-focus show the update prompt uses.
func (s *Server) SetPeekWindow(fn func() bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peekWindow = fn
}

// SetShowWindow registers the callback a second launch (or the tray) uses to
// bring the window back on screen.
func (s *Server) SetShowWindow(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.showWindow = fn
}

// TrayInfo is what the tray menu and tooltip need each refresh.
type TrayInfo struct {
	Running    bool
	Linked     bool
	Username   string
	ProfileURL string
	UpdateVer  string // newer release tag when this build is behind, else ""
	Status     orchestrator.Status
}

// TrayInfo returns the current state for the tray (menu status line, tooltip,
// alert icon, "open my profile").
func (s *Server) TrayInfo() TrayInfo {
	s.mu.Lock()
	linked := s.cfg.Token != ""
	name := s.username
	base := pair.BaseFrom(s.cfg.Endpoint)
	upd := s.updateVer
	s.mu.Unlock()
	url := ""
	if name != "" {
		url = base + "/" + name
	}
	return TrayInfo{Running: s.orch.Running(), Linked: linked, Username: name, ProfileURL: url, UpdateVer: upd, Status: s.orch.Status()}
}

// HideOnDiscord, HideOnDiscordForGame and ShowOnDiscord are the tray menu's
// Discord items: the profile keeps updating, only the Rich Presence goes away.
func (s *Server) HideOnDiscord(d time.Duration) { s.orch.HideOnDiscord(d) }

// HideOnDiscordForGame hides until the running game ends; false with no game.
func (s *Server) HideOnDiscordForGame() bool { return s.orch.HideOnDiscordForGame() }

// ShowOnDiscord ends a hide early.
func (s *Server) ShowOnDiscord() { s.orch.ShowOnDiscord() }

// SetQuit registers the app's real exit (main wires it), for the uninstaller
// asking the running copy to close.
func (s *Server) SetQuit(fn func()) {
	s.mu.Lock()
	s.quit = fn
	s.mu.Unlock()
}

func (s *Server) hQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "post", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	fn := s.quit
	s.mu.Unlock()
	writeJSON(w, map[string]any{"ok": fn != nil})
	if fn != nil {
		go fn()
	}
}

// HideOnProfile keeps the game off glow.moe for d (the tray item and the
// window's link); Discord isn't touched.
func (s *Server) HideOnProfile(d time.Duration) { s.orch.HideOnProfile(d) }

// ShowOnProfile ends a profile hide early.
func (s *Server) ShowOnProfile() { s.orch.ShowOnProfile() }

func (s *Server) hProfileHide(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hide bool `json:"hide"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Hide {
		s.HideOnProfile(time.Hour)
	} else {
		s.ShowOnProfile()
	}
	writeJSON(w, map[string]any{"ok": true})
}

// SetHideToTray registers the callback that parks the window in the system tray
// (wired by main to the native window). Called once the collector starts pushing
// so the widget auto-tucks away instead of sitting open on screen.
func (s *Server) SetHideToTray(fn func()) {
	s.mu.Lock()
	s.hideToTray = fn
	s.mu.Unlock()
}

// ReportLifecycle tells glow.moe this app started or quit and how ("how=boot
// autostart=on installed=yes"), so God Mode can tell a member who closed the
// app from one whose app vanished (an antivirus, a crash). Nothing to report
// when unlinked. A start retries a network that isn't up yet at boot; a quit
// gets one try.
func (s *Server) ReportLifecycle(kind, detail string) {
	s.mu.Lock()
	base, tok := pair.BaseFrom(s.cfg.Endpoint), s.cfg.Token
	s.mu.Unlock()
	if tok == "" {
		return
	}
	tries := 1
	if kind == "start" {
		tries = 3
	}
	for i := 0; i < tries; i++ {
		if poster.Report(base, tok, kind, detail, "", runtime.GOOS) == nil {
			return
		}
		if i < tries-1 {
			time.Sleep(20 * time.Second)
		}
	}
}

// refreshIdentity re-fetches the profile name + avatar when they're missing
// (e.g. the startup whoami raced a cold network). Runs at most once at a time.
func (s *Server) refreshIdentity() {
	s.mu.Lock()
	// An account with no username yet answers without one every time: ask
	// again at most every 30 seconds, not on every status poll.
	if s.refreshing || s.cfg.Token == "" || time.Since(s.identityAt) < 30*time.Second {
		s.mu.Unlock()
		return
	}
	s.refreshing = true
	s.identityAt = time.Now()
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
		s.mu.Lock()
		s.userID = uid
		s.mu.Unlock()
	}
}

// NewServer wires the server to the saved config.
func NewServer(cfg config.Config, version string) *Server {
	// Report this build to the server on every push so it can turn away versions
	// that are too old (forced update).
	poster.SetVersion(version)
	s := &Server{cfg: cfg, version: version, orch: orchestrator.New(cfg)}
	s.orch.OnSeenGame(s.recordSeenGame)
	s.orch.OnUnlinked(s.dropLink)
	go s.watchUpdates()
	go s.noteNewVersion()
	if cfg.Token != "" {
		var uid string
		s.username, s.avatar, uid = whoami(cfg.Endpoint, cfg.Token)
		s.userID, s.identityAt = uid, time.Now()
		s.orch.SetUsername(s.username)
		s.orch.SetUserID(uid)
		s.orch.Start() // already linked → auto-start collecting
	}
	return s
}

// watchUpdates checks for a newer release on startup and every few hours after,
// from the user's own machine. A dev build never reports an update.
func (s *Server) watchUpdates() {
	s.checkUpdate()
	go func() {
		// Wait a little after boot so the prompt doesn't land on top of
		// everything else that opens with the computer.
		time.Sleep(90 * time.Second)
		for {
			s.maybeAutoUpdate()
			s.maybePromptUpdate()
			s.maybePromptLink()
			time.Sleep(time.Minute)
		}
	}()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for range t.C {
		s.checkUpdate()
	}
}

// checkUpdate asks GitHub for the latest release and remembers it when this
// build is behind. It returns that latest tag ("" when the check failed).
func (s *Server) checkUpdate() string {
	v, at, ok := update.Check(s.version)
	// A release that died on this PC before (the launcher fell back from it)
	// isn't offered again; the next one is. Nor is one glow.moe put on hold.
	if ok && (install.IsBad(v) || update.Held(v)) {
		return v
	}
	if ok {
		s.mu.Lock()
		s.updateVer, s.updateAt = v, at
		s.mu.Unlock()
	}
	return v
}

// autoUpdateAfter is how long a release must have been out before the app
// installs it on its own, so a bad one can be pulled before it spreads, when
// glow.moe hasn't said (it sets it: update.AutoUpdateAfter). The "Check for
// updates" button and "Update now" don't wait.
const autoUpdateAfter = 24 * time.Hour

// maybeAutoUpdate installs a waiting release in the background when the user
// left automatic updates on: only once it has been out autoUpdateAfter, never
// while a game is being read, and one try per release per session. The new
// copy comes back in the tray.
func (s *Server) maybeAutoUpdate() {
	st := s.orch.Status()
	s.mu.Lock()
	ver := s.updateVer
	due := autoUpdateDue(s.cfg.AutoUpdate, ver, s.autoFailed, s.updateAt, time.Now(), st.InGame)
	if due {
		s.autoFailed = ver // set before trying: a failure must not retry every minute
	}
	s.mu.Unlock()
	// Put on hold since the last check (a bad release being pulled).
	if !due || update.Held(ver) {
		return
	}
	if err := install.ApplyUpdate(ver); err != nil {
		applog.Printf("auto update to %s failed: %v", ver, err)
		return
	}
	applog.Printf("auto update to %s installed, restarting", ver)
	install.Restart(true)
}

// autoUpdateDue decides whether the automatic update should install `ver` now.
func autoUpdateDue(on bool, ver, failed string, published, now time.Time, inGame bool) bool {
	return on && ver != "" && ver != failed && !inGame &&
		!published.IsZero() && now.Sub(published) >= update.AutoUpdateAfter(autoUpdateAfter)
}

// hUpdateCheck: the "Check for updates" button. Asks GitHub right now and says
// whether this build is current; when it isn't, the update banner shows up
// (any "Later" is dropped).
func (s *Server) hUpdateCheck(w http.ResponseWriter, _ *http.Request) {
	if !update.IsRelease(s.version) {
		writeJSON(w, map[string]any{"ok": true, "dev": true, "current": s.version})
		return
	}
	latest := s.checkUpdate()
	if latest == "" {
		writeJSON(w, map[string]any{"ok": false, "current": s.version})
		return
	}
	s.mu.Lock()
	available := s.updateVer != ""
	if available {
		s.cfg.UpdateLaterVer, s.cfg.UpdateLaterUntil = "", 0
	}
	cfg := s.cfg
	s.mu.Unlock()
	if available {
		_ = config.Save(cfg)
	}
	writeJSON(w, map[string]any{"ok": true, "current": s.version, "latest": latest, "available": available})
}

// maybePromptUpdate brings the window up in its corner (no focus taken) once
// per release when an update is out, so a copy that lives in the tray still
// hears about it. Never while a game or show is live; "Later" quiets that
// release for a day unless the server has started turning this build away.
func (s *Server) maybePromptUpdate() {
	st := s.orch.Status()
	s.mu.Lock()
	ver, peek := s.updateVer, s.peekWindow
	key := ver
	if st.Outdated {
		key += "|forced"
	}
	snoozed := !st.Outdated && s.cfg.UpdateLaterVer == ver && time.Now().UnixMilli() < s.cfg.UpdateLaterUntil
	// With automatic updates on, the app installs it itself; no pop-up, unless
	// the server is turning this build away.
	if s.cfg.AutoUpdate && !st.Outdated {
		snoozed = true
	}
	skip := ver == "" || peek == nil || s.prompted == key || snoozed || st.InGame
	s.mu.Unlock()
	if skip || !peek() {
		return
	}
	s.mu.Lock()
	s.prompted, s.peeked = key, true
	s.mu.Unlock()
}

// maybePromptLink brings the window up in its corner (no focus taken) with the
// link screen after glow.moe turned this PC's key down, so a copy that lives
// in the tray doesn't sit there linked to nothing. Waits out full screen.
func (s *Server) maybePromptLink() {
	s.mu.Lock()
	peek := s.peekWindow
	due := s.linkPrompt && s.cfg.Token == "" && peek != nil
	s.mu.Unlock()
	if !due || !peek() {
		return
	}
	s.mu.Lock()
	s.linkPrompt = false
	s.mu.Unlock()
}

// hUpdateLater: "Later" on the prompt. Quiet for a day, and back to the tray if
// the prompt is what brought the window up.
func (s *Server) hUpdateLater(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.cfg.UpdateLaterVer = s.updateVer
	s.cfg.UpdateLaterUntil = time.Now().Add(24 * time.Hour).UnixMilli()
	cfg := s.cfg
	hide := s.peeked
	s.peeked = false
	fn := s.hideToTray
	s.mu.Unlock()
	_ = config.Save(cfg)
	if hide && fn != nil {
		fn()
	}
	writeJSON(w, map[string]any{"ok": true})
}

// hOpenDownload opens the releases page in the user's browser.
func (s *Server) hOpenDownload(w http.ResponseWriter, _ *http.Request) {
	openURL(update.ReleasesPage)
	writeJSON(w, map[string]any{"ok": true})
}

// hUpdateApply downloads the newest release, verifies it, swaps this binary and
// relaunches. On any failure it reports back so the UI can fall back to the
// manual download link.
func (s *Server) hUpdateApply(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	tag := s.updateVer
	s.mu.Unlock()
	if tag == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "no update available"})
		return
	}
	if err := install.ApplyUpdate(tag); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
	// Let the response flush, then relaunch into the freshly-swapped binary.
	go func() {
		time.Sleep(400 * time.Millisecond)
		install.Restart(false)
	}()
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
	mux.HandleFunc("/api/update-apply", s.hUpdateApply)
	mux.HandleFunc("/api/update-later", s.hUpdateLater)
	mux.HandleFunc("/api/update-check", s.hUpdateCheck)
	mux.HandleFunc("/api/diagnose", s.hDiagnose)
	mux.HandleFunc("/api/profile-hide", s.hProfileHide)
	mux.HandleFunc("/api/quit", s.hQuit)
	mux.HandleFunc("/api/whats-new-seen", s.hWhatsNewSeen)
	mux.HandleFunc("/api/export-logs", s.hExportLogs)
	mux.HandleFunc("/api/open-onboarding", s.hOpenOnboarding)
	// Local OBS overlay data: the overlay page (below, or the glow.moe page with
	// ?src=local) reads the current masked League snapshot from here instead of
	// polling glow.moe, so the ~2s live poll never touches our server.
	mux.HandleFunc("/live.json", s.hLive)
	// Self-contained OBS overlay page. Point an OBS Browser Source at
	// http://127.0.0.1:47100/overlay: it renders the League card entirely from
	// /live.json on this loopback server, so nothing touches glow.moe (works
	// offline). Rank crests + fonts are bundled; ddragon art is Riot's CDN.
	mux.HandleFunc("/overlay", s.hOverlay)
	return localOnly(mux)
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
	s.mu.Lock()
	endpoint := s.cfg.Endpoint
	s.mu.Unlock()
	b := remoteOverlay(endpoint) // glow.moe's current page (overlay.go)
	if b == nil {
		var err error
		if b, err = webFS.ReadFile("web/overlay.html"); err != nil {
			http.Error(w, "overlay not found", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts")
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
		"version":     s.version,
		"linked":      linked,
		"username":    s.username,
		"avatar":      s.avatar,
		"linking":     s.linking,
		"linkCode":    s.linkCode,
		"wasUnlinked": s.wasUnlinked,
		"whatsNew":    map[string]any{"version": s.whatsNew, "notes": s.whatsNewNotes},
		// Linked to an account that hasn't picked a username: nothing shows
		// until it does (glow.moe/onboarding).
		"noProfile": linked && s.userID != "" && s.username == "",
		"running":   s.orch.Running(),
		"status":    s.orch.Status(),
		"league":    orchestrator.LeagueSupported(),
		"update":    map[string]any{"version": s.updateVer, "url": update.ReleasesPage},
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
			DelaySec        *int    `json:"delaySec"`
			PollMs          *int    `json:"pollMs"`
			Endpoint        *string `json:"endpoint"`
			AnimePresence   *bool   `json:"animePresence"`
			ReadingPresence *bool   `json:"readingPresence"`
			MusicPresence   *bool   `json:"musicPresence"`
			RobloxPresence  *bool   `json:"robloxPresence"`
			SteamPresence   *bool   `json:"steamPresence"`
			// SplitcraftPresence: the SplitCraft session on Discord.
			SplitcraftPresence *bool `json:"splitcraftPresence"`
			AutoStart          *bool `json:"autoStart"`
			StartHidden        *bool `json:"startHidden"`
			HideOnGame         *bool `json:"hideOnGame"`
			AutoUpdate         *bool `json:"autoUpdate"`
			HideGame           *struct {
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
		if body.ReadingPresence != nil {
			s.cfg.ReadingPresence = *body.ReadingPresence
		}
		if body.MusicPresence != nil {
			s.cfg.MusicPresence = *body.MusicPresence
		}
		if body.RobloxPresence != nil {
			s.cfg.RobloxPresence = *body.RobloxPresence
		}
		if body.SteamPresence != nil {
			s.cfg.SteamPresence = *body.SteamPresence
		}
		if body.SplitcraftPresence != nil {
			s.cfg.SplitcraftPresence = *body.SplitcraftPresence
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
		if body.AutoUpdate != nil {
			s.cfg.AutoUpdate = *body.AutoUpdate
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
		"delaySec":           s.cfg.DelaySec,
		"pollMs":             s.cfg.PollMs,
		"endpoint":           s.cfg.Endpoint,
		"animePresence":      s.cfg.AnimePresence,
		"readingPresence":    s.cfg.ReadingPresence,
		"musicPresence":      s.cfg.MusicPresence,
		"robloxPresence":     s.cfg.RobloxPresence,
		"steamPresence":      s.cfg.SteamPresence,
		"splitcraftPresence": s.cfg.SplitcraftPresence,
		"autoStart":          s.cfg.AutoStart,
		"startHidden":        s.cfg.StartHidden,
		"hideOnGame":         s.cfg.HideOnGame,
		"autoUpdate":         s.cfg.AutoUpdate,
		"seenGames":          s.cfg.SeenGames,
		"hiddenGames":        s.cfg.HiddenGames,
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
		s.wasUnlinked, s.linkPrompt = false, false
		s.cfg.Token = ""
		s.username = ""
		s.avatar = ""
		s.userID = ""
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
		s.wasUnlinked = false
		base := pair.BaseFrom(s.cfg.Endpoint)
		s.mu.Unlock()
		go func() {
			tok, err := pair.Run(base, func(code string) {
				s.mu.Lock()
				s.linkCode = code
				s.mu.Unlock()
			})
			if err != nil {
				applog.Printf("link failed: %v", err)
			} else {
				applog.Printf("link approved")
			}
			s.mu.Lock()
			s.linking = false
			s.linkCode = ""
			if err == nil && tok != "" {
				s.cfg.Token = tok
				cfg := s.cfg
				s.mu.Unlock()
				_ = config.Save(cfg)
				s.orch.SetConfig(cfg)
				name, av, uid := whoami(cfg.Endpoint, tok)
				s.mu.Lock()
				s.userID, s.identityAt = uid, time.Now()
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
	s.userID, s.identityAt = uid, time.Now()
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
	resp, err := poster.Do(&http.Client{Timeout: 6 * time.Second}, req)
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
