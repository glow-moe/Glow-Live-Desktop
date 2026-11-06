package orchestrator

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/lcu"
	"github.com/glow-moe/glow-collector/internal/valorant"
)

// appValorant is VALORANT's own Discord application (from Discord's detectable
// games list, like the Steam table), so the headline reads "Playing VALORANT".
// A build can override it; so can glow.moe (settings.discordApps).
var appValorant = "700136079562375258"

// valorantNames come with the L!VE settings: map code -> name + splash, queue
// id -> mode. The site owns these tables; the app only uses them for Discord.
type valorantNames struct {
	Maps  map[string]struct{ Name, Splash string } `json:"maps"`
	Modes map[string]string                        `json:"modes"`
	// Agents: agent id -> name.
	Agents map[string]string `json:"agents"`
}

// valPush is the frame glow.moe gets: your Riot ID and the presence blob
// (trimmed to the site's valorantPick), mapped on the site, plus the match
// and the last scoreboard when the site turned those reads on.
type valPush struct {
	Game      string `json:"game"`
	Running   bool   `json:"running"`
	UpdatedAt int64  `json:"updatedAt"`
	Raw       struct {
		Name    string          `json:"name"`
		Tag     string          `json:"tag"`
		Private json.RawMessage `json:"private"`
		Match   *valorant.Match `json:"match,omitempty"`
		Final   *valorant.Final `json:"final,omitempty"`
		Party   *valorant.Party `json:"party,omitempty"`
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

func (n *valorantNames) agentOf(id string) string {
	if n == nil {
		return ""
	}
	return n.Agents[strings.ToLower(id)]
}

// Where the member is, which picks the Discord lines, the art and the clock.
const (
	valMenus  = "menus"
	valLobby  = "lobby"
	valQueue  = "queue"
	valCustom = "custom"
	valSelect = "select"
	valMatch  = "match"
)

func valorantStage(v *valorant.State) string {
	switch v.Phase {
	case "INGAME":
		return valMatch
	case "PREGAME":
		return valSelect
	}
	switch {
	case v.PartyState == "MATCHMAKING":
		return valQueue
	case v.PartyState == "CUSTOM_GAME_SETUP":
		return valCustom
	case v.PartySize > 1:
		return valLobby
	}
	return valMenus
}

// partyLine is "In queue · (2/5)".
func partyLine(label string, v *valorant.State) string {
	if v.PartySize > 0 && v.PartyMax > 0 {
		return label + " · (" + strconv.Itoa(v.PartySize) + "/" + strconv.Itoa(v.PartyMax) + ")"
	}
	return label
}

// Deathmatch has no teams: its "score" is your own kills against the leader,
// and Riot only refreshes it about once a minute.
func valorantScored(v *valorant.State) bool {
	return v.Phase == "INGAME" && v.Flow != "ShootingRange" && !strings.EqualFold(v.Queue, "deathmatch")
}

var valIDRE = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

const valNilID = "00000000-0000-0000-0000-000000000000"

// valorantMediaURL is an agent's or card's art on valorant-api.com, "" for an
// id that isn't one.
func valorantMediaURL(kind, id, file string) string {
	id = strings.ToLower(id)
	if !valIDRE.MatchString(id) || id == valNilID {
		return ""
	}
	return "https://media.valorant-api.com/" + kind + "/" + id + "/" + file
}

// valorantArt is the picture the status leads with: your agent once you've
// locked one, your player card in the menus. Empty means the map or the logo.
func valorantArt(v *valorant.State, n *valorantNames, stage, agent string) (art, text string) {
	if stage == valMatch || stage == valSelect {
		if art = valorantMediaURL("agents", agent, "displayicon.png"); art != "" {
			text = n.agentOf(agent)
		}
		return art, text
	}
	return valorantMediaURL("playercards", v.Card, "smallart.png"), ""
}

// valorantStatus is the Discord status for this tick, with its template vars.
func (o *Orchestrator) valorantStatus(v *valorant.State, n *valorantNames, username string) (discord.Activity, map[string]string) {
	stage := valorantStage(v)
	agent := o.valMatch.SelfAgent()
	since := o.valorantSince(stage)
	return valorantActivity(v, n, stage, agent, since, username), valorantVars(v, n, stage, agent)
}

// valorantSince is when this stage began: Riot publishes no match start, so
// the clock counts from when the app first saw it. Zero for a stage with no clock.
func (o *Orchestrator) valorantSince(stage string) time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	if stage != o.valStage {
		o.valStage, o.valStageAt = stage, time.Now()
	}
	if stage == valMatch || stage == valQueue {
		return o.valStageAt
	}
	return time.Time{}
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

func valorantActivity(v *valorant.State, n *valorantNames, stage, agent string, since time.Time, username string) discord.Activity {
	mapName, splash := n.mapOf(v.MapCode)
	mode := n.modeOf(v)
	details, state := "In the menus", valorant.RankName(v.Tier)
	switch stage {
	case valSelect:
		details, state = "Agent select", joinDots(mode, mapName)
	case valMatch:
		details = joinDots(mode, mapName)
		state = ""
		if valorantScored(v) {
			state = strconv.Itoa(v.AllyScore) + " - " + strconv.Itoa(v.FoeScore)
		}
	case valLobby:
		details, state = mode, partyLine("In lobby", v)
	case valQueue:
		details, state = mode, partyLine("In queue", v)
	case valCustom:
		details, state = "Custom game", partyLine("In lobby", v)
	}
	if state == "" && v.Level > 0 {
		state = "Level " + strconv.Itoa(v.Level)
	}
	large, largeText := glowIcon, "VALORANT"
	if splash != "" && v.Phase != "MENUS" {
		large, largeText = splash, mapName
	}
	if art, text := valorantArt(v, n, stage, agent); art != "" {
		large = art
		if text != "" {
			largeText = text
		}
	}
	a := discord.Activity{
		Details: atLeast2(details, "VALORANT"),
		State:   atLeast2(state, ""),
		Assets:  &discord.Assets{LargeImage: large, LargeText: largeText, SmallImage: glowIcon, SmallText: "glow.moe"},
	}
	if !since.IsZero() {
		a.Timestamps = &discord.Timestamps{Start: since.UnixMilli()}
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "VALORANT profile", URL: "https://glow.moe/" + username + "/valorant"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

func valorantVars(v *valorant.State, n *valorantNames, stage, agent string) map[string]string {
	mapName, splash := n.mapOf(v.MapCode)
	vars := map[string]string{
		"phase": v.Phase, "queue": v.Queue, "mode": n.modeOf(v), "map": mapName, "mapCode": v.MapCode,
		"splash": splash, "rank": valorant.RankName(v.Tier), "level": itoa(v.Level), "name": v.Name,
	}
	if valorantScored(v) {
		vars["ally"], vars["enemy"] = strconv.Itoa(v.AllyScore), strconv.Itoa(v.FoeScore)
	}
	vars["stage"] = stage
	vars["art"], vars["artText"] = valorantArt(v, n, stage, agent)
	vars["agent"] = vars["artText"]
	if v.PartySize > 0 {
		vars["party"], vars["partyMax"] = strconv.Itoa(v.PartySize), strconv.Itoa(v.PartyMax)
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
