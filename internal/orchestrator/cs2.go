package orchestrator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/cs2"
	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/proc"
	"github.com/glow-moe/glow-collector/internal/steam"
)

// cs2Push is the frame glow.moe gets: the GSI blocks without Steam ids,
// mapped on the site (src/lib/cs2/gsi.ts there).
type cs2Push struct {
	Game      string          `json:"game"`
	Live      bool            `json:"live"`
	UpdatedAt int64           `json:"updatedAt"`
	Raw       json.RawMessage `json:"raw"`
}

// ensureCS2 installs the GSI cfg and starts the receiver, retrying every 5
// minutes until CS2 is installed (and the port is free).
func (o *Orchestrator) ensureCS2() *cs2.Receiver {
	o.mu.Lock()
	r, tried := o.cs2Recv, o.cs2TriedAt
	o.mu.Unlock()
	if r != nil || time.Since(tried) < 5*time.Minute {
		return r
	}
	o.mu.Lock()
	o.cs2TriedAt = time.Now()
	o.mu.Unlock()
	token, err := cs2.Install()
	if err != nil || token == "" {
		return nil
	}
	if r, err = cs2.Listen(token); err != nil {
		return nil
	}
	o.mu.Lock()
	o.cs2Recv = r
	o.mu.Unlock()
	return r
}

// cs2Own keeps the member's own side and K/D/A while CS2 sends a teammate
// they spectate after dying (tick goroutine only).
func (o *Orchestrator) cs2Own(s cs2.Summary) cs2.Summary {
	if s.Self {
		o.cs2Last = s
		return s
	}
	if o.cs2Last.Map == s.Map && o.cs2Last.Side != "" {
		s.Side, s.Kills, s.Deaths, s.Assists = o.cs2Last.Side, o.cs2Last.Kills, o.cs2Last.Deaths, o.cs2Last.Assists
		if s.Side == "T" { // Summarize put CT first without a side
			s.Mine, s.Theirs = s.Theirs, s.Mine
		}
	}
	return s
}

var cs2Maps = map[string]string{
	"dust2": "Dust II", "mirage": "Mirage", "inferno": "Inferno", "nuke": "Nuke", "ancient": "Ancient", "anubis": "Anubis",
	"vertigo": "Vertigo", "overpass": "Overpass", "train": "Train", "office": "Office", "italy": "Italy",
}

var cs2Modes = map[string]string{
	"competitive": "Competitive", "premier": "Premier", "casual": "Casual", "deathmatch": "Deathmatch",
	"scrimcomp2v2": "Wingman", "gungameprogressive": "Arms Race", "custom": "Custom", "training": "Training",
}

func cs2MapName(raw string) string {
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		raw = raw[i+1:]
	}
	for _, p := range []string{"de_", "cs_", "ar_", "dz_"} {
		raw = strings.TrimPrefix(raw, p)
	}
	if n, ok := cs2Maps[raw]; ok {
		return n
	}
	words := strings.Split(raw, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// cs2MapArts are the maps glow.moe has a screenshot of (public/cs2/maps there,
// taken from the game's own files).
var cs2MapArts = map[string]bool{
	"ar_baggage": true, "ar_pool_day": true, "ar_shoots": true, "ar_shoots_night": true, "cs_italy": true, "cs_office": true,
	"cs_shelter": true, "de_ancient": true, "de_ancient_night": true, "de_anubis": true, "de_boulder": true, "de_cache": true,
	"de_debris": true, "de_dust": true, "de_dust2": true, "de_eldorado": true, "de_fachwerk": true, "de_inferno": true,
	"de_mirage": true, "de_nuke": true, "de_overpass": true, "de_poseidon": true, "de_train": true, "de_vertigo": true,
}

// cs2MapArt is the map's screenshot on glow.moe for the Discord status, "" for
// a map without one (workshop maps).
func cs2MapArt(raw string) string {
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		raw = raw[i+1:]
	}
	if !cs2MapArts[raw] {
		return ""
	}
	return "https://glow.moe/cs2/maps/" + raw + ".jpg"
}

func cs2ModeName(m string) string {
	if n, ok := cs2Modes[m]; ok {
		return n
	}
	return "Counter-Strike 2"
}

func cs2Detail(s cs2.Summary) string {
	return fmt.Sprintf("CS2 · %s · %d–%d · %d/%d/%d", cs2MapName(s.Map), s.Mine, s.Theirs, s.Kills, s.Deaths, s.Assists)
}

// cs2Activity is the built-in Discord status; the site can re-word it ("cs2").
func cs2Activity(s cs2.Summary, username string) discord.Activity {
	details := fmt.Sprintf("%s · %d – %d", cs2MapName(s.Map), s.Mine, s.Theirs)
	state := fmt.Sprintf("%d / %d / %d  ·  %s", s.Kills, s.Deaths, s.Assists, cs2ModeName(s.Mode))
	switch s.Phase {
	case "warmup":
		details = cs2MapName(s.Map) + " · Warmup"
		state = cs2ModeName(s.Mode)
	case "gameover":
		switch {
		case s.Mine > s.Theirs:
			details = fmt.Sprintf("%s · Won %d – %d", cs2MapName(s.Map), s.Mine, s.Theirs)
		case s.Mine < s.Theirs:
			details = fmt.Sprintf("%s · Lost %d – %d", cs2MapName(s.Map), s.Mine, s.Theirs)
		}
	}
	large := cs2MapArt(s.Map)
	if large == "" {
		large = steam.CoverImage(cs2.AppID)
	}
	if large == "" {
		large = glowIcon
	}
	a := discord.Activity{
		Details: details,
		State:   state,
		Assets:  &discord.Assets{LargeImage: large, LargeText: "Counter-Strike 2", SmallImage: glowIcon, SmallText: "glow.moe"},
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "Live match", URL: "https://glow.moe/" + username + "/cs2"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

func cs2Vars(s cs2.Summary) map[string]string {
	return map[string]string{
		"map": s.Map, "mapName": cs2MapName(s.Map), "mode": s.Mode, "modeName": cs2ModeName(s.Mode), "phase": s.Phase,
		"round": itoa(s.Round), "side": s.Side, "mine": strconv.Itoa(s.Mine), "theirs": strconv.Itoa(s.Theirs),
		"kills": strconv.Itoa(s.Kills), "deaths": strconv.Itoa(s.Deaths), "assists": strconv.Itoa(s.Assists),
	}
}

// pcPayload is a game spotted by its exe outside Steam (the site's
// lib/live/pc-games names which); glow.moe keeps the clock and finds the art.
type pcPayload struct {
	Game      string `json:"game"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updatedAt"`
}

// pcGame is the first running game from the site's list (exe -> name), read
// from the process list at most every 5 seconds.
func (o *Orchestrator) pcGame(list map[string]string) (string, bool) {
	if len(list) == 0 {
		return "", false
	}
	if time.Since(o.pcScanAt) >= 5*time.Second {
		o.pcScanAt = time.Now()
		o.pcFound = ""
		running := proc.Names()
		best := ""
		for exe, name := range list {
			// Map order is random: take the first exe alphabetically so two games
			// open at once don't swap places between scans.
			if running[strings.ToLower(exe)] && (best == "" || exe < best) {
				best, o.pcFound = exe, name
			}
		}
	}
	return o.pcFound, o.pcFound != ""
}
