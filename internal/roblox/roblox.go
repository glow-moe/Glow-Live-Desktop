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

// joinState is what the log has said so far: the newest join, unless a leave
// came after it.
type joinState struct {
	placeID int64
	at      time.Time
	ok      bool
}

// feed moves the state along the given log lines.
func (j *joinState) feed(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if m := joinRe.FindSubmatch(line); m != nil {
			id, err := strconv.ParseInt(string(m[1]), 10, 64)
			if err != nil {
				continue
			}
			j.placeID, j.ok = id, true
			j.at = time.Time{}
			if t := tsRe.FindSubmatch(line); t != nil {
				j.at, _ = time.Parse(time.RFC3339Nano, string(t[1]))
			}
			continue
		}
		if j.ok && leaveRe.Match(line) {
			j.ok, j.placeID = false, 0
		}
	}
}

// lastJoin scans a log for the newest join that wasn't followed by a leave.
func lastJoin(r io.Reader) (placeID int64, at time.Time, ok bool) {
	var j joinState
	j.feed(r)
	return j.placeID, j.at, j.ok
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

// logReader follows one log file: the first look reads its last few MB (logs
// grow to tens of MB), after that only the lines added since, so a tick every
// second or so costs a few KB, not a re-read.
type logReader struct {
	path   string
	offset int64
	state  joinState
}

const (
	firstLook = 4 << 20
	maxStep   = 8 << 20
)

func (l *logReader) read(path string) (joinState, error) {
	fh, err := os.Open(path)
	if err != nil {
		return joinState{}, err
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return joinState{}, err
	}
	size := st.Size()
	if path != l.path || size < l.offset {
		// A new session's log (or one that was cut short): start over.
		*l = logReader{path: path, offset: max(0, size-firstLook)}
	}
	if size-l.offset > maxStep {
		l.offset = size - maxStep
	}
	if size > l.offset {
		buf := make([]byte, size-l.offset)
		n, err := fh.ReadAt(buf, l.offset)
		if err != nil && err != io.EOF {
			return l.state, err
		}
		buf = buf[:n]
		// Only whole lines; a half-written one waits for the next look.
		if end := bytes.LastIndexByte(buf, '\n'); end >= 0 {
			l.state.feed(bytes.NewReader(buf[:end+1]))
			l.offset += int64(end + 1)
		}
	}
	return l.state, nil
}

var (
	mu        sync.Mutex
	infoCache = map[int64]Game{}
	missAt    = map[int64]time.Time{}
	fetching  = map[int64]bool{}

	logMu sync.Mutex
	logs  logReader
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
	logMu.Lock()
	j, err := logs.read(p)
	logMu.Unlock()
	if err != nil || !j.ok {
		return Game{}, false
	}
	g := lookup(j.placeID)
	g.PlaceID, g.JoinedAt = j.placeID, j.at
	return g, true
}

// lookup names a place from the cache. A miss starts the lookup in the
// background (the tick never waits on Roblox's API) and shows plain "Roblox"
// until it lands; a failed lookup is retried after a minute.
func lookup(placeID int64) Game {
	mu.Lock()
	defer mu.Unlock()
	if g, ok := infoCache[placeID]; ok {
		return g
	}
	if t, ok := missAt[placeID]; (ok && time.Since(t) < time.Minute) || fetching[placeID] {
		return Game{Name: "Roblox"}
	}
	fetching[placeID] = true
	go func() {
		g, err := fetchInfo(placeID)
		mu.Lock()
		defer mu.Unlock()
		delete(fetching, placeID)
		if err != nil {
			missAt[placeID] = time.Now()
			return
		}
		if len(infoCache) > 500 {
			infoCache = map[int64]Game{}
		}
		infoCache[placeID] = g
	}()
	return Game{Name: "Roblox"}
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
