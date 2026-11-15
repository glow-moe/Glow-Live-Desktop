// Package rocketleague reads Rocket League's Stats API: Psyonix's own local
// feed (rocketleague.com/developer/stats-api), switched on in the game's
// TAGame/Config/DefaultStatsAPI.ini and served on 127.0.0.1:49123 as a stream
// of JSON messages {"Event": ..., "Data": ...} during a match. The app sends
// glow.moe the score, the clock and the member's own numbers, never the other
// players' names.
package rocketleague

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/steam"
)

// SteamAppID is the Steam release; most players have the Epic one.
const SteamAppID = 252950

// Addr is where the game listens once the Stats API is on.
const Addr = "127.0.0.1:49123"

// Match is what the app sends: one match, the member's side first.
type Match struct {
	GUID     string `json:"guid,omitempty"`
	Arena    string `json:"arena"`
	Mine     int    `json:"mine"`
	Theirs   int    `json:"theirs"`
	Clock    int    `json:"clock"` // seconds left
	Overtime bool   `json:"overtime,omitempty"`
	Ended    bool   `json:"ended,omitempty"`
	Won      bool   `json:"won,omitempty"`
	Goals    int    `json:"goals"`
	Shots    int    `json:"shots"`
	Saves    int    `json:"saves"`
	Assists  int    `json:"assists"`
	Demos    int    `json:"demos"`
	Score    int    `json:"score"`
}

// InstallDir finds the game: the Epic launcher's manifests, then Steam.
func InstallDir() string {
	if runtime.GOOS == "windows" {
		dir := filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests")
		items, _ := filepath.Glob(filepath.Join(dir, "*.item"))
		for _, p := range items {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var m struct {
				DisplayName     string `json:"DisplayName"`
				InstallLocation string `json:"InstallLocation"`
			}
			if json.Unmarshal(b, &m) == nil && strings.EqualFold(m.DisplayName, "Rocket League") && m.InstallLocation != "" {
				return m.InstallLocation
			}
		}
	}
	return steam.InstallDir(SteamAppID)
}

// Install turns the Stats API on (10 updates a second on Addr's port) in the
// game's config. changed is true when it wrote the file: the game reads it
// when it starts, so a running game needs a restart once.
func Install() (changed bool, err error) {
	dir := InstallDir()
	if dir == "" {
		return false, nil
	}
	cfgDir := filepath.Join(dir, "TAGame", "Config")
	path := filepath.Join(cfgDir, "TAStatsAPI.ini")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(cfgDir, "DefaultStatsAPI.ini")
	}
	old, _ := os.ReadFile(path)
	next := withStatsAPI(string(old))
	if next == string(old) {
		return false, nil
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(next), 0o644)
}

// withStatsAPI sets PacketSendRate and Port in the exporter's section, keeping
// everything else in the file as it was.
func withStatsAPI(ini string) string {
	const section = "[TAGame.MatchStatsExporter_TA]"
	want := map[string]string{"PacketSendRate": "10", "Port": "49123"}
	nl := "\n"
	if strings.Contains(ini, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(ini, "\r\n", "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), section) {
			start = i
			break
		}
	}
	if start < 0 {
		out := strings.TrimRight(strings.ReplaceAll(ini, "\r\n", "\n"), "\n")
		if out != "" {
			out += "\n\n"
		}
		out += section + "\nPacketSendRate=10\nPort=49123\n"
		return strings.ReplaceAll(out, "\n", nl)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			end = i
			break
		}
	}
	seen := map[string]bool{}
	for i := start + 1; i < end; i++ {
		k, _, ok := strings.Cut(lines[i], "=")
		k = strings.TrimSpace(k)
		if v, has := want[k]; ok && has {
			lines[i] = k + "=" + v
			seen[k] = true
		}
	}
	var add []string
	for _, k := range []string{"PacketSendRate", "Port"} {
		if !seen[k] {
			add = append(add, k+"="+want[k])
		}
	}
	rest := append([]string{}, lines[start+1:]...)
	lines = append(append(lines[:start+1], add...), rest...)
	return strings.Join(lines, nl)
}

// Reader keeps the newest match from the feed.
type Reader struct {
	mu    sync.Mutex
	match Match
	at    time.Time
	me    string // the member's name in this match: the car the camera follows
	guid  string
}

// Run connects while the game is up and reads until it closes; call it again
// after it returns (the caller retries every few seconds).
func (r *Reader) Run() {
	c, err := net.DialTimeout("tcp", Addr, 2*time.Second)
	if err != nil {
		return
	}
	defer c.Close()
	dec := json.NewDecoder(bufio.NewReader(c))
	for {
		var msg struct {
			Event string          `json:"Event"`
			Data  json.RawMessage `json:"Data"`
		}
		_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
		if err := dec.Decode(&msg); err != nil {
			return
		}
		data := msg.Data
		// Some builds send Data as a JSON string holding the object.
		var inner string
		if json.Unmarshal(data, &inner) == nil {
			data = json.RawMessage(inner)
		}
		r.handle(msg.Event, data)
	}
}

type player struct {
	Name    string `json:"Name"`
	TeamNum int    `json:"TeamNum"`
	Score   int    `json:"Score"`
	Goals   int    `json:"Goals"`
	Shots   int    `json:"Shots"`
	Assists int    `json:"Assists"`
	Saves   int    `json:"Saves"`
	Demos   int    `json:"Demos"`
}

type updateState struct {
	MatchGUID string   `json:"MatchGuid"`
	Players   []player `json:"Players"`
	Game      struct {
		Teams []struct {
			TeamNum int `json:"TeamNum"`
			Score   int `json:"Score"`
		} `json:"Teams"`
		TimeSeconds int    `json:"TimeSeconds"`
		Overtime    bool   `json:"bOvertime"`
		Replay      bool   `json:"bReplay"`
		HasWinner   bool   `json:"bHasWinner"`
		Arena       string `json:"Arena"`
		HasTarget   bool   `json:"bHasTarget"`
		Target      struct {
			Name string `json:"Name"`
		} `json:"Target"`
	} `json:"Game"`
}

func (r *Reader) handle(event string, data json.RawMessage) {
	switch event {
	case "UpdateState":
		var d updateState
		if json.Unmarshal(data, &d) != nil {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if d.MatchGUID != r.guid {
			r.guid, r.me = d.MatchGUID, ""
		}
		// The camera follows the member's car while they play; the first car
		// it follows in a match (outside a replay) is theirs.
		if r.me == "" && d.Game.HasTarget && !d.Game.Replay && d.Game.Target.Name != "" {
			r.me = d.Game.Target.Name
		}
		r.match, r.at = summarize(d, r.me), time.Now()
	case "MatchDestroyed":
		r.mu.Lock()
		r.match, r.at, r.me, r.guid = Match{}, time.Time{}, "", ""
		r.mu.Unlock()
	}
}

// summarize is one UpdateState as the member sees it (their team's score
// first, their own numbers); blue first while it isn't known who they are.
func summarize(d updateState, me string) Match {
	m := Match{GUID: d.MatchGUID, Arena: d.Game.Arena, Clock: d.Game.TimeSeconds, Overtime: d.Game.Overtime, Ended: d.Game.HasWinner}
	team := 0
	for _, p := range d.Players {
		if me != "" && p.Name == me {
			team = p.TeamNum
			m.Goals, m.Shots, m.Saves, m.Assists, m.Demos, m.Score = p.Goals, p.Shots, p.Saves, p.Assists, p.Demos, p.Score
		}
	}
	for _, t := range d.Game.Teams {
		if t.TeamNum == team {
			m.Mine = t.Score
		} else {
			m.Theirs = t.Score
		}
	}
	m.Won = m.Ended && m.Mine > m.Theirs
	return m
}

// Latest is the newest match if it is younger than maxAge.
func (r *Reader) Latest(maxAge time.Duration) (Match, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.at.IsZero() || time.Since(r.at) > maxAge {
		return Match{}, false
	}
	return r.match, true
}
