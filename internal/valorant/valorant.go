// Package valorant reads VALORANT's state from the Riot Client's local chat
// API: your own presence carries a base64 JSON blob (menus / agent select /
// match, map, queue, score, rank, level). The app decodes it and forwards it
// as is; glow.moe names the map, mode and rank (src/lib/valorant/presence.ts
// there), so the app stays a thin pipe.
//
// The presence is local and read-only; friends' presences are in the same list
// and are never read past the puuid check. The match itself (who's in it, the
// server, the scoreboard after) comes from remote.go, only when glow.moe
// switches it on.
package valorant

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/glow-moe/glow-collector/internal/riotclient"
)

// ErrNoGame means VALORANT isn't running (no Riot Client, or you're in another
// Riot game).
var ErrNoGame = errors.New("valorant not running")

// State is what the app knows: your Riot ID and the decoded presence blob.
type State struct {
	Name    string
	Tag     string
	Private json.RawMessage
	// Basics the app itself uses (status line + Discord).
	Phase     string // MENUS | PREGAME | INGAME
	Queue     string
	MapCode   string // last segment of the map path, "Ascent", "Duality"...
	Flow      string // provisioningFlow: Matchmaking | CustomGame | ShootingRange
	AllyScore int
	FoeScore  int
	Tier      int
	Level     int
	// The party: DEFAULT, MATCHMAKING or CUSTOM_GAME_SETUP, how many, of how many.
	PartyState string
	PartySize  int
	PartyMax   int
	Card       string // equipped player card (valorant-api.com id)

	// The local Riot Client the presence came from, for the match reads
	// (remote.go).
	port, password, puuid string
}

// Fetch returns your VALORANT presence, or ErrNoGame.
func Fetch() (*State, error) {
	l, err := riotclient.Open()
	if err != nil {
		return nil, ErrNoGame
	}
	self, err := l.Self()
	if err != nil {
		return nil, ErrNoGame
	}
	p, ok := l.Own(self, "valorant")
	dec := riotclient.Decode(p.Private)
	if !ok || dec == nil {
		return nil, ErrNoGame
	}
	st := &State{Name: self.Name, Tag: self.Tag, Private: dec, port: l.Port, password: l.Password, puuid: self.PUUID}
	st.read(dec)
	return st, nil
}

// read pulls the basics out of the blob. Current clients nest them under
// matchPresenceData / partyPresenceData / playerPresenceData; older ones were
// flat, so both are read.
func (s *State) read(dec []byte) {
	var pb struct {
		Match struct {
			MatchMap         string `json:"matchMap"`
			QueueID          string `json:"queueId"`
			SessionLoopState string `json:"sessionLoopState"`
			ProvisioningFlow string `json:"provisioningFlow"`
		} `json:"matchPresenceData"`
		Party struct {
			MatchMap  string `json:"partyOwnerMatchMap"`
			ScoreAlly int    `json:"partyOwnerMatchScoreAllyTeam"`
			ScoreFoe  int    `json:"partyOwnerMatchScoreEnemyTeam"`
			State     string `json:"partyState"`
			Size      int    `json:"partySize"`
			Max       int    `json:"maxPartySize"`
		} `json:"partyPresenceData"`
		Player struct {
			AccountLevel    int    `json:"accountLevel"`
			CompetitiveTier int    `json:"competitiveTier"`
			PlayerCardID    string `json:"playerCardId"`
			CardID          string `json:"cardId"`
		} `json:"playerPresenceData"`
		SessionLoopState string `json:"sessionLoopState"`
		ProvisioningFlow string `json:"provisioningFlow"`
		QueueID          string `json:"queueId"`
		MatchMap         string `json:"matchMap"`
		ScoreAlly        int    `json:"partyOwnerMatchScoreAllyTeam"`
		ScoreFoe         int    `json:"partyOwnerMatchScoreEnemyTeam"`
		AccountLevel     int    `json:"accountLevel"`
		CompetitiveTier  int    `json:"competitiveTier"`
		PartyState       string `json:"partyState"`
		PartySize        int    `json:"partySize"`
		MaxPartySize     int    `json:"maxPartySize"`
		PlayerCardID     string `json:"playerCardId"`
	}
	if json.Unmarshal(dec, &pb) != nil {
		return
	}
	s.Phase = or(pb.Match.SessionLoopState, pb.SessionLoopState)
	s.Queue = or(pb.Match.QueueID, pb.QueueID)
	s.Flow = or(pb.Match.ProvisioningFlow, pb.ProvisioningFlow)
	path := or(pb.Party.MatchMap, or(pb.Match.MatchMap, pb.MatchMap))
	if path != "" {
		seg := strings.Split(strings.Trim(path, "/"), "/")
		s.MapCode = seg[len(seg)-1]
	}
	s.AllyScore = orN(pb.Party.ScoreAlly, pb.ScoreAlly)
	s.FoeScore = orN(pb.Party.ScoreFoe, pb.ScoreFoe)
	s.Tier = orN(pb.Player.CompetitiveTier, pb.CompetitiveTier)
	s.Level = orN(pb.Player.AccountLevel, pb.AccountLevel)
	s.PartyState = or(pb.Party.State, pb.PartyState)
	s.PartySize = orN(pb.Party.Size, pb.PartySize)
	s.PartyMax = orN(pb.Party.Max, pb.MaxPartySize)
	s.Card = or(pb.Player.PlayerCardID, or(pb.Player.CardID, pb.PlayerCardID))
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func orN(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

var tiers = []string{"Iron", "Bronze", "Silver", "Gold", "Platinum", "Diamond", "Ascendant", "Immortal"}

// RankName is "Gold 2" for competitive tier 13 ("" unranked, "Radiant" 27).
func RankName(n int) string {
	if n <= 2 {
		return ""
	}
	if n >= 27 {
		return "Radiant"
	}
	i, div := (n-3)/3, (n-3)%3+1
	if i >= len(tiers) {
		return ""
	}
	return tiers[i] + " " + string(rune('0'+div))
}
