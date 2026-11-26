package orchestrator

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/fortnite"
)

// fortniteSnap is the frame glow.moe gets: what the log said, mapped on the
// site. The game server's address never leaves this PC.
type fortniteSnap struct {
	Game      string         `json:"game"`
	InMatch   bool           `json:"inMatch"`
	Map       string         `json:"map,omitempty"`
	Playlist  string         `json:"playlist,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Status    string         `json:"status,omitempty"`
	Island    string         `json:"island,omitempty"`
	StartedAt int64          `json:"startedAt,omitempty"`
	Last      *fortniteFinal `json:"last,omitempty"`
	Loadout   *fortniteGear  `json:"loadout,omitempty"`
	Name      string         `json:"name,omitempty"`
	ID        string         `json:"id,omitempty"`
	Party     []fortniteMate `json:"party,omitempty"`
	Stw       *fortniteStw   `json:"stw,omitempty"`
	UpdatedAt int64          `json:"updatedAt"`
}

// fortniteStw is Save the World from the log: the mission's map and kind, the
// wave, the three medals, and the last finished mission's scores and XP.
type fortniteStw struct {
	Map     string            `json:"map,omitempty"`
	Mission string            `json:"mission,omitempty"`
	Wave    int               `json:"wave,omitempty"`
	Medals  []fortniteMedal   `json:"medals,omitempty"`
	Last    *fortniteStwFinal `json:"last,omitempty"`
}

type fortniteMedal struct {
	Kind  string `json:"kind"`
	Score int    `json:"score"`
	Tier  int    `json:"tier"`
	Next  int    `json:"next,omitempty"`
}

type fortniteStwFinal struct {
	Map     string             `json:"map,omitempty"`
	Mission string             `json:"mission,omitempty"`
	Players []fortniteStwScore `json:"players"`
	XP      int                `json:"xp,omitempty"`
	Level   int                `json:"level,omitempty"`
	LevelUp int                `json:"levelUp,omitempty"`
	EndedAt int64              `json:"endedAt,omitempty"`
}

type fortniteStwScore struct {
	Name     string `json:"name"`
	Me       bool   `json:"me,omitempty"`
	Combat   int    `json:"combat"`
	Building int    `json:"building"`
	Utility  int    `json:"utility"`
	Total    int    `json:"total"`
}

// stwPayload is the frame's Save the World part, nil outside it (and the last
// mission once it's 3 hours old).
func stwPayload(f fortnite.State) *fortniteStw {
	if _, ok := stwPlace(f); !ok {
		return nil
	}
	out := &fortniteStw{Map: f.Stw.Map, Mission: f.Stw.Mission, Wave: f.Stw.Wave}
	for i, m := range f.Stw.Medals {
		if m.Score > 0 {
			out.Medals = append(out.Medals, fortniteMedal{Kind: fortnite.StwMedalKinds[i], Score: m.Score, Tier: m.Tier, Next: m.Next})
		}
	}
	if f.StwLast != nil && time.Since(f.StwLast.EndedAt) < 3*time.Hour {
		l, xp := f.StwLast.Mine(f.ID, f.Name)
		fin := &fortniteStwFinal{Map: l.Map, Mission: l.Mission, XP: xp.XP, Level: xp.Level, LevelUp: xp.LevelUp, EndedAt: ms(l.EndedAt)}
		for _, p := range l.Players {
			fin.Players = append(fin.Players, fortniteStwScore{p.Name, p.Me, p.Combat, p.Building, p.Utility, p.Total})
		}
		out.Last = fin
	}
	return out
}

// fortniteMate is someone else in the lobby; the site names them by id.
type fortniteMate struct {
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}

// fortniteGear is what you have equipped, as Fortnite item ids.
type fortniteGear struct {
	Character string `json:"character,omitempty"`
	Backpack  string `json:"backpack,omitempty"`
	Pickaxe   string `json:"pickaxe,omitempty"`
	Glider    string `json:"glider,omitempty"`
	Contrail  string `json:"contrail,omitempty"`
	Pet       string `json:"pet,omitempty"`
	PetName   string `json:"petName,omitempty"`
}

type fortniteFinal struct {
	Playlist  string `json:"playlist,omitempty"`
	Status    string `json:"status,omitempty"`
	Left      int    `json:"left,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
	EndedAt   int64  `json:"endedAt,omitempty"`
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fortnitePayload(f fortnite.State) fortniteSnap {
	p := fortniteSnap{Game: "fortnite", InMatch: f.InMatch, Map: f.Map, Playlist: f.Playlist, Phase: f.Phase, Status: f.Status, Island: f.Island, StartedAt: ms(f.JoinedAt), Name: f.Name, ID: f.ID, UpdatedAt: time.Now().UnixMilli()}
	for _, m := range f.Others() {
		p.Party = append(p.Party, fortniteMate{m.Name, m.ID})
	}
	if g := f.Loadout; g.Character != "" {
		p.Loadout = &fortniteGear{g.Character, g.Backpack, g.Pickaxe, g.Glider, g.Contrail, g.Pet, g.PetName}
	}
	p.Stw = stwPayload(f)
	if l := f.Last; l != nil {
		p.Last = &fortniteFinal{Playlist: l.Playlist, Status: l.Status, Left: l.Left, StartedAt: ms(l.JoinedAt), EndedAt: ms(l.EndedAt)}
	}
	return p
}

// fortniteMode is the status line without its "- 73 Left" tail.
func fortniteMode(status string) (mode, left string) {
	if i := strings.LastIndex(status, " - "); i > 0 && strings.HasSuffix(status, " Left") {
		return status[:i], strings.TrimSuffix(status[i+3:], " Left") + " left"
	}
	return status, ""
}

func fortniteDetail(f fortnite.State) string {
	mode, left := fortniteMode(f.Status)
	if !f.InMatch {
		return joinDots("In the lobby", mode)
	}
	return joinDots(mode, left)
}

// fortniteOutfit is the outfit's picture on fortnite-api.com, by item id.
func fortniteOutfit(id string) string {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return ""
	}
	return "https://fortnite-api.com/images/cosmetics/br/" + id + "/icon.png"
}

var fortnitePhases = map[string]string{
	"Setup":     "Loading in",
	"Warmup":    "Warming up",
	"Aircraft":  "On the Battle Bus",
	"SafeZones": "In the storm",
	"EndGame":   "Endgame",
}

// appFortnite is Fortnite's own Discord application (Discord's detectable
// games list), so the card reads "Competing in Fortnite". glow.moe can swap
// it (settings.discordApps); members who don't want a status turn it off in L!VE.
var appFortnite = "1402418703554842694"

// fortniteActivity: Fortnite sets its own "Playing Fortnite" card and Discord
// shows one Playing card at a time (the game's wins, even under the same app
// id), so ours is a "Competing in Fortnite" card, which sits beside it: the
// outfit as the big picture with glow in the corner, the mode on top,
// players left (or the phase) under it, the match clock, and buttons to the
// lobby page and the profile.
func fortniteActivity(f fortnite.State, username string) discord.Activity {
	if w, ok := stwPlace(f); ok {
		return stwActivity(f, w, username)
	}
	mode, left := fortniteMode(f.Status)
	a := discord.Activity{Type: 5, Details: atLeast2(mode, "Fortnite")}
	switch {
	case !f.InMatch:
		a.State = "In the lobby"
		if n := len(f.Others()); n > 0 {
			a.State = fmt.Sprintf("In the lobby · party of %d", n+1)
		}
	case left != "":
		a.State = joinDots(left, fortnitePhases[f.Phase])
	default:
		a.State = fortnitePhases[f.Phase]
	}
	a.State = atLeast2(a.State, "")
	if f.InMatch && !f.JoinedAt.IsZero() && time.Since(f.JoinedAt) >= 0 && time.Since(f.JoinedAt) < 6*time.Hour {
		a.Timestamps = &discord.Timestamps{Start: f.JoinedAt.UnixMilli()}
	}
	large := fortniteOutfit(f.Loadout.Character)
	if large == "" {
		large = glowIcon
	}
	a.Assets = &discord.Assets{LargeImage: large, LargeText: fortniteLine(f, fortnite.OutfitName(f.Loadout.Character), username), SmallImage: glowIcon, SmallText: "glow.moe"}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "View my lobby", URL: "https://glow.moe/" + username + "/fortnite"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

// fortniteLine is the card's third line (the big picture's text): in the
// lobby, how the last match ended; otherwise the outfit's name; glow.moe/<you>
// until either is known.
func fortniteLine(f fortnite.State, outfit, username string) string {
	if !f.InMatch && f.Last != nil && f.Last.Left > 0 {
		return fmt.Sprintf("Last match: out with %d left", f.Last.Left)
	}
	if outfit != "" {
		return atLeast2(outfit, "")
	}
	if username != "" {
		return "glow.moe/" + username
	}
	return "glow.moe"
}

// stwLine is Fortnite's status line in Save the World ("Save the World:
// Stonewood (15)", "Save the World: Homebase (1 - Normal)"). It follows the
// game's language, so another language keeps the Battle Royale card. The
// number in brackets is the zone's power level (Stonewood 15); at Homebase it
// reads "1 - Normal" and stays off the card.
var stwLine = regexp.MustCompile(`(?i)^Save the World(?::\s*(.+?))?(?:\s*\((?:(\d{1,3})\)|.*\)))?$`)

var stwZones = map[string]string{"homebase": "homebase", "stonewood": "stonewood", "plankerton": "plankerton", "canny valley": "canny", "twine peaks": "twine"}

// stwWhere is where they are in Save the World: the place's name, the zone
// glow.moe draws a sky for ("other" for a place it doesn't know) and its power
// level ("" when the line has none).
type stwWhere struct{ place, zone, level string }

func stwPlace(f fortnite.State) (stwWhere, bool) {
	if !f.InMatch {
		return stwWhere{}, false
	}
	m := stwLine.FindStringSubmatch(strings.TrimSpace(f.Status))
	if m == nil {
		return stwWhere{}, false
	}
	w := stwWhere{place: strings.TrimSpace(m[1]), level: m[2]}
	if w.place == "" {
		w.place = "Homebase"
	}
	if w.zone = stwZones[strings.ToLower(w.place)]; w.zone == "" {
		w.zone = "other"
	}
	if w.zone == "homebase" || w.level == "0" {
		w.level = ""
	}
	return w, true
}

// stwActivity is the Fortnite card in Save the World: the zone on top, the
// zone's power level (or "On a mission", "At Homebase") and the party under it, their
// outfit standing under that zone's sky as the big picture (glow.moe draws it,
// the sky from their live page), glow in the corner, the clock and the buttons.
func stwActivity(f fortnite.State, w stwWhere, username string) discord.Activity {
	place, zone := w.place, w.zone
	home := zone == "homebase"
	a := discord.Activity{Type: 5, Details: "Save the World"}
	// The second line stays short (Discord cuts it): a Storm Shield's wave, else
	// the power level, stands in for "On a mission".
	where, doing := "At Homebase", "At Homebase"
	if !home {
		a.Details = atLeast2("Save the World · "+place, "Save the World")
		doing = "On a mission"
		where = doing
		if w.level != "" {
			where = stwLevel(w.level)
		}
		if f.Stw.Mission != "" && f.Stw.Wave > 0 {
			where = fmt.Sprintf("Storm Shield · Wave %d", f.Stw.Wave)
		}
	}
	if n := len(f.Others()); n > 0 {
		where = fmt.Sprintf("%s · party of %d", where, n+1)
	}
	a.State = where
	if !f.JoinedAt.IsZero() && time.Since(f.JoinedAt) >= 0 && time.Since(f.JoinedAt) < 6*time.Hour {
		a.Timestamps = &discord.Timestamps{Start: f.JoinedAt.UnixMilli()}
	}
	art := "https://glow.moe/api/live/rpc-art/stw?zone=" + zone
	if id := f.Loadout.Character; id != "" && !strings.ContainsAny(id, "/?#&") {
		art += "&outfit=" + url.QueryEscape(strings.ToLower(id))
	}
	// The third line: the outfit's name once it's looked up, else what they're doing.
	text := atLeast2(fortnite.OutfitName(f.Loadout.Character), "")
	if text == "" {
		text = doing
	}
	a.Assets = &discord.Assets{LargeImage: art, LargeText: text, SmallImage: glowIcon, SmallText: "glow.moe"}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "View my lobby", URL: "https://glow.moe/" + username + "/fortnite"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

// stwLevel is the power level as the card shows it ("Power level 15").
func stwLevel(level string) string {
	if level == "" {
		return ""
	}
	return "Power level " + level
}

func fortniteVars(f fortnite.State) map[string]string {
	mode, left := fortniteMode(f.Status)
	lastLeft := ""
	if f.Last != nil && f.Last.Left > 0 {
		lastLeft = fmt.Sprint(f.Last.Left)
	}
	v := map[string]string{
		"mode": mode, "left": left, "status": f.Status, "playlist": f.Playlist, "phase": f.Phase,
		"phaseName": fortnitePhases[f.Phase], "island": f.Island, "outfit": fortniteOutfit(f.Loadout.Character),
		"outfitName": fortnite.OutfitName(f.Loadout.Character), "lastLeft": lastLeft,
		"party": fmt.Sprint(len(f.Others()) + 1),
	}
	// Save the World only: {stwPlace} ("Stonewood"), {stwZone} ("stonewood",
	// "canny", "other"), {stwHome} "yes" at Homebase, {stwLevel} the zone's
	// power level ("15"), {stwWave} a Storm Shield's wave ("3"); empty anywhere else.
	if w, ok := stwPlace(f); ok {
		v["stwPlace"], v["stwZone"], v["stwHome"], v["stwLevel"] = w.place, w.zone, yes(w.zone == "homebase"), w.level
		if f.Stw.Wave > 0 {
			v["stwWave"] = fmt.Sprint(f.Stw.Wave)
		}
	}
	return v
}
