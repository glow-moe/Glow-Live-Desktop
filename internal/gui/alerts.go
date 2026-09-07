package gui

// Local, self-contained hype-alert config: the user picks a gif/image + a sound
// per event (first blood / double / triple / quadra / penta) from a LOCAL page
// (/alerts/settings). Uploaded files live in the app's config dir and are served
// back over loopback, so the /alerts OBS overlay can show them - nothing touches
// glow.moe. Config is a small JSON file next to config.json.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AlertEvent is one hype event's setup. Gif/Sound are URLs the overlay loads:
// either a local asset ("/alerts/asset/<file>") or a pasted https URL. Empty =
// fall back to the built-in banner + chime.
type AlertEvent struct {
	On    bool   `json:"on"`
	Gif   string `json:"gif"`
	Sound string `json:"sound"`
	// Volume 0..1 for the sound (0 = default 0.9).
	Volume float64 `json:"volume"`
	// Original file names, shown on the settings page (assets are renamed on disk).
	GifName   string `json:"gifName,omitempty"`
	SoundName string `json:"soundName,omitempty"`
}

// AlertsConfig is the whole local setup, keyed by event kind.
type AlertsConfig struct {
	Events map[string]AlertEvent `json:"events"`
}

// alertKinds is the fixed set the settings page + overlay know about.
var alertKinds = []string{"firstblood", "double", "triple", "quadra", "penta"}

func alertsBaseDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "glow-collector"), nil
}

func alertsConfigPath() (string, error) {
	d, err := alertsBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "alerts.json"), nil
}

func alertsAssetsDir() (string, error) {
	d, err := alertsBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "alerts-assets"), nil
}

func loadAlerts() AlertsConfig {
	c := AlertsConfig{Events: map[string]AlertEvent{}}
	p, err := alertsConfigPath()
	if err != nil {
		return c
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c)
	if c.Events == nil {
		c.Events = map[string]AlertEvent{}
	}
	return c
}

func saveAlerts(c AlertsConfig) error {
	p, err := alertsConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// hAlertsConfig GETs the current config or POSTs a new one (JSON body). CORS-open
// so the loopback overlay/settings pages can read/write it.
func (s *Server) hAlertsConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodPost {
		var c AlertsConfig
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if c.Events == nil {
			c.Events = map[string]AlertEvent{}
		}
		if err := saveAlerts(c); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	b, _ := json.Marshal(loadAlerts())
	_, _ = w.Write(b)
}

// hAlertsUpload saves an uploaded gif/sound for an event locally and returns its
// loopback URL. One file per (event,type): re-uploading replaces the previous.
//
//	POST /api/alerts/upload?event=penta&type=gif   (multipart, field "file")
func (s *Server) hAlertsUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	event := safeSeg(r.URL.Query().Get("event"))
	typ := safeSeg(r.URL.Query().Get("type"))
	if event == "" || (typ != "gif" && typ != "sound") {
		http.Error(w, "event+type required", http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB cap
		http.Error(w, "too big", http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file", http.StatusBadRequest)
		return
	}
	defer f.Close()

	dir, err := alertsAssetsDir()
	if err != nil {
		http.Error(w, "dir", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "dir", http.StatusInternalServerError)
		return
	}
	// Drop any previous asset for this event/type so files don't accumulate.
	if old, _ := filepath.Glob(filepath.Join(dir, event+"-"+typ+"-*")); old != nil {
		for _, o := range old {
			_ = os.Remove(o)
		}
	}
	ext := safeExt(hdr.Filename)
	name := fmt.Sprintf("%s-%s-%d%s", event, typ, time.Now().UnixNano(), ext)
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		http.Error(w, "create", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": "/alerts/asset/" + name})
}

// hAlertsAsset serves a saved asset (basename only, no path traversal).
func (s *Server) hAlertsAsset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	name := filepath.Base(strings.TrimPrefix(r.URL.Path, "/alerts/asset/"))
	if name == "" || name == "." || strings.ContainsAny(name, "/\\") {
		http.NotFound(w, r)
		return
	}
	dir, err := alertsAssetsDir()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filepath.Join(dir, name))
}

// safeSeg keeps a short a-z0-9 slug (for event/type query values).
func safeSeg(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() >= 16 {
			break
		}
	}
	return b.String()
}

// safeExt returns a lowercase ".ext" from a filename, or "" - only for common
// image/audio types we're willing to serve.
func safeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".gif", ".png", ".jpg", ".jpeg", ".webp", ".apng", ".mp3", ".ogg", ".wav", ".m4a", ".aac", ".webm":
		return ext
	}
	return ""
}
