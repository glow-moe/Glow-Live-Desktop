package snapshot

import (
	"strings"

	"github.com/glow-moe/glow-collector/internal/lcu"
	"github.com/glow-moe/glow-collector/internal/live"
)

// TFT is a Teamfight Tactics game. It has no champion, scoreboard or match
// feed; with this set, the page and Discord show these instead.
type TFT struct {
	Queue         string `json:"queue,omitempty"`
	Companion     string `json:"companion,omitempty"`
	CompanionIcon string `json:"companionIcon,omitempty"`
	Tier          string `json:"tier,omitempty"`
	Division      string `json:"division,omitempty"`
	LP            int    `json:"lp,omitempty"`
}

// IsTFT reports whether the live game is Teamfight Tactics.
func IsTFT(d *live.AllGameData) bool {
	return strings.EqualFold(d.GameData.GameMode, "TFT")
}

// BuildTFT maps a TFT game. Only your level and the clock come from the Live
// Client API; info, read from the client, may be nil.
func BuildTFT(d *live.AllGameData, info *lcu.TFTInfo, patch string, now int64) Snapshot {
	ap := d.ActivePlayer
	name, tag := splitRiotID(ap.RiotID, ap.RiotIDName, ap.SummonerName)
	tft := &TFT{}
	if info != nil {
		tft = &TFT{
			Queue: info.Queue, Companion: info.Companion, CompanionIcon: info.CompanionIcon,
			Tier: info.Tier, Division: info.Division, LP: info.LP,
		}
	}
	return Snapshot{
		Game:      "lol",
		Patch:     patch,
		Live:      true,
		Phase:     "inGame",
		UpdatedAt: now,
		Clock:     fmtClock(d.GameData.GameTime),
		MapName:   mapLabel(d.GameData),
		Me:        Me{RiotName: name, RiotTag: tag, Level: ap.Level},
		Blue:      []Player{},
		Red:       []Player{},
		Feed:      []FeedEvent{},
		Tft:       tft,
	}
}
