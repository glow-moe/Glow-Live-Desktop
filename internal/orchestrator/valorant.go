package orchestrator

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/lcu"
	"github.com/glow-moe/glow-collector/internal/valorant"
)

// appValorant is a Discord app named "VALORANT" (build-time, like the others);
// unset means the glow app carries the status.
var appValorant = ""

// valorantNames come with the L!VE settings: map code -> name + splash, queue
// id -> mode. The site owns these tables; the app only uses them for Discord.
type valorantNames struct {
	Maps  map[string]struct{ Name, Splash string } `json:"maps"`
	Modes map[string]string                        `json:"modes"`
}

// valPush is the frame glow.moe gets: your Riot ID and the presence blob
// (trimmed to the site's valorantPick), mapped on the site.
type valPush struct {
	Game      string `json:"game"`
	Running   bool   `json:"running"`
	UpdatedAt int64  `json:"updatedAt"`
	Raw       struct {
		Name    string          `json:"name"`
		Tag     string          `json:"tag"`
		Private json.RawMessage `json:"private"`
	} `json:"raw"`
}

// valorantState reads VALORANT at most every 3s (10s after a "not running"),
// since the Riot Client is also up whenever League is.
func (o *Orchestrator) valorantState() *valorant.State {
	o.mu.Lock()
	last, at := o.valLast, o.valAt
	o.mu.Unlock()
	wait := 3 * time.Second
	if last == nil {
		wait = 10 * time.Second
	}
	if !at.IsZero() && time.Since(at) < wait {
		return last
	}
	st, err := valorant.Fetch()
	if err != nil {
		st = nil
	}
	o.mu.Lock()
	o.valLast, o.valAt = st, time.Now()
	o.mu.Unlock()
	return st
}

func (n *valorantNames) mapOf(code string) (name, splash string) {
	if n != nil {
		if m, ok := n.Maps[code]; ok {
			return m.Name, m.Splash
		}
	}
	return code, ""
}

func (n *valorantNames) modeOf(v *valorant.State) string {
	if v.Flow == "ShootingRange" {
		return "The Range"
	}
	if n != nil {
		if m, ok := n.Modes[strings.ToLower(v.Queue)]; ok {
			return m
		}
	}
	if v.Queue == "" {
		return "Custom"
	}
	return strings.ToUpper(v.Queue[:1]) + v.Queue[1:]
}

func valorantDetail(v *valorant.State, n *valorantNames) string {
	mapName, _ := n.mapOf(v.MapCode)
	switch v.Phase {
	case "INGAME":
		return joinDots(n.modeOf(v), mapName)
	case "PREGAME":
		return "Agent select · " + mapName
	}
	return "In the menus"
}

func valorantActivity(v *valorant.State, n *valorantNames, username string) discord.Activity {
	mapName, splash := n.mapOf(v.MapCode)
	mode := n.modeOf(v)
	rank := valorant.RankName(v.Tier)
	details, state := "In the menus", rank
	switch v.Phase {
	case "PREGAME":
		details, state = "Agent select", joinDots(mode, mapName)
	case "INGAME":
		details = joinDots(mode, mapName)
		state = ""
		if v.Flow != "ShootingRange" {
			state = strconv.Itoa(v.AllyScore) + " - " + strconv.Itoa(v.FoeScore)
		}
	}
	if state == "" && v.Level > 0 {
		state = "Level " + strconv.Itoa(v.Level)
	}
	large, largeText := glowIcon, "VALORANT"
	if splash != "" && v.Phase != "MENUS" {
		large, largeText = splash, mapName
	}
	a := discord.Activity{
		Details: atLeast2(details, "VALORANT"),
		State:   atLeast2(state, ""),
		Assets:  &discord.Assets{LargeImage: large, LargeText: largeText, SmallImage: glowIcon, SmallText: "glow.moe"},
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "VALORANT profile", URL: "https://glow.moe/" + username + "/valorant"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

func valorantVars(v *valorant.State, n *valorantNames) map[string]string {
	mapName, splash := n.mapOf(v.MapCode)
	vars := map[string]string{
		"phase": v.Phase, "queue": v.Queue, "mode": n.modeOf(v), "map": mapName, "mapCode": v.MapCode,
		"splash": splash, "rank": valorant.RankName(v.Tier), "level": itoa(v.Level), "name": v.Name,
	}
	if v.Phase == "INGAME" && v.Flow != "ShootingRange" {
		vars["ally"], vars["enemy"] = strconv.Itoa(v.AllyScore), strconv.Itoa(v.FoeScore)
	}
	return vars
}

// valorantFrame builds the push for a VALORANT state.
func valorantFrame(v *valorant.State, pick []string) valPush {
	p := valPush{Game: "valorant", Running: true, UpdatedAt: time.Now().UnixMilli()}
	p.Raw.Name, p.Raw.Tag = v.Name, v.Tag
	p.Raw.Private = lcu.PickFields(v.Private, pick)
	return p
}
