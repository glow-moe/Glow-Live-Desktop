// Package roblox works out which Roblox experience is being played, for the
// Discord "Playing" line. Roblox writes a log line every time the client joins
// a server ("! Joining game '<server>' place <placeId> at <ip>"); the newest
// one, while the player process is running, is the game. Names and icons come
// from Roblox's public web APIs. The server id and IP are never used or shared.
package roblox

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Game is the experience being played right now.
type Game struct {
	PlaceID  int64
	Name     string
	Creator  string
	Icon     string // https URL, or "" when Roblox has none
	JoinedAt time.Time
}

var (
	joinRe  = regexp.MustCompile(`! Joining game '[^']*' place (\d+) at `)
	leaveRe = regexp.MustCompile(`\[FLog::Network\] (?:Time to disconnect replication data|Client:Disconnect)`)
	tsRe    = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)`)
)

// lastJoin scans a log for the newest join that wasn't followed by a leave.
func lastJoin(r io.Reader) (placeID int64, at time.Time, ok bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if m := joinRe.FindSubmatch(line); m != nil {
			id, err := strconv.ParseInt(string(m[1]), 10, 64)
			if err != nil {
				continue
			}
			placeID, ok = id, true
			at = time.Time{}
			if t := tsRe.FindSubmatch(line); t != nil {
				at, _ = time.Parse(time.RFC3339Nano, string(t[1]))
			}
			continue
		}
		if ok && leaveRe.Match(line) {
			ok, placeID = false, 0
		}
	}
	return placeID, at, ok
}

// newestLog is the most recently written client log across the folders, if
// it's recent.
func newestLog(dirs []string) (string, bool) {
	var files []string
	for _, dir := range dirs {
		if m, err := filepath.Glob(filepath.Join(dir, "*.log")); err == nil {
			files = append(files, m...)
		}
	}
	if len(files) == 0 {
		return "", false
	}
	type f struct {
		p string
		t time.Time
	}
	var all []f
	for _, p := range files {
		// Studio writes its own logs next to the player's; only the player counts.
		// The crash handler keeps a small log of its own beside the player's.
		if b := strings.ToLower(filepath.Base(p)); strings.Contains(b, "studio") || strings.Contains(b, "crashhandler") {
			continue
		}
		if st, err := os.Stat(p); err == nil {
			all = append(all, f{p, st.ModTime()})
		}
	}
	if len(all) == 0 {
		return "", false
	}
	sort.Slice(all, func(i, j int) bool { return all[i].t.After(all[j].t) })
	if time.Since(all[0].t) > 12*time.Hour {
		return "", false
	}
	return all[0].p, true
}

// tail reads at most the last n bytes of a file (logs grow to tens of MB).
func tail(path string, n int64) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	if _, err := fh.Seek(off, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(fh)
}

var (
	mu        sync.Mutex
	infoCache = map[int64]Game{}
	missAt    = map[int64]time.Time{}
)

// Current is the experience being played, or ok=false when Roblox isn't
// running or the player is between servers.
func Current() (Game, bool) {
	if !playerRunning() {
		return Game{}, false
	}
	p, ok := newestLog(logDirs())
	if !ok {
		return Game{}, false
	}
	b, err := tail(p, 4<<20)
	if err != nil {
		return Game{}, false
	}
	placeID, at, ok := lastJoin(bytes.NewReader(b))
	if !ok {
		return Game{}, false
	}
	g := lookup(placeID)
	g.PlaceID, g.JoinedAt = placeID, at
	return g, true
}

// lookup names a place, cached; a failed lookup is retried after a minute and
// the game shows as plain "Roblox" meanwhile.
func lookup(placeID int64) Game {
	mu.Lock()
	if g, ok := infoCache[placeID]; ok {
		mu.Unlock()
		return g
	}
	if t, ok := missAt[placeID]; ok && time.Since(t) < time.Minute {
		mu.Unlock()
		return Game{Name: "Roblox"}
	}
	mu.Unlock()

	g, err := fetchInfo(placeID)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		missAt[placeID] = time.Now()
		return Game{Name: "Roblox"}
	}
	if len(infoCache) > 500 {
		infoCache = map[int64]Game{}
	}
	infoCache[placeID] = g
	return g
}

var client = &http.Client{Timeout: 6 * time.Second}

func getJSON(url string, v any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("roblox api: %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

func fetchInfo(placeID int64) (Game, error) {
	var u struct {
		UniverseID int64 `json:"universeId"`
	}
	if err := getJSON(fmt.Sprintf("https://apis.roblox.com/universes/v1/places/%d/universe", placeID), &u); err != nil || u.UniverseID == 0 {
		return Game{}, fmt.Errorf("no universe for place %d", placeID)
	}
	var games struct {
		Data []struct {
			Name    string `json:"name"`
			Creator struct {
				Name string `json:"name"`
			} `json:"creator"`
		} `json:"data"`
	}
	if err := getJSON(fmt.Sprintf("https://games.roblox.com/v1/games?universeIds=%d", u.UniverseID), &games); err != nil || len(games.Data) == 0 {
		return Game{}, fmt.Errorf("no game for universe %d", u.UniverseID)
	}
	g := Game{Name: strings.TrimSpace(games.Data[0].Name), Creator: games.Data[0].Creator.Name}
	if g.Name == "" {
		g.Name = "Roblox"
	}
	var icons struct {
		Data []struct {
			ImageURL string `json:"imageUrl"`
			State    string `json:"state"`
		} `json:"data"`
	}
	if getJSON(fmt.Sprintf("https://thumbnails.roblox.com/v1/games/icons?universeIds=%d&size=512x512&format=Png&isCircular=false", u.UniverseID), &icons) == nil &&
		len(icons.Data) > 0 && icons.Data[0].State == "Completed" && strings.HasPrefix(icons.Data[0].ImageURL, "https://") {
		g.Icon = icons.Data[0].ImageURL
	}
	return g, nil
}
