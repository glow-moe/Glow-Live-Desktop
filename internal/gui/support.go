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
	"github.com/glow-moe/glow-collector/internal/install"
	"github.com/glow-moe/glow-collector/internal/pair"
	"github.com/glow-moe/glow-collector/internal/poster"
	"github.com/glow-moe/glow-collector/internal/update"
)

// check is one line of "Test connection": what was tried and how it went.
type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// diagnose runs the connection checks side by side: glow.moe itself, the
// fallback address, this PC's key, Discord, GitHub (updates) and what the app
// is reading right now. Nothing here changes anything.
func (s *Server) diagnose() []check {
	s.mu.Lock()
	base, tok := pair.BaseFrom(s.cfg.Endpoint), s.cfg.Token
	s.mu.Unlock()
	client := &http.Client{Timeout: 8 * time.Second}
	ping := func(name, url string) check {
		t := time.Now()
		resp, err := client.Get(url)
		if err != nil {
			return check{name, false, shortErr(err)}
		}
		resp.Body.Close()
		ms := time.Since(t).Milliseconds()
		return check{name, resp.StatusCode/100 == 2, fmt.Sprintf("%s in %d ms", resp.Status, ms)}
	}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		out = make([]check, 6)
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
	run(0, func() check { return ping("glow.moe", base+"/api/health") })
	run(1, func() check {
		if fb := poster.Fallback(); fb != "" {
			return ping("Backup address", fb+"/api/health")
		}
		return check{"Backup address", true, "none set (not needed)"}
	})
	run(2, func() check {
		if tok == "" {
			return check{"This PC's link", false, "not linked"}
		}
		name, _, id := whoami(s.cfg.Endpoint, tok)
		switch {
		case id == "":
			return check{"This PC's link", false, "glow.moe didn't accept this PC's key: unlink and link again"}
		case name == "":
			return check{"This PC's link", false, "linked, but the account has no username yet (glow.moe/onboarding)"}
		}
		return check{"This PC's link", true, "linked as @" + name}
	})
	run(3, func() check {
		on, last := s.orch.DiscordState()
		if on {
			return check{"Discord", true, "connected"}
		}
		if last == "" {
			last = "not connected yet (it connects when there's a status to show)"
		}
		return check{"Discord", false, last}
	})
	run(4, func() check {
		if !update.IsRelease(s.version) {
			return check{"Updates", true, "test build, updates are off"}
		}
		latest, _, behind := update.Check(s.version)
		if latest == "" {
			return check{"Updates", false, "couldn't reach GitHub"}
		}
		if behind {
			return check{"Updates", true, latest + " is out (this is " + s.version + ")"}
		}
		return check{"Updates", true, "on the latest (" + s.version + ")"}
	})
	run(5, func() check {
		st := s.orch.Status()
		if !s.orch.Running() {
			return check{"Reading", false, "stopped (press Start)"}
		}
		if st.Err != "" {
			return check{"Reading", false, st.Err}
		}
		if st.InGame {
			return check{"Reading", true, strings.TrimSpace(st.Game + " " + st.Detail)}
		}
		return check{"Reading", true, "running, no game open right now"}
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
		applog.Printf("test %s ok=%v %s", c.Name, c.OK, c.Detail)
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
		fmt.Fprintf(&b, "  %s %s: %s\n", mark, c.Name, c.Detail)
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
