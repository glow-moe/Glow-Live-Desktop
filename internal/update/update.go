// Package update checks whether a newer desktop release is out, by asking
// GitHub's releases API from the user's own machine (their IP, their 60/hr
// budget - glow is never involved). Best-effort: any failure just means "no
// update to show".
package update

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	releasesAPI = "https://api.github.com/repos/glow-moe/Glow-Live-Desktop/releases/latest"
	// ReleasesPage is where the "download" button sends the user.
	// ReleasesPage is where the "download" button sends people: glow.moe's L!VE
	// page, whose buttons go wherever releases live.
	ReleasesPage = "https://glow.moe/dashboard/live"
)

// Check reports the latest published release, when it was published, and
// whether `current` is behind it. A dev build (anything not starting with "v")
// returns available=false: those are test builds and must not nag the person
// running them.
func Check(current string) (latest string, published time.Time, available bool) {
	if !IsRelease(current) {
		return "", time.Time{}, false
	}
	// glow.moe first: a release it hosts is the one to take.
	if _, r, ok := releaseInfo(); ok {
		hostedMu.Lock()
		hosted = r
		hostedMu.Unlock()
		if r != nil {
			return r.Tag, r.PublishedAt, cmpVer(current, r.Tag) < 0
		}
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(releasesAPI)
	if err != nil {
		return "", time.Time{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, false
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.TagName == "" {
		return "", time.Time{}, false
	}
	return body.TagName, body.PublishedAt, cmpVer(current, body.TagName) < 0
}

// notesURL is one release on GitHub, for its notes.
var notesURL = "https://api.github.com/repos/glow-moe/Glow-Live-Desktop/releases/tags/"

// Notes returns release `tag`'s notes as lines of plain text (the bullets of
// the release body, markdown stripped). Nil when GitHub can't be reached.
func Notes(tag string) []string {
	// Right after an update nothing has asked glow.moe yet in this run.
	hostedMu.Lock()
	known := hosted != nil
	hostedMu.Unlock()
	if !known {
		if _, r, ok := releaseInfo(); ok && r != nil {
			hostedMu.Lock()
			hosted = r
			hostedMu.Unlock()
		}
	}
	hostedMu.Lock()
	if hosted != nil && hosted.Tag == tag && len(hosted.Notes) > 0 {
		n := append([]string(nil), hosted.Notes...)
		hostedMu.Unlock()
		return n
	}
	hostedMu.Unlock()
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(notesURL + tag)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var body struct {
		Body string `json:"body"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(body.Body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") {
			continue
		}
		line = strings.NewReplacer("**", "", "`", "").Replace(strings.TrimSpace(line[2:]))
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// Held reports whether glow.moe put release `tag` on hold. A failed check
// isn't a hold: the day's wait after a release already guards most of it.
func Held(tag string) bool {
	hold, _, ok := releaseInfo()
	if !ok {
		return false
	}
	for _, h := range hold {
		if strings.TrimSpace(h) == tag {
			return true
		}
	}
	return false
}

// IsRelease reports whether a build label is a numbered release ("v26.9"),
// as opposed to a dev or test build, which never updates itself.
func IsRelease(version string) bool { return strings.HasPrefix(version, "v") }

// Compare orders two release labels ("v26.9.1" < "v26.10"): -1, 0 or 1.
func Compare(a, b string) int { return cmpVer(strings.TrimSpace(a), strings.TrimSpace(b)) }

// cmpVer compares dotted versions ("v1.2" vs "v1.3"), ignoring a leading "v".
func cmpVer(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parts(v string) [3]int {
	var out [3]int
	for i, s := range strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3) {
		if i > 2 {
			break
		}
		out[i], _ = strconv.Atoi(s)
	}
	return out
}
