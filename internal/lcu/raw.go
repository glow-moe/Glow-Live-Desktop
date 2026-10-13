package lcu

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Raw reads: glow.moe lists League client paths to read (in its settings), the
// app GETs them as-is and sends the bodies back untouched, and the site decides
// what to show. New League details then need a site update, not an app release.
//
// Only read-only areas are reachable, whatever the list says: no chat, store,
// login, settings or anything that acts on the account. GET only, no query
// strings or dot segments, and a size cap per body.

// rawPrefixes are the client areas the site may ask for.
var rawPrefixes = []string{
	"/lol-lobby/",
	"/lol-champ-select/",
	"/lol-challenges/",
	"/lol-ranked/",
	"/lol-summoner/",
	"/lol-game-queues/",
	"/lol-gameflow/",
}

var rawPathRE = regexp.MustCompile(`^/[a-z0-9\-/]+$`)

// rawMax caps one body (the challenge summary is ~10 KB).
const rawMax = 256 << 10

// ErrRawNotAllowed is returned for a path outside the allowlist.
var ErrRawNotAllowed = errors.New("lcu: raw read not allowed")

// RawAllowed reports whether the site may have the app read this client path.
func RawAllowed(path string) bool {
	if !rawPathRE.MatchString(path) || strings.Contains(path, "//") || strings.Contains(path, "..") {
		return false
	}
	for _, p := range rawPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// PickFields keeps only the named top-level keys of a JSON object (the site
// says which it needs, so pushes stay small); anything else comes back as is.
func PickFields(body json.RawMessage, keys []string) json.RawMessage {
	if len(keys) == 0 {
		return body
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil {
		return body
	}
	out := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			out[k] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return body
	}
	return b
}

// ReadRaw GETs one allowed client path and returns its JSON body unchanged.
func ReadRaw(leaguePath, path string) (json.RawMessage, error) {
	if !RawAllowed(path) {
		return nil, ErrRawNotAllowed
	}
	c, err := dial(leaguePath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, rawMax))
		return nil, errors.New(resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, rawMax+1))
	if err != nil {
		return nil, err
	}
	if len(b) > rawMax {
		return nil, errors.New("lcu: raw body too large")
	}
	if !json.Valid(b) {
		return nil, errors.New("lcu: raw body is not JSON")
	}
	return json.RawMessage(b), nil
}
