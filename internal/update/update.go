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
	ReleasesPage = "https://github.com/glow-moe/Glow-Live-Desktop/releases/latest"
)

// Check reports the latest published release and whether `current` is behind it.
// A dev build (anything not starting with "v") returns available=false: those
// are test builds and must not nag the person running them.
func Check(current string) (latest string, available bool) {
	if !strings.HasPrefix(current, "v") {
		return "", false
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(releasesAPI)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.TagName == "" {
		return "", false
	}
	return body.TagName, cmpVer(current, body.TagName) < 0
}

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
