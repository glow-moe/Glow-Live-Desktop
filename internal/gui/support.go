package gui

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/applog"
	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/install"
	"github.com/glow-moe/glow-collector/internal/pair"
	"github.com/glow-moe/glow-collector/internal/poster"
	"github.com/glow-moe/glow-collector/internal/update"
)

// check is one line of "Test connection": what was tried and how it went,
// in plain words for the window (Detail) and with the technical reason for
// the exported log (Tech, never shown).
type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Tech   string `json:"-"`
}

// diagnose runs the connection checks side by side: glow.moe itself (and the
// backup address when glow.moe fails and one is set), this PC's link,
// Discord, updates and what the app is reading right now. Nothing here
// changes anything. A row is red only for something the person can fix.
func (s *Server) diagnose() []check {
	s.mu.Lock()
	base, tok := pair.BaseFrom(s.cfg.Endpoint), s.cfg.Token
	s.mu.Unlock()
	client := &http.Client{Timeout: 8 * time.Second}
	ping := func(url string) (bool, string) {
		t := time.Now()
		resp, err := client.Get(url)
		if err != nil {
			return false, shortErr(err)
		}
		resp.Body.Close()
		return resp.StatusCode/100 == 2, fmt.Sprintf("%s in %d ms", resp.Status, time.Since(t).Milliseconds())
	}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		out = make([]check, 5)
	)
	run := func(i int, fn func() check) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := fn()
			mu.Lock()
			out[i] = c
			mu.Unlock()
		}()
	}
	run(0, func() check {
		ok, tech := ping(base + "/api/health")
		if ok {
			return check{"glow.moe", true, "connected", tech}
		}
		// glow.moe is out of reach: say whether the backup way gets through.
		if fb := poster.Fallback(); fb != "" {
			if fbOK, fbTech := ping(fb + "/api/health"); fbOK {
				return check{"glow.moe", true, "connected through the backup route", tech + " | backup " + fbTech}
			}
		}
		return check{"glow.moe", false, "can't reach it. Check your internet, or try another connection.", tech}
	})
	run(1, func() check {
		if tok == "" {
			return check{"Your account", false, "this PC isn't linked yet", ""}
		}
		name, _, id := whoami(s.cfg.Endpoint, tok)
		switch {
		case id == "":
			return check{"Your account", false, "not linked anymore. Use unlink / switch account to link again.", "whoami without an id"}
		case name == "":
			return check{"Your account", false, "pick a username on glow.moe first", "no username"}
		}
		return check{"Your account", true, "linked as @" + name, ""}
	})
	run(2, func() check {
		on, last := s.orch.DiscordState()
		switch {
		case on:
			return check{"Discord", true, "connected", ""}
		case strings.Contains(strings.ToLower(last), "not running"):
			return check{"Discord", false, "isn't open. Open Discord to show your status there.", last}
		}
		// Not connected yet only means nothing to show so far.
		return check{"Discord", true, "ready, shows up when you play", last}
	})
	run(3, func() check {
		if !update.IsRelease(s.version) {
			return check{"Updates", true, "test build", ""}
		}
		latest, _, behind := update.Check(s.version)
		if latest == "" {
			return check{"Updates", true, "couldn't check right now", "GitHub unreachable"}
		}
		if behind {
			return check{"Updates", true, latest + " is out", "this is " + s.version}
		}
		return check{"Updates", true, "up to date", s.version}
	})
	run(4, func() check {
		st := s.orch.Status()
		if !s.orch.Running() {
			return check{"Games", false, "stopped. Press Start.", ""}
		}
		if st.Err != "" {
			return check{"Games", false, st.Err, st.Err}
		}
		if st.InGame {
			return check{"Games", true, "reading " + strings.TrimSpace(st.Detail), st.Game}
		}
		return check{"Games", true, "ready, no game open right now", ""}
	})
	wg.Wait()
	return out
}

// shortErr keeps the end of a network error, where the reason is.
func shortErr(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i > 0 && len(msg)-i < 160 {
		return msg[i+2:]
	}
	return msg
}

func (s *Server) hDiagnose(w http.ResponseWriter, _ *http.Request) {
	checks := s.diagnose()
	for _, c := range checks {
		applog.Printf("test %s ok=%v %s (%s)", c.Name, c.OK, c.Detail, c.Tech)
	}
	writeJSON(w, map[string]any{"ok": true, "checks": checks})
}

// hExportLogs writes the log file to Downloads with a header (this build,
// the install, settings, a fresh connection test) and shows it in Explorer.
// The key never goes in.
func (s *Server) hExportLogs(w http.ResponseWriter, _ *http.Request) {
	checks := s.diagnose()
	s.mu.Lock()
	cfg, name := s.cfg, s.username
	s.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "glow L!VE %s on %s/%s, exported %s\n", s.version, runtime.GOOS, runtime.GOARCH, time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "linked: %v  account: @%s  autostart: %v  auto update: %v\n", cfg.Token != "", name, cfg.AutoStart, cfg.AutoUpdate)
	if d := install.Describe(); d != "" {
		b.WriteString("install: " + d + "\n")
	}
	b.WriteString("\nconnection test:\n")
	for _, c := range checks {
		mark := "ok  "
		if !c.OK {
			mark = "FAIL"
		}
		fmt.Fprintf(&b, "  %s %s: %s [%s]\n", mark, c.Name, c.Detail, c.Tech)
	}
	path, err := applog.Export(b.String())
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	reveal(path)
	writeJSON(w, map[string]any{"ok": true, "path": path, "name": filepath.Base(path)})
}

// reveal shows the file in the system file manager.
func reveal(path string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer", "/select,", path).Start()
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	default:
		_ = exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

// hOpenOnboarding opens the username picker for an account that has none.
func (s *Server) hOpenOnboarding(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	base := pair.BaseFrom(s.cfg.Endpoint)
	s.mu.Unlock()
	openURL(base + "/onboarding")
	writeJSON(w, map[string]any{"ok": true})
}

// noteNewVersion runs at start: the first start of a newer release (after an
// update, not a fresh install) fetches that release's notes for the "What's
// new" card, and every start records this build as the last one.
func (s *Server) noteNewVersion() {
	s.mu.Lock()
	last, seen := s.cfg.LastVersion, s.cfg.WhatsNewSeen
	s.cfg.LastVersion = s.version
	cfg := s.cfg
	s.mu.Unlock()
	if last != s.version {
		_ = config.Save(cfg)
	}
	updated := last != "" && update.IsRelease(s.version) && update.IsRelease(last) && update.Compare(s.version, last) > 0
	// Also after a restart that came before the card was closed.
	pending := last == s.version && seen != s.version && update.IsRelease(s.version) && seen != ""
	if !updated && !pending {
		return
	}
	notes := update.Notes(s.version)
	if len(notes) == 0 {
		return
	}
	applog.Printf("what's new: %s (%d notes)", s.version, len(notes))
	s.mu.Lock()
	s.whatsNew, s.whatsNewNotes = s.version, notes
	s.mu.Unlock()
}

// hWhatsNewSeen closes the "What's new" card for this release.
func (s *Server) hWhatsNewSeen(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.whatsNew, s.whatsNewNotes = "", nil
	s.cfg.WhatsNewSeen = s.version
	cfg := s.cfg
	s.mu.Unlock()
	_ = config.Save(cfg)
	writeJSON(w, map[string]any{"ok": true})
}

// dropLink forgets a key glow.moe turned down (this PC was removed on the
// site, or the password changed): the app goes back to its link screen, with
// the big Link button and a line saying why, instead of a status to decode.
func (s *Server) dropLink(token string) {
	s.mu.Lock()
	// Only the key glow.moe turned down: a read that left before linking again
	// can't drop the new one.
	if s.cfg.Token == "" || s.cfg.Token != token {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.orch.Stop()
	s.mu.Lock()
	if s.cfg.Token != token {
		s.mu.Unlock()
		return
	}
	s.cfg.Token = ""
	s.username, s.avatar, s.userID = "", "", ""
	s.wasUnlinked = true
	s.linkPrompt = true
	cfg := s.cfg
	s.mu.Unlock()
	_ = config.Save(cfg)
	s.orch.SetConfig(cfg)
	applog.Printf("glow.moe turned this PC's key down: back to the link screen")
	s.maybePromptLink()
}
