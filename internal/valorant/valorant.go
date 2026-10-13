// Package valorant reads VALORANT's state from the Riot Client's local chat
// API: your own presence carries a base64 JSON blob (menus / agent select /
// match, map, queue, score, rank, level). The app decodes it and forwards it
// as is; glow.moe names the map, mode and rank (src/lib/valorant/presence.ts
// there), so the app stays a thin pipe.
//
// Presence only, all local and read-only: no Riot servers, nothing about the
// match beyond what your friends list already sees. Friends' presences are in
// the same list and are never read past the puuid check.
package valorant

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
}

func lockfilePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Riot Games", "Riot Client", "Config", "lockfile")
}

// One small pool, like the League client's (lcu.go): a fresh transport per call
// leaks sockets until the local API starts refusing.
var client = &http.Client{
	Timeout: 4 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, // #nosec G402 - localhost Riot cert
		MaxIdleConnsPerHost: 2,
		MaxConnsPerHost:     4,
		IdleConnTimeout:     30 * time.Second,
	},
}

func get(port, password, path string, max int64, out any) error {
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+port+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("riot:"+password)))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, max)).Decode(out)
}

// Fetch returns your VALORANT presence, or ErrNoGame.
func Fetch() (*State, error) {
	lf := lockfilePath()
	if lf == "" {
		return nil, ErrNoGame
	}
	b, err := os.ReadFile(lf)
	if err != nil {
		return nil, ErrNoGame
	}
	parts := strings.Split(strings.TrimSpace(string(b)), ":")
	if len(parts) < 5 {
		return nil, ErrNoGame
	}
	port, password := parts[2], parts[3]

	var sess struct {
		PUUID    string `json:"puuid"`
		GameName string `json:"game_name"`
		GameTag  string `json:"game_tag"`
	}
	if err := get(port, password, "/chat/v1/session", 64<<10, &sess); err != nil || sess.PUUID == "" {
		return nil, ErrNoGame
	}
	var pr struct {
		Presences []struct {
			PUUID   string `json:"puuid"`
			Product string `json:"product"`
			Private string `json:"private"`
		} `json:"presences"`
	}
	if err := get(port, password, "/chat/v4/presences", 4<<20, &pr); err != nil {
		return nil, ErrNoGame
	}
	for _, p := range pr.Presences {
		if p.PUUID != sess.PUUID || p.Product != "valorant" || p.Private == "" {
			continue
		}
		dec, err := base64.StdEncoding.DecodeString(p.Private)
		if err != nil || !json.Valid(dec) || len(dec) > 64<<10 {
			continue
		}
		st := &State{Name: sess.GameName, Tag: sess.GameTag, Private: dec}
		st.read(dec)
		return st, nil
	}
	return nil, ErrNoGame
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
		} `json:"partyPresenceData"`
		Player struct {
			AccountLevel    int `json:"accountLevel"`
			CompetitiveTier int `json:"competitiveTier"`
		} `json:"playerPresenceData"`
		SessionLoopState string `json:"sessionLoopState"`
		ProvisioningFlow string `json:"provisioningFlow"`
		QueueID          string `json:"queueId"`
		MatchMap         string `json:"matchMap"`
		ScoreAlly        int    `json:"partyOwnerMatchScoreAllyTeam"`
		ScoreFoe         int    `json:"partyOwnerMatchScoreEnemyTeam"`
		AccountLevel     int    `json:"accountLevel"`
		CompetitiveTier  int    `json:"competitiveTier"`
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
