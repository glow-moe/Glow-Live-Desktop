// Package lcu reads the League *client* (LCU) API to report out-of-game state:
// in the client, in a lobby, in queue, or in champion select. The client writes
// a lockfile ("LeagueClient:pid:port:password:https") in its install dir; we
// read the port + password from it and call the local HTTPS API with basic auth.
package lcu

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Member is one person in a party lobby.
type Member struct {
	Name   string   `json:"name"`
	Tag    string   `json:"tag"`
	Level  int      `json:"level"`
	IconID int      `json:"iconId"`
	Leader bool     `json:"leader"`
	You    bool     `json:"you"`
	Roles  []string `json:"roles"`
}

// Draftee is one cell in champ select (either team).
type Draftee struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	ChampID int    `json:"champId"`
	HoverID int    `json:"hoverId"`
	Spell1  int    `json:"spell1"`
	Spell2  int    `json:"spell2"`
	Status  string `json:"status"` // locked/picking/hover/waiting
	You     bool   `json:"you"`
}

// Lobby is the resolved out-of-game state.
type Lobby struct {
	Phase    string `json:"phase"`
	Label    string `json:"label"`
	Summoner string `json:"summoner"`
	Level    int    `json:"level"`
	IconID   int    `json:"iconId"`
	ChampID  int    `json:"champId"` // self's picked champion in champ select

	// party lobby
	QueueName string   `json:"queueName"`
	MapName   string   `json:"mapName"`
	ModeLabel string   `json:"modeLabel"`
	IsRanked  bool     `json:"isRanked"`
	MaxSize   int      `json:"maxSize"`
	PartyID   string   `json:"partyId"`
	Members   []Member `json:"members"`

	// champ select
	TimeLeft   int       `json:"timeLeft"`
	PhaseLabel string    `json:"phaseLabel"`
	BlueBans   []int     `json:"blueBans"`
	RedBans    []int     `json:"redBans"`
	Blue       []Draftee `json:"blue"`
	Red        []Draftee `json:"red"`

	// in the client (phase "None"): the rich profile card
	Client *ClientProfile `json:"client,omitempty"`
}

// Rank is one ranked queue's standing (raw ids; the snapshot/web derive colors).
type Rank struct {
	Queue     string // "Solo/Duo" | "Flex"
	Tier      string // "GOLD" … "" = unranked
	Division  string // "I".."IV" ("" for Master+ / unranked)
	LP        int
	Wins      int
	Losses    int
	HotStreak bool
}

// ChallengeToken is one pinned challenge crystal.
type ChallengeToken struct {
	Name string
	Tier string
}

// ChallengeProgress is a featured challenge's progress toward its next level.
type ChallengeProgress struct {
	ID        int
	Name      string
	Level     string
	Current   float64
	Threshold float64
}

// MasteryChamp is a top-mastery champion (champ id resolved in the snapshot).
type MasteryChamp struct {
	ChampID int
	Level   int
	Points  int
	Tokens  int
}

// MatchInfo is one recent-match summary (last 5).
type MatchInfo struct {
	ChampID     int
	Queue       string
	DurationSec int
	Kills       int
	Deaths      int
	Assists     int
	Win         bool
}

// ClientProfile is everything shown on the "In the client" screen. All sections
// are best-effort - a failing LCU call just leaves that slice empty/zero.
type ClientProfile struct {
	GameName      string
	TagLine       string
	Level         int
	IconID        int
	XpSince       int
	XpTo          int
	BannerChampID int
	BannerSkinID  int
	Availability  string
	StatusMessage string
	Title         string
	HonorLevel    int
	Ranks         []Rank
	ChallengeScore  int
	OverallLevel    string
	ChallengeTokens []ChallengeToken
	Challenges      []ChallengeProgress
	MasteryScore    int
	Mastery         []MasteryChamp
	Matches         []MatchInfo
}

var commonPaths = []string{
	`C:\Riot Games\League of Legends`,
	`C:\Program Files\Riot Games\League of Legends`,
	`C:\Program Files (x86)\Riot Games\League of Legends`,
	`D:\Riot Games\League of Legends`,
	`E:\Riot Games\League of Legends`,
}

// ErrNoClient means the League client isn't running (no readable lockfile).
var ErrNoClient = errors.New("league client not running")

// SaveRaw, when set, writes raw /lol-lobby + /lol-champ-select responses here so
// the mapping can be finalized against real data.
var SaveRaw = "."

// profCache caches the heavy "in the client" profile so we don't refetch its ~8
// LCU endpoints every poll (see Fetch's "None" case).
var (
	profCache *ClientProfile
	profAt    time.Time
)

func findLockfile(override string) (string, bool) {
	dirs := commonPaths
	if override != "" {
		dirs = append([]string{override}, dirs...)
	}
	for _, d := range dirs {
		p := filepath.Join(d, "lockfile")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

type conn struct {
	base string
	auth string
	http *http.Client
}

func dial(override string) (*conn, error) {
	lf, ok := findLockfile(override)
	if !ok {
		// No lockfile = the client isn't running (distinct from a transient
		// timeout while it's busy, which the caller keeps state through).
		return nil, ErrNoClient
	}
	b, err := os.ReadFile(lf)
	if err != nil {
		return nil, fmt.Errorf("lockfile unreadable (%s): %w", lf, err)
	}
	parts := strings.Split(strings.TrimSpace(string(b)), ":")
	if len(parts) < 5 {
		return nil, errors.New("lockfile malformed")
	}
	port, password := parts[2], parts[3]
	return &conn{
		base: "https://127.0.0.1:" + port,
		auth: "Basic " + base64.StdEncoding.EncodeToString([]byte("riot:"+password)),
		http: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, // #nosec G402 - localhost LCU cert
		},
	}, nil
}

func (c *conn) getRaw(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *conn) get(path string, out any) error {
	b, err := c.getRaw(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// Fetch returns the current out-of-game state, or ErrNoClient if the client
// isn't running. leaguePath is an optional lockfile-dir override.
func Fetch(leaguePath string) (*Lobby, error) {
	c, err := dial(leaguePath)
	if err != nil {
		return nil, err
	}

	var phase string
	if err := c.get("/lol-gameflow/v1/gameflow-phase", &phase); err != nil {
		if isConnDown(err) {
			// Lockfile present but nothing listening on the port - the client is
			// down / mid-restart (a stale lockfile from an unclean close). Treat it
			// as not-running so we idle cleanly and recover once it's back up with
			// a fresh lockfile, instead of sitting on "client busy" forever.
			return nil, ErrNoClient
		}
		return nil, fmt.Errorf("client api call failed: %w", err)
	}
	lob := &Lobby{Phase: phase, Label: labelFor(phase)}

	var sum struct {
		GameName    string `json:"gameName"`
		DisplayName string `json:"displayName"`
		TagLine     string `json:"tagLine"`
		Level       int    `json:"summonerLevel"`
		IconID      int    `json:"profileIconId"`
		Puuid       string `json:"puuid"`
		SummonerID  int64  `json:"summonerId"`
		XpSince     int    `json:"xpSinceLastLevel"`
		XpTo        int    `json:"xpUntilNextLevel"`
	}
	_ = c.get("/lol-summoner/v1/current-summoner", &sum)
	lob.Summoner = firstNonEmpty(sum.GameName, sum.DisplayName)
	lob.Level, lob.IconID = sum.Level, sum.IconID

	switch phase {
	case "Lobby":
		fetchLobby(c, lob, sum.Puuid, sum.SummonerID)
	case "ChampSelect":
		fetchChampSelect(c, lob)
	case "InProgress":
		// Loading screen: the Live Client Data API isn't up yet, but the gameflow
		// session already has both rosters (champs + spells). Show them until the
		// in-game HUD takes over.
		fetchLoading(c, lob, sum.Puuid)
	case "None", "":
		seed := &ClientProfile{
			GameName: lob.Summoner,
			TagLine:  strings.TrimPrefix(sum.TagLine, "#"),
			Level:    sum.Level,
			IconID:   sum.IconID,
			XpSince:  sum.XpSince,
			XpTo:     sum.XpTo,
		}
		// The heavy profile (rank/mastery/matches/challenges/… ≈ 8 LCU calls) barely
		// changes, so fetch it at most every 30s and reuse it in between - hammering
		// 8 endpoints every 2s loads the LCU and invites timeouts. Identity fields
		// (name/level/icon) stay live every tick.
		if profCache != nil && time.Since(profAt) < 30*time.Second {
			merged := *profCache
			merged.GameName, merged.TagLine = seed.GameName, seed.TagLine
			merged.Level, merged.IconID = seed.Level, seed.IconID
			merged.XpSince, merged.XpTo = seed.XpSince, seed.XpTo
			lob.Client = &merged
		} else {
			fetchClientProfile(c, seed, sum.Puuid)
			profCache = seed
			profAt = time.Now()
			lob.Client = seed
		}
	}
	return lob, nil
}

// fetchClientProfile fills the "In the client" card from the LCU. Every call is
// independent + best-effort; raw responses are saved so mapping can be finalized.
func fetchClientProfile(c *conn, p *ClientProfile, puuid string) {
	// Ranked standing (Solo/Duo + Flex).
	if raw, err := c.getRaw("/lol-ranked/v1/current-ranked-stats"); err == nil {
		saveRaw("lcu-ranked.json", raw)
		var rs struct {
			QueueMap map[string]struct {
				Tier      string `json:"tier"`
				Division  string `json:"division"`
				LP        int    `json:"leaguePoints"`
				Wins      int    `json:"wins"`
				Losses    int    `json:"losses"`
				HotStreak bool   `json:"isHotStreak"`
			} `json:"queueMap"`
		}
		if json.Unmarshal(raw, &rs) == nil {
			for _, q := range []struct{ key, label string }{
				{"RANKED_SOLO_5x5", "Solo/Duo"},
				{"RANKED_FLEX_SR", "Flex"},
			} {
				if e, ok := rs.QueueMap[q.key]; ok {
					p.Ranks = append(p.Ranks, Rank{
						Queue: q.label, Tier: cleanTier(e.Tier), Division: cleanDiv(e.Division),
						LP: e.LP, Wins: e.Wins, Losses: e.Losses, HotStreak: e.HotStreak,
					})
				}
			}
		}
	}

	// Honor level.
	if raw, err := c.getRaw("/lol-honor-v2/v1/profile"); err == nil {
		saveRaw("lcu-honor.json", raw)
		var h struct {
			HonorLevel int `json:"honorLevel"`
		}
		if json.Unmarshal(raw, &h) == nil {
			p.HonorLevel = h.HonorLevel
		}
	} else {
		fmt.Println("  (honor fetch failed:", err, ")")
	}

	// Challenges: total score, overall crystal level, selected title. Field names
	// vary by client version, so read both known shapes.
	if raw, err := c.getRaw("/lol-challenges/v1/summary-player-data/local-player"); err == nil {
		saveRaw("lcu-challenges.json", raw)
		var ch struct {
			TotalPoints struct {
				Level   string `json:"level"`
				Current int    `json:"current"`
			} `json:"totalPoints"`
			TotalChallengeScore   int    `json:"totalChallengeScore"`
			OverallChallengeLevel string `json:"overallChallengeLevel"`
			Title                 struct {
				Name string `json:"name"`
			} `json:"title"`
			TopChallenges []struct {
				ChallengeID   int     `json:"challengeId"`
				ID            int     `json:"id"`
				CurrentLevel  string  `json:"currentLevel"`
				Level         string  `json:"level"`
				CurrentValue  float64 `json:"currentValue"`
				Value         float64 `json:"value"`
				NextThreshold float64 `json:"nextThreshold"`
			} `json:"topChallenges"`
		}
		if json.Unmarshal(raw, &ch) == nil {
			p.ChallengeScore = ch.TotalPoints.Current
			if p.ChallengeScore == 0 {
				p.ChallengeScore = ch.TotalChallengeScore
			}
			p.OverallLevel = titleCase(firstNonEmpty(ch.TotalPoints.Level, ch.OverallChallengeLevel))
			p.Title = ch.Title.Name
			for _, t := range ch.TopChallenges {
				id := t.ChallengeID
				if id == 0 {
					id = t.ID
				}
				p.Challenges = append(p.Challenges, ChallengeProgress{
					ID:        id,
					Level:     titleCase(firstNonEmpty(t.CurrentLevel, t.Level)),
					Current:   firstNonZero(t.CurrentValue, t.Value),
					Threshold: t.NextThreshold,
				})
			}
		}
	} else {
		fmt.Println("  (challenges fetch failed:", err, ")")
	}
	// Challenge names (separate catalog endpoint; best-effort - bars still work
	// without them). The catalog is keyed by challenge id.
	if len(p.Challenges) > 0 {
		if raw, err := c.getRaw("/lol-challenges/v1/challenges/local-player"); err == nil {
			saveRaw("lcu-challenge-catalog.json", raw)
			var cat map[string]struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &cat) == nil {
				for i := range p.Challenges {
					if e, ok := cat[fmt.Sprint(p.Challenges[i].ID)]; ok {
						p.Challenges[i].Name = e.Name
					}
				}
			}
		}
	}

	// Top mastery champions. Riot moved these under /{puuid}/…; keep the older
	// local-player list as a fallback. Sort by points and take the top 3.
	type masteryRow struct {
		ChampionID     int `json:"championId"`
		ChampionLevel  int `json:"championLevel"`
		ChampionPoints int `json:"championPoints"`
		TokensEarned   int `json:"tokensEarned"`
	}
	masteryPaths := []string{}
	if puuid != "" {
		masteryPaths = append(masteryPaths, "/lol-champion-mastery/v1/"+puuid+"/champion-mastery/top?limit=12")
	}
	masteryPaths = append(masteryPaths, "/lol-champion-mastery/v1/local-player/champion-mastery")
	for _, path := range masteryPaths {
		raw, err := c.getRaw(path)
		if err != nil {
			continue
		}
		var rows []masteryRow
		if json.Unmarshal(raw, &rows) == nil && len(rows) > 0 {
			saveRaw("lcu-mastery.json", raw)
			sort.Slice(rows, func(i, j int) bool { return rows[i].ChampionPoints > rows[j].ChampionPoints })
			for i, m := range rows {
				if i >= 12 {
					break
				}
				p.Mastery = append(p.Mastery, MasteryChamp{
					ChampID: m.ChampionID, Level: m.ChampionLevel,
					Points: m.ChampionPoints, Tokens: m.TokensEarned,
				})
			}
			break
		}
	}
	if len(p.Mastery) == 0 {
		fmt.Println("  (mastery fetch failed - tried puuid + local-player)")
	}
	scorePaths := []string{}
	if puuid != "" {
		scorePaths = append(scorePaths, "/lol-champion-mastery/v1/"+puuid+"/champion-mastery-score")
	}
	scorePaths = append(scorePaths, "/lol-champion-mastery/v1/local-player/champion-mastery-score")
	for _, path := range scorePaths {
		if raw, err := c.getRaw(path); err == nil {
			var score int
			if json.Unmarshal(raw, &score) == nil {
				p.MasteryScore = score
				break
			}
		}
	}
	if len(p.Mastery) > 0 {
		p.BannerChampID = p.Mastery[0].ChampID
	}

	// Recent matches (last 5).
	if raw, err := c.getRaw("/lol-match-history/v1/products/lol/current-summoner/matches?begIndex=0&endIndex=5"); err == nil {
		saveRaw("lcu-matches.json", raw)
		var mh struct {
			Games struct {
				Games []struct {
					GameMode     string `json:"gameMode"`
					QueueID      int    `json:"queueId"`
					GameDuration int    `json:"gameDuration"`
					Participants []struct {
						ChampionID int `json:"championId"`
						Stats      struct {
							Win     bool `json:"win"`
							Kills   int  `json:"kills"`
							Deaths  int  `json:"deaths"`
							Assists int  `json:"assists"`
						} `json:"stats"`
					} `json:"participants"`
				} `json:"games"`
			} `json:"games"`
		}
		if json.Unmarshal(raw, &mh) == nil {
			for _, g := range mh.Games.Games {
				if len(g.Participants) == 0 {
					break
				}
				pt := g.Participants[0]
				p.Matches = append(p.Matches, MatchInfo{
					ChampID: pt.ChampionID, Queue: matchQueue(g.QueueID),
					DurationSec: g.GameDuration, Kills: pt.Stats.Kills,
					Deaths: pt.Stats.Deaths, Assists: pt.Stats.Assists, Win: pt.Stats.Win,
				})
				if len(p.Matches) >= 5 {
					break
				}
			}
		}
	}

	// Selected profile background (a champion skin, or an event background). Used
	// as the full-page backdrop; falls back to the top-mastery champ on the site.
	if puuid != "" {
		if raw, err := c.getRaw("/lol-summoner/v1/summoner-profile?puuid=" + puuid); err == nil {
			saveRaw("lcu-profile.json", raw)
			var pr struct {
				BackgroundSkinID int `json:"backgroundSkinId"`
			}
			if json.Unmarshal(raw, &pr) == nil {
				p.BannerSkinID = pr.BackgroundSkinID
			}
		}
	}
	// Banner champ fallback: no mastery → use the most recent match's champ, so
	// both the site backdrop and the Discord large image always have real art.
	if p.BannerChampID == 0 && len(p.Matches) > 0 {
		p.BannerChampID = p.Matches[0].ChampID
	}

	// Availability + status message.
	if raw, err := c.getRaw("/lol-chat/v1/me"); err == nil {
		saveRaw("lcu-chatme.json", raw)
		var me struct {
			Availability  string `json:"availability"`
			StatusMessage string `json:"statusMessage"`
		}
		if json.Unmarshal(raw, &me) == nil {
			p.Availability = availabilityLabel(me.Availability)
			p.StatusMessage = me.StatusMessage
		}
	}
	if p.Availability == "" {
		p.Availability = "Online"
	}
}

func fetchLobby(c *conn, lob *Lobby, puuid string, sumID int64) {
	// Modern clients serve /v2; keep /v1 as a fallback.
	raw, err := c.getRaw("/lol-lobby/v2/lobby")
	if err != nil {
		raw, err = c.getRaw("/lol-lobby/v1/lobby")
	}
	if err != nil {
		fmt.Println("  (lobby data unavailable:", err, ")")
		return
	}
	saveRaw("lcu-lobby.json", raw)
	var lb struct {
		PartyID    string `json:"partyId"`
		GameConfig struct {
			QueueID       int    `json:"queueId"`
			MapID         int    `json:"mapId"`
			GameMode      string `json:"gameMode"`
			MaxLobbySize  int    `json:"maxLobbySize"`
			IsRanked      bool   `json:"isRanked"`
		} `json:"gameConfig"`
		Members []struct {
			GameName      string `json:"summonerName"`
			GameName2     string `json:"gameName"`
			TagLine       string `json:"tagLine"`
			Level         int    `json:"summonerLevel"`
			IconID        int    `json:"summonerIconId"`
			IsLeader      bool   `json:"isLeader"`
			Puuid         string `json:"puuid"`
			SummonerID    int64  `json:"summonerId"`
			FirstPosition string `json:"firstPositionPreference"`
			SecondPos     string `json:"secondPositionPreference"`
		} `json:"members"`
	}
	if json.Unmarshal(raw, &lb) != nil {
		return
	}
	gc := lb.GameConfig
	lob.PartyID = lb.PartyID
	lob.MaxSize = orDefault(gc.MaxLobbySize, 5)
	lob.QueueName = queueName(gc.QueueID)
	lob.MapName = mapName(gc.MapID)
	lob.ModeLabel = modeLabel(gc.QueueID, gc.GameMode)
	lob.IsRanked = gc.IsRanked || rankedQueue(gc.QueueID)
	for _, m := range lb.Members {
		lob.Members = append(lob.Members, Member{
			Name:   firstNonEmpty(m.GameName2, m.GameName),
			Tag:    tagOf(m.TagLine),
			Level:  m.Level,
			IconID: m.IconID,
			Leader: m.IsLeader,
			You:    (puuid != "" && m.Puuid == puuid) || (sumID != 0 && m.SummonerID == sumID),
			Roles:  roles(m.FirstPosition, m.SecondPos),
		})
	}
}

// gamePlayer is one entry in a gameflow-session team roster.
type gamePlayer struct {
	ChampionID   int    `json:"championId"`
	SkinIndex    int    `json:"selectedSkinIndex"`
	Spell1       int    `json:"spell1Id"`
	Spell2       int    `json:"spell2Id"`
	SummonerName string `json:"summonerName"`
	InternalName string `json:"summonerInternalName"`
	Position     string `json:"selectedPosition"`
	Puuid        string `json:"puuid"`
}

// fetchLoading reads the gameflow session (available on the loading screen,
// before the Live Client Data API is up) into both rosters.
func fetchLoading(c *conn, lob *Lobby, selfPuuid string) {
	raw, err := c.getRaw("/lol-gameflow/v1/session")
	if err != nil {
		return
	}
	saveRaw("lcu-gameflow.json", raw)
	var s struct {
		GameData struct {
			Queue struct {
				ID    int `json:"id"`
				MapID int `json:"mapId"`
			} `json:"queue"`
			PlayerChampionSelections []gamePlayer `json:"playerChampionSelections"`
			TeamOne                  []gamePlayer `json:"teamOne"`
			TeamTwo                  []gamePlayer `json:"teamTwo"`
		} `json:"gameData"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return
	}
	lob.PhaseLabel = "Loading into game"
	lob.QueueName = queueName(s.GameData.Queue.ID)
	lob.MapName = mapName(s.GameData.Queue.MapID)
	// Champs may live on the team entries or in playerChampionSelections - index
	// the selections by internal name and fall back to them.
	byName := map[string]gamePlayer{}
	for _, p := range s.GameData.PlayerChampionSelections {
		byName[p.InternalName] = p
	}
	lob.Blue = gameTeam(s.GameData.TeamOne, byName, selfPuuid)
	lob.Red = gameTeam(s.GameData.TeamTwo, byName, selfPuuid)
}

func gameTeam(players []gamePlayer, byName map[string]gamePlayer, selfPuuid string) []Draftee {
	out := make([]Draftee, 0, len(players))
	for i, p := range players {
		champ, s1, s2 := p.ChampionID, p.Spell1, p.Spell2
		if champ == 0 {
			if x, ok := byName[p.InternalName]; ok {
				champ, s1, s2 = x.ChampionID, x.Spell1, x.Spell2
			}
		}
		name := firstNonEmpty(p.SummonerName, p.InternalName)
		if name == "" {
			name = fmt.Sprintf("Player %d", i+1)
		}
		out = append(out, Draftee{
			Name:    name,
			Role:    strings.ToUpper(p.Position),
			ChampID: champ,
			Spell1:  s1,
			Spell2:  s2,
			Status:  "locked",
			You:     selfPuuid != "" && p.Puuid == selfPuuid,
		})
	}
	return out
}

func fetchChampSelect(c *conn, lob *Lobby) {
	raw, err := c.getRaw("/lol-champ-select/v1/session")
	if err != nil {
		return
	}
	saveRaw("lcu-champselect.json", raw)
	var s struct {
		Timer struct {
			Phase             string `json:"phase"`
			AdjustedTimeLeft  int    `json:"adjustedTimeLeftInPhase"`
		} `json:"timer"`
		LocalCellID int      `json:"localPlayerCellId"`
		MyTeam      []cell   `json:"myTeam"`
		TheirTeam   []cell   `json:"theirTeam"`
		Actions     [][]act  `json:"actions"`
		Bans        struct {
			MyTeamBans    []int `json:"myTeamBans"`
			TheirTeamBans []int `json:"theirTeamBans"`
		} `json:"bans"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return
	}
	lob.PhaseLabel = phaseLabel(s.Timer.Phase)
	lob.TimeLeft = s.Timer.AdjustedTimeLeft / 1000
	lob.BlueBans, lob.RedBans = s.Bans.MyTeamBans, s.Bans.TheirTeamBans

	inProgress := map[int]bool{}
	for _, grp := range s.Actions {
		for _, a := range grp {
			if a.IsInProgress && a.Type == "pick" {
				inProgress[a.ActorCellID] = true
			}
		}
	}
	self := func(c cell) bool { return c.CellID == s.LocalCellID }
	lob.Blue = draftees(s.MyTeam, inProgress, self, true)
	lob.Red = draftees(s.TheirTeam, inProgress, self, false)
}

type cell struct {
	CellID             int    `json:"cellId"`
	ChampionID         int    `json:"championId"`
	ChampionPickIntent int    `json:"championPickIntent"`
	AssignedPosition   string `json:"assignedPosition"`
	Spell1ID           int    `json:"spell1Id"`
	Spell2ID           int    `json:"spell2Id"`
	GameName           string `json:"gameName"`
	SummonerName       string `json:"summonerName"`
}

type act struct {
	ActorCellID  int    `json:"actorCellId"`
	Type         string `json:"type"`
	IsInProgress bool   `json:"isInProgress"`
}

func draftees(cells []cell, inProgress map[int]bool, isSelf func(cell) bool, blue bool) []Draftee {
	out := make([]Draftee, 0, len(cells))
	for i, c := range cells {
		status := "waiting"
		switch {
		case c.ChampionID > 0:
			status = "locked"
		case inProgress[c.CellID]:
			status = "picking"
		case c.ChampionPickIntent > 0:
			status = "hover"
		}
		name := firstNonEmpty(c.GameName, c.SummonerName)
		if name == "" {
			if blue {
				name = fmt.Sprintf("Ally %d", i+1)
			} else {
				name = fmt.Sprintf("Summoner %d", i+1)
			}
		}
		out = append(out, Draftee{
			Name:    name,
			Role:    strings.ToUpper(c.AssignedPosition),
			ChampID: c.ChampionID,
			HoverID: c.ChampionPickIntent,
			Spell1:  c.Spell1ID,
			Spell2:  c.Spell2ID,
			Status:  status,
			You:     isSelf(c),
		})
	}
	return out
}

func saveRaw(name string, b []byte) {
	if SaveRaw == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(SaveRaw, name), b, 0o644)
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// isConnDown reports whether err means nothing is listening on the LCU port
// (connection refused) - i.e. the client is down, not merely slow/busy.
// "connection refused" (unix) / "actively refused" (windows) both contain it.
func isConnDown(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "refused") ||
		strings.Contains(s, "no connection could be made") ||
		strings.Contains(s, "target machine actively refused")
}

func firstNonZero(a ...float64) float64 {
	for _, v := range a {
		if v != 0 {
			return v
		}
	}
	return 0
}

func tagOf(t string) string {
	if t == "" {
		return ""
	}
	return "#" + t
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

func roles(a ...string) []string {
	var out []string
	for _, r := range a {
		r = strings.ToUpper(strings.TrimSpace(r))
		if r == "" || r == "UNSELECTED" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func labelFor(phase string) string {
	switch phase {
	case "Lobby":
		return "In a lobby"
	case "Matchmaking":
		return "In queue"
	case "ReadyCheck":
		return "Match found"
	case "ChampSelect":
		return "In champion select"
	case "InProgress":
		return "In game"
	case "PreEndOfGame", "EndOfGame", "WaitingForStats":
		return "Post-game"
	case "None", "":
		return "In the client"
	}
	return "Online"
}

func phaseLabel(p string) string {
	switch p {
	case "BAN_PICK":
		return "Ban / Pick"
	case "PLANNING":
		return "Declare intent"
	case "FINALIZATION":
		return "Finalizing"
	}
	return "Champion select"
}

var rankedQueues = map[int]bool{420: true, 440: true, 470: true, 1700: false}

func rankedQueue(q int) bool { return rankedQueues[q] }

var queueNames = map[int]string{
	420:  "Ranked Solo/Duo",
	440:  "Ranked Flex",
	400:  "Normal Draft",
	430:  "Normal Blind",
	450:  "ARAM",
	490:  "Quickplay",
	700:  "Clash",
	1700: "Arena",
	1900: "URF",
	900:  "ARURF",
}

func queueName(q int) string {
	if n, ok := queueNames[q]; ok {
		return n
	}
	return "Custom"
}

// matchQueue is a short queue label for the recent-match rows.
var matchQueues = map[int]string{
	420: "Ranked Solo", 440: "Ranked Flex", 400: "Normal", 430: "Normal",
	450: "ARAM", 490: "Quickplay", 700: "Clash", 1700: "Arena", 1900: "URF", 900: "ARURF",
}

func matchQueue(q int) string {
	if n, ok := matchQueues[q]; ok {
		return n
	}
	return "Custom"
}

// cleanTier normalizes an LCU ranked tier ("NONE"/"" → unranked).
func cleanTier(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	if t == "NONE" || t == "UNRANKED" {
		return ""
	}
	return t
}

// cleanDiv normalizes a division ("NA" = no division / unranked).
func cleanDiv(d string) string {
	d = strings.ToUpper(strings.TrimSpace(d))
	if d == "NA" {
		return ""
	}
	return d
}

// titleCase turns "GOLD" into "Gold".
func titleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// availabilityLabel maps an LCU chat availability to a friendly word.
func availabilityLabel(a string) string {
	switch strings.ToLower(a) {
	case "away":
		return "Away"
	case "dnd":
		return "Busy"
	case "mobile":
		return "Mobile"
	case "offline":
		return "Offline"
	}
	return "Online"
}

var mapNames = map[int]string{11: "Summoner's Rift", 12: "Howling Abyss", 30: "Arena"}

func mapName(id int) string {
	if n, ok := mapNames[id]; ok {
		return n
	}
	return "Summoner's Rift"
}

func modeLabel(q int, mode string) string {
	switch q {
	case 420, 440, 400:
		return "Draft Pick"
	case 430:
		return "Blind Pick"
	case 450, 900:
		return "ARAM"
	}
	if mode != "" {
		return strings.Title(strings.ToLower(mode))
	}
	return ""
}
