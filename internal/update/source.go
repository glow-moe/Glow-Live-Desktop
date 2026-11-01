package update

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/poster"
)

// Where releases come from. glow.moe answers first (GET /api/live/release):
// when it hosts the newest release its manifest names the files, their
// checksums and the notes, and the app updates from there. While it doesn't
// (no "latest"), GitHub stays the source, as before. Moving releases off
// GitHub (say the source goes private) then needs no new app: only the files
// and the manifest on the server (glow.moe lib/live/releases).

var holdURL = "https://glow.moe/api/live/release"

type hostedFile struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type hostedRelease struct {
	Tag         string                `json:"tag"`
	PublishedAt time.Time             `json:"publishedAt"`
	Files       map[string]hostedFile `json:"files"`
	Notes       []string              `json:"notes"`
}

var (
	hostedMu sync.Mutex
	hosted   *hostedRelease // glow.moe's newest release from the last check, if it hosts one
	shaRE    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// releaseInfo asks glow.moe for the hold list and the release it hosts.
func releaseInfo() (hold []string, latest *hostedRelease, ok bool) {
	req, err := http.NewRequest(http.MethodGet, holdURL, nil)
	if err != nil {
		return nil, nil, false
	}
	resp, err := poster.Do(&http.Client{Timeout: 6 * time.Second}, req)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false
	}
	var body struct {
		Hold   []string       `json:"hold"`
		Latest *hostedRelease `json:"latest"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return nil, nil, false
	}
	return body.Hold, usable(body.Latest), true
}

// usable keeps a hosted release only when it has this OS's file, from
// glow.moe itself over https, with a proper checksum.
func usable(r *hostedRelease) *hostedRelease {
	if r == nil || !IsRelease(r.Tag) {
		return nil
	}
	f, ok := r.Files[assetName()]
	if !ok || !shaRE.MatchString(strings.ToLower(f.SHA256)) || !glowURL(f.URL) {
		return nil
	}
	return r
}

// glowURL: an https address on glow.moe or one of its subdomains.
func glowURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "glow.moe" || strings.HasSuffix(h, ".glow.moe")
}

// hostedFor is glow.moe's file for release `tag`, when that's where it lives.
func hostedFor(tag string) (hostedFile, bool) {
	hostedMu.Lock()
	defer hostedMu.Unlock()
	if hosted == nil || hosted.Tag != tag {
		return hostedFile{}, false
	}
	f, ok := hosted.Files[assetName()]
	return f, ok
}

// assetURL is where release `tag` of this OS's binary downloads from.
func assetURL(tag string) string {
	if f, ok := hostedFor(tag); ok {
		return f.URL
	}
	return downloadBase + "/" + tag + "/" + assetName()
}
