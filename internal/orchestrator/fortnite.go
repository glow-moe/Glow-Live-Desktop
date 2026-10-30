package orchestrator

import (
	"fmt"
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
	UpdatedAt int64          `json:"updatedAt"`
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

// appFortnite is Fortnite's own Discord application (Discord's detectable
// games list), so the card reads "Playing Fortnite". glow.moe can swap it
// (settings.discordApps); members who don't want a status turn it off in L!VE.
var appFortnite = "1402418703554842694"

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

// fortniteActivity: the outfit as the big picture with glow in the corner,
// the mode on top, players left (or the phase) under it, the match clock,
// and buttons to the lobby page and the profile.
func fortniteActivity(f fortnite.State, username string) discord.Activity {
	mode, left := fortniteMode(f.Status)
	a := discord.Activity{Details: atLeast2(mode, "Fortnite")}
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
	a.Assets = &discord.Assets{LargeImage: large, LargeText: atLeast2(mode, "Fortnite"), SmallImage: glowIcon, SmallText: "glow.moe"}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "View my lobby", URL: "https://glow.moe/" + username + "/fortnite"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

func fortniteVars(f fortnite.State) map[string]string {
	mode, left := fortniteMode(f.Status)
	return map[string]string{
		"mode": mode, "left": left, "status": f.Status, "playlist": f.Playlist, "phase": f.Phase,
		"phaseName": fortnitePhases[f.Phase], "island": f.Island, "outfit": fortniteOutfit(f.Loadout.Character),
		"party": fmt.Sprint(len(f.Others()) + 1),
	}
}
