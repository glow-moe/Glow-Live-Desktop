// Package cs2 receives Counter-Strike 2's Game State Integration, the same way
// package dota does for Dota 2: a cfg file in the game's cfg folder makes CS2
// POST its state to a loopback port here, and the app forwards the useful
// blocks to glow.moe as they are (the site maps them, src/lib/cs2/gsi.ts).
//
// Unlike Dota, CS2 reads gamestate_integration_*.cfg on every start without a
// launch option. The Steam ids in a frame never leave the app: the player
// block is marked `self` when it's the member's own (not a teammate they
// spectate while dead) and the ids are dropped.
package cs2

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/jsonx"
	"github.com/glow-moe/glow-collector/internal/steam"
)

// AppID is Counter-Strike 2 on Steam.
const AppID = 730

// Addr is the fixed loopback address the cfg points the game at (Dota has 47101).
const Addr = "127.0.0.1:47102"

const cfgName = "gamestate_integration_glow.cfg"

// keep are the GSI blocks sent on to glow.moe; "provider" (your Steam id) and
// "auth" (our token) never leave the app, nor do the "previously"/"added" diffs.
var keep = map[string]bool{"map": true, "round": true, "player": true, "phase_countdowns": true}

// playerKeep are the player block's fields sent on: no Steam id, no name or clan.
var playerKeep = map[string]bool{"team": true, "activity": true, "state": true, "match_stats": true, "weapons": true}

var tokenRE = regexp.MustCompile(`"token"\s+"([a-f0-9]{32})"`)

// Install writes (or refreshes) the GSI cfg in the CS2 install and returns the
// token CS2 will send. "" + nil error means CS2 isn't installed.
func Install() (string, error) {
	dir := steam.InstallDir(AppID)
	if dir == "" {
		return "", nil
	}
	cfgDir := filepath.Join(dir, "game", "csgo", "cfg")
	if _, err := os.Stat(cfgDir); err != nil {
		return "", nil
	}
	path := filepath.Join(cfgDir, cfgName)
	token := ""
	if b, err := os.ReadFile(path); err == nil {
		if m := tokenRE.FindSubmatch(b); m != nil {
			token = string(m[1])
		}
	}
	if token == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		token = hex.EncodeToString(buf)
	}
	want := cfgText(token)
	if b, err := os.ReadFile(path); err == nil && string(b) == want {
		return token, nil
	}
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		return "", err
	}
	return token, nil
}

func cfgText(token string) string {
	return fmt.Sprintf(`"glow L!VE"
{
	"uri"		"http://%s/"
	"timeout"	"5.0"
	"buffer"	"0.5"
	"throttle"	"1.0"
	"heartbeat"	"10.0"
	"data"
	{
		"provider"			"1"
		"map"				"1"
		"round"				"1"
		"player_id"			"1"
		"player_state"		"1"
		"player_match_stats"	"1"
		"player_weapons"	"1"
		"phase_countdowns"	"1"
	}
	"auth"
	{
		"token"	"%s"
	}
}
`, Addr, token)
}

// Receiver holds the newest frame CS2 sent.
type Receiver struct {
	token string
	mu    sync.Mutex
	frame json.RawMessage
	at    time.Time
}

// Listen starts the loopback receiver for frames carrying token.
func Listen(token string) (*Receiver, error) {
	if token == "" {
		return nil, errors.New("no token")
	}
	ln, err := net.Listen("tcp", Addr)
	if err != nil {
		return nil, err
	}
	r := &Receiver{token: token}
	srv := &http.Server{Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return r, nil
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "post only", http.StatusMethodNotAllowed)
		return
	}
	b, err := io.ReadAll(io.LimitReader(req.Body, 1<<20+1))
	if err != nil || len(b) > 1<<20 {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	frame, err := r.accept(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	r.mu.Lock()
	r.frame, r.at = frame, time.Now()
	r.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// accept checks the token, keeps the blocks glow.moe gets and drops the ids.
func (r *Receiver) accept(b []byte) (json.RawMessage, error) {
	var in map[string]json.RawMessage
	if json.Unmarshal(b, &in) != nil {
		return nil, errors.New("not json")
	}
	var auth struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(in["auth"], &auth) != nil || auth.Token != r.token {
		return nil, errors.New("bad token")
	}
	var provider, who struct {
		SteamID string `json:"steamid"`
	}
	_ = json.Unmarshal(in["provider"], &provider)
	_ = json.Unmarshal(in["player"], &who)
	out := map[string]json.RawMessage{}
	for k, v := range in {
		if keep[k] {
			out[k] = v
		}
	}
	if p, ok := out["player"]; ok {
		var fields map[string]json.RawMessage
		if json.Unmarshal(p, &fields) == nil {
			slim := map[string]json.RawMessage{}
			for k, v := range fields {
				if playerKeep[k] {
					slim[k] = v
				}
			}
			out["player"], _ = json.Marshal(slim)
		} else {
			delete(out, "player")
		}
	}
	self := provider.SteamID != "" && provider.SteamID == who.SteamID
	out["self"], _ = json.Marshal(self)
	return json.Marshal(out)
}

// Latest is the newest frame if it is younger than maxAge.
func (r *Receiver) Latest(maxAge time.Duration) (json.RawMessage, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frame == nil || time.Since(r.at) > maxAge {
		return nil, false
	}
	return r.frame, true
}

// Summary is what the app itself reads from a frame (status line + Discord).
type Summary struct {
	Playing bool   // a match on a map (not the menus)
	Map     string // "de_dust2"
	Mode    string // "competitive"
	Phase   string // warmup | live | intermission | gameover
	Round   int
	Side    string // CT | T, "" while spectating someone else
	Mine    int    // your side's score
	Theirs  int
	Self    bool
	Kills   int
	Deaths  int
	Assists int
	// RoundKills is this round's kills (5 in a 5v5 mode is an ace).
	RoundKills int
}

// Summarize reads a frame's basics.
func Summarize(frame json.RawMessage) Summary {
	var f struct {
		Self bool `json:"self"`
		Map  struct {
			Mode   string `json:"mode"`
			Name   string `json:"name"`
			Phase  string `json:"phase"`
			Round  int    `json:"round"`
			TeamCT struct {
				Score int `json:"score"`
			} `json:"team_ct"`
			TeamT struct {
				Score int `json:"score"`
			} `json:"team_t"`
		} `json:"map"`
		Player struct {
			Team  string `json:"team"`
			State struct {
				RoundKills int `json:"round_kills"`
			} `json:"state"`
			Stats struct {
				Kills   int `json:"kills"`
				Deaths  int `json:"deaths"`
				Assists int `json:"assists"`
			} `json:"match_stats"`
		} `json:"player"`
	}
	if jsonx.Unmarshal(frame, &f) != nil {
		return Summary{}
	}
	s := Summary{Map: f.Map.Name, Mode: f.Map.Mode, Phase: f.Map.Phase, Round: f.Map.Round, Self: f.Self}
	switch f.Map.Phase {
	case "warmup", "live", "intermission", "gameover":
		s.Playing = true
	}
	if f.Self {
		s.Side = f.Player.Team
		s.Kills, s.Deaths, s.Assists = f.Player.Stats.Kills, f.Player.Stats.Deaths, f.Player.Stats.Assists
		s.RoundKills = f.Player.State.RoundKills
	}
	switch s.Side {
	case "CT":
		s.Mine, s.Theirs = f.Map.TeamCT.Score, f.Map.TeamT.Score
	case "T":
		s.Mine, s.Theirs = f.Map.TeamT.Score, f.Map.TeamCT.Score
	default:
		s.Mine, s.Theirs = f.Map.TeamCT.Score, f.Map.TeamT.Score
	}
	return s
}
