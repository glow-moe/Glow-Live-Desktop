package snapshot

import (
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/tft"
)

// TFT is Teamfight Tactics, from its own client: where you are, the queue,
// the set and the champions you bought this game. Sent as its own game
// ("tft"), shown at glow.moe/<you>/tft.
type TFT struct {
	State   string    `json:"state"` // menus | queue | inGame | postGame
	Queue   string    `json:"queue,omitempty"`
	QueueID int       `json:"queueId,omitempty"`
	Set     int       `json:"set,omitempty"`
	Units   []TFTUnit `json:"units,omitempty"`
	// Your Little Legend's art; glow.moe names it.
	Companion     string `json:"companion,omitempty"`
	CompanionIcon string `json:"companionIcon,omitempty"`
}

// TFTUnit is a champion on your side and the most stars it reached.
type TFTUnit struct {
	ChampKey string `json:"champKey"`
	Stars    int    `json:"stars"`
}

// tftQueues are the queues the TFT client has shown so far. glow.moe names
// them too (lib/league/names.ts); this is for the app's own status line.
var tftQueues = map[int]string{
	1090: "Normal",
	1100: "Ranked",
	1130: "Hyper Roll",
	1160: "Double Up",
	2210: "Tutorial",
}

const companionArt = "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/loadouts/companions/tooltip_"

// TFTCompanionIcon is a Little Legend's art on Community Dragon, where every
// path is lower case: "StyleTwoAhri_Base_Classic_Tier1" ->
// ".../tooltip_styletwoahri_base_classic_tier1.png".
func TFTCompanionIcon(id string) string {
	if id == "" {
		return ""
	}
	return companionArt + strings.ToLower(id) + ".png"
}

// TFTQueue names a TFT queue, or "" for one not seen yet.
func TFTQueue(id int) string { return tftQueues[id] }

// FromTFT is the frame for the TFT client. Live only while a game runs, so
// menus don't count as time played.
func FromTFT(s tft.State, now time.Time) Snapshot {
	t := &TFT{State: s.Phase, Queue: TFTQueue(s.QueueID), QueueID: s.QueueID, Set: s.Set, Companion: s.Companion, CompanionIcon: TFTCompanionIcon(s.Companion)}
	for _, u := range s.Best(tft.MaxUnits) {
		t.Units = append(t.Units, TFTUnit{ChampKey: u.Champ, Stars: u.Stars})
	}
	clock := ""
	if !s.StartedAt.IsZero() {
		clock = fmtClock(now.Sub(s.StartedAt).Seconds())
	}
	phase := "lobby"
	if s.Phase == tft.PhaseInGame {
		phase = "inGame"
	}
	return Snapshot{
		Game:      "tft",
		Live:      s.Phase == tft.PhaseInGame,
		Phase:     phase,
		UpdatedAt: now.UnixMilli(),
		Clock:     clock,
		MapName:   "Teamfight Tactics",
		Me:        Me{RiotName: s.Name, RiotTag: s.Tag},
		Blue:      []Player{},
		Red:       []Player{},
		Feed:      []FeedEvent{},
		Tft:       t,
	}
}
