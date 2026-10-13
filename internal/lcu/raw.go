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
// login, settings or anything that acts on the account. GET only, no dot
// segments, a plain query string at most, and a size cap per body.

// rawPrefixes are the client areas the site may ask for.
var rawPrefixes = []string{
	"/lol-lobby/",
	"/lol-champ-select/",
	"/lol-challenges/",
	"/lol-ranked/",
	"/lol-summoner/",
	"/lol-game-queues/",
	"/lol-gameflow/",
	"/lol-honor-v2/",
	"/lol-match-history/",
	"/lol-champion-mastery/",
}

// rawExact are single paths from areas that are otherwise off limits: your own
// chat presence (availability + status line), never messages or friends.
var rawExact = map[string]bool{"/lol-chat/v1/me": true}

var rawPathRE = regexp.MustCompile(`^/[a-z0-9\-/]+(\?[A-Za-z0-9=&_\-]{1,200})?$`)

// rawMax caps one body (the challenge summary is ~10 KB).
const rawMax = 256 << 10

// ErrRawNotAllowed is returned for a path outside the allowlist.
var ErrRawNotAllowed = errors.New("lcu: raw read not allowed")

// RawAllowed reports whether the site may have the app read this client path.
func RawAllowed(path string) bool {
	if !rawPathRE.MatchString(path) || strings.Contains(path, "//") || strings.Contains(path, "..") {
		return false
	}
	if rawExact[path] {
		return true
	}
	for _, p := range rawPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// PickFields keeps only the listed fields of a JSON body (the site says which it
// needs, so pushes stay small). A field is a dotted path ("timer.phase",
// "games.games.participants.championId"); arrays on the way are walked element
// by element. No fields = the body as is; a body that isn't JSON comes back as is.
func PickFields(body json.RawMessage, fields []string) json.RawMessage {
	if len(fields) == 0 {
		return body
	}
	var v any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	tree := map[string]any{}
	for _, f := range fields {
		node := tree
		parts := strings.Split(f, ".")
		for i, p := range parts {
			if i == len(parts)-1 {
				node[p] = true
				break
			}
			next, ok := node[p].(map[string]any)
			if !ok {
				if node[p] == true {
					break // the whole parent is already kept
				}
				next = map[string]any{}
				node[p] = next
			}
			node = next
		}
	}
	b, err := json.Marshal(project(v, tree))
	if err != nil {
		return body
	}
	return b
}

// project keeps the parts of v named by tree (true = keep the whole value).
func project(v any, tree map[string]any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = project(e, tree)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, sub := range tree {
			val, ok := x[k]
			if !ok {
				continue
			}
			if sub == true {
				out[k] = val
			} else if m, ok := sub.(map[string]any); ok {
				out[k] = project(val, m)
			}
		}
		return out
	}
	return v // a number or string where the path wanted more: kept as is
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
