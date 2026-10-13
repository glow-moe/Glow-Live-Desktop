// Package dota receives Dota 2's Game State Integration. A cfg file in the
// game's gamestate_integration folder makes Dota POST its state to a loopback
// port here; the app keeps the newest frame and forwards the useful blocks to
// glow.moe as they are (the site maps them, so the app stays a thin pipe).
//
// Dota only reads the cfg when launched with -gamestateintegration (a Steam
// launch option the player adds once); the app can't set that for them.
package dota

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

	"github.com/glow-moe/glow-collector/internal/steam"
)

// AppID is Dota 2 on Steam.
const AppID = 570

// Addr is the fixed loopback address the cfg points the game at.
const Addr = "127.0.0.1:47101"

const cfgName = "gamestate_integration_glow.cfg"

// keep are the GSI blocks sent on to glow.moe. "auth" (our token) and the
// "previously"/"added" diffs never leave the app.
var keep = map[string]bool{
	"provider": true, "map": true, "player": true, "hero": true, "abilities": true,
	"items": true, "draft": true, "buildings": true, "roshan": true, "events": true,
}

var tokenRE = regexp.MustCompile(`"token"\s+"([a-f0-9]{32})"`)

// Install writes (or refreshes) the GSI cfg in the Dota install and returns
// the token Dota will send. "" + nil error means Dota isn't installed.
func Install() (string, error) {
	dir := steam.InstallDir(AppID)
	if dir == "" {
		return "", nil
	}
	cfgDir := filepath.Join(dir, "game", "dota", "cfg", "gamestate_integration")
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
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return "", err
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
		"provider"	"1"
		"map"		"1"
		"player"	"1"
		"hero"		"1"
		"abilities"	"1"
		"items"		"1"
		"draft"		"1"
		"buildings"	"1"
		"roshan"	"1"
		"events"	"1"
		"wearables"	"0"
		"minimap"	"0"
		"couriers"	"0"
		"neutralitems"	"0"
	}
	"auth"
	{
		"token"	"%s"
	}
}
`, Addr, token)
}

// Receiver holds the newest frame Dota sent.
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

// accept checks the token and keeps only the blocks glow.moe gets.
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
	out := map[string]json.RawMessage{}
	for k, v := range in {
		if keep[k] {
			out[k] = v
		}
	}
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
	Playing bool   // in a match you play (not menus, not spectating)
	Hero    string // internal key, "juggernaut"
	Level   int
	Kills   int
	Deaths  int
	Assists int
	Clock   int // seconds, negative before the horn
	Draft   bool
}

// Summarize reads a frame's basics.
func Summarize(frame json.RawMessage) Summary {
	var f struct {
		Map struct {
			GameState string `json:"game_state"`
			ClockTime int    `json:"clock_time"`
		} `json:"map"`
		Player struct {
			Activity string `json:"activity"`
			Kills    int    `json:"kills"`
			Deaths   int    `json:"deaths"`
			Assists  int    `json:"assists"`
		} `json:"player"`
		Hero struct {
			Name  string `json:"name"`
			Level int    `json:"level"`
		} `json:"hero"`
	}
	if json.Unmarshal(frame, &f) != nil {
		return Summary{}
	}
	s := Summary{Kills: f.Player.Kills, Deaths: f.Player.Deaths, Assists: f.Player.Assists, Level: f.Hero.Level, Clock: f.Map.ClockTime}
	if len(f.Hero.Name) > len("npc_dota_hero_") && f.Hero.Name[:len("npc_dota_hero_")] == "npc_dota_hero_" {
		s.Hero = f.Hero.Name[len("npc_dota_hero_"):]
	}
	switch f.Map.GameState {
	case "DOTA_GAMERULES_STATE_HERO_SELECTION", "DOTA_GAMERULES_STATE_STRATEGY_TIME", "DOTA_GAMERULES_STATE_WAIT_FOR_PLAYERS_TO_LOAD":
		s.Draft = true
		s.Playing = f.Player.Activity == "playing"
	case "DOTA_GAMERULES_STATE_PRE_GAME", "DOTA_GAMERULES_STATE_GAME_IN_PROGRESS", "DOTA_GAMERULES_STATE_TEAM_SHOWCASE":
		s.Playing = f.Player.Activity == "playing"
	}
	return s
}
