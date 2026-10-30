package valorant

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The match you're in, read from Riot's game servers with your own Riot Client
// session: the same requests the VALORANT client makes for its own agent
// select, scoreboard and match history. Nothing touches the game process.
//
// What leaves the app per player is only what the game shows you: team, agent
// (and whether it's locked), card, title, level border, level unless they hide
// it, and name unless they play incognito. Then the server's name, and after
// the match each player's K/D/A, score and rounds. Never ranks, never puuids.
// glow.moe switches the whole thing on or off (settings.valorantMatch).

// Player is one player in agent select or the match.
type Player struct {
	Team   string `json:"team"`
	Agent  string `json:"agent,omitempty"`
	Locked bool   `json:"locked,omitempty"`
	Card   string `json:"card,omitempty"`
	Title  string `json:"title,omitempty"`
	Border string `json:"border,omitempty"`
	Level  int    `json:"level,omitempty"`
	Name   string `json:"name,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Self   bool   `json:"self,omitempty"`
	// Party only: ready to queue, and who leads.
	Ready bool `json:"ready,omitempty"`
	Owner bool `json:"owner,omitempty"`

	puuid  string
	hidden bool
}

// Match is agent select (your team only, the game hides the other) or the match.
type Match struct {
	Map     string   `json:"map"`
	Mode    string   `json:"mode,omitempty"`
	Queue   string   `json:"queue,omitempty"`
	Pod     string   `json:"pod,omitempty"`
	Enemies int      `json:"enemies,omitempty"`
	Players []Player `json:"players"`

	id   string
	live bool // the match itself, not agent select
}

// Party is the lobby you're in: who's with you and whether it's open.
type Party struct {
	Open    bool     `json:"open,omitempty"`
	Queue   string   `json:"queue,omitempty"`
	Members []Player `json:"members"`
}

// FinalPlayer is one line of the scoreboard.
type FinalPlayer struct {
	Team    string `json:"team,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Card    string `json:"card,omitempty"`
	Name    string `json:"name,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Self    bool   `json:"self,omitempty"`
	Kills   int    `json:"kills"`
	Deaths  int    `json:"deaths"`
	Assists int    `json:"assists"`
	Score   int    `json:"score"`
	Rounds  int    `json:"rounds"`
}

// FinalTeam is one team's result ("Red" / "Blue"; deathmatch has none).
type FinalTeam struct {
	Team   string `json:"team"`
	Won    bool   `json:"won"`
	Rounds int    `json:"rounds"`
}

// Final is the scoreboard of the match you just finished.
type Final struct {
	Map      string        `json:"map"`
	Queue    string        `json:"queue,omitempty"`
	Mode     string        `json:"mode,omitempty"`
	Pod      string        `json:"pod,omitempty"`
	EndedAt  int64         `json:"endedAt"`
	LengthMs int64         `json:"lengthMs"`
	Teams    []FinalTeam   `json:"teams,omitempty"`
	Players  []FinalPlayer `json:"players"`
}

// Tracker keeps the current match and the last scoreboard. Tick starts a read
// in the background when one is due; Latest is what goes in the next push.
type Tracker struct {
	mu      sync.Mutex
	busy    bool
	id      string // the match the names below belong to
	match   *Match
	readAt  time.Time
	final   *Final
	pending string    // finished match whose scoreboard isn't in yet
	endAt   time.Time // when it finished
	tryAt   time.Time // next scoreboard try
	names   map[string][2]string
	hidden  map[string]bool
	party   *Party
	partyAt time.Time
	pnames  map[string][2]string // party members' names, kept across matches
}

// Latest returns the match (nil outside one) and the last scoreboard.
func (t *Tracker) Latest() (*Match, *Final) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.match, t.final
}

// LatestParty is the lobby (nil outside the menus).
func (t *Tracker) LatestParty() *Party {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.party
}

// Tick reads what's due for this presence, in the background.
func (t *Tracker) Tick(st *State) {
	now := time.Now()
	t.mu.Lock()
	if t.busy {
		t.mu.Unlock()
		return
	}
	due := false
	switch st.Phase {
	case "PREGAME":
		due = now.Sub(t.readAt) >= 3*time.Second
	case "INGAME":
		due = t.match == nil || now.Sub(t.readAt) >= time.Minute
	default:
		if t.match != nil && t.match.live {
			// Back in the menus: the match is over, its scoreboard comes next.
			t.pending, t.endAt, t.tryAt = t.match.id, now, now.Add(15*time.Second)
		}
		t.match = nil
		if t.pending != "" && now.Sub(t.endAt) > 10*time.Minute {
			t.pending = ""
		}
		due = (t.pending != "" && !now.Before(t.tryAt)) || now.Sub(t.partyAt) >= 10*time.Second
	}
	if st.Phase == "PREGAME" || st.Phase == "INGAME" {
		t.party = nil
	}
	if !due {
		t.mu.Unlock()
		return
	}
	t.busy, t.readAt = true, now
	t.mu.Unlock()
	go func() {
		defer func() {
			t.mu.Lock()
			t.busy = false
			t.mu.Unlock()
		}()
		t.read(st)
	}()
}

func (t *Tracker) read(st *State) {
	s, err := st.session()
	if err != nil {
		return
	}
	if st.Phase == "PREGAME" || st.Phase == "INGAME" {
		var m *Match
		if st.Phase == "PREGAME" {
			m, err = s.pregame()
		} else {
			m, err = s.coreGame()
		}
		if err != nil || m == nil {
			return
		}
		t.mu.Lock()
		if t.id != m.id {
			// A new match: forget the last one's names and scoreboard.
			t.id, t.names, t.hidden = m.id, map[string][2]string{}, map[string]bool{}
			t.final, t.pending = nil, ""
		}
		var ask []string
		for _, p := range m.Players {
			t.hidden[p.puuid] = p.hidden && !p.Self
			if _, ok := t.names[p.puuid]; !ok && !t.hidden[p.puuid] {
				ask = append(ask, p.puuid)
			}
		}
		t.mu.Unlock()
		if len(ask) > 0 {
			got := s.names(ask)
			t.mu.Lock()
			for id, n := range got {
				t.names[id] = n
			}
			t.mu.Unlock()
		}
		t.mu.Lock()
		for i := range m.Players {
			p := &m.Players[i]
			if n, ok := t.names[p.puuid]; ok && !t.hidden[p.puuid] {
				p.Name, p.Tag = n[0], n[1]
			}
		}
		t.match = m
		t.mu.Unlock()
		return
	}

	t.mu.Lock()
	id := t.pending
	hidden := t.hidden
	wantFinal := id != "" && !time.Now().Before(t.tryAt)
	wantParty := time.Since(t.partyAt) >= 10*time.Second
	if wantParty {
		t.partyAt = time.Now()
	}
	t.mu.Unlock()
	if wantParty {
		t.readParty(s)
	}
	if !wantFinal {
		return
	}
	f, done := s.details(id, hidden)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending != id {
		return
	}
	if done {
		t.final, t.pending = f, ""
	} else {
		t.tryAt = time.Now().Add(20 * time.Second)
	}
}

func (t *Tracker) readParty(s *session) {
	p, err := s.party()
	if err != nil {
		t.mu.Lock()
		t.party = nil
		t.mu.Unlock()
		return
	}
	t.mu.Lock()
	if t.pnames == nil {
		t.pnames = map[string][2]string{}
	}
	var ask []string
	for _, m := range p.Members {
		if _, ok := t.pnames[m.puuid]; !ok && (!m.hidden || m.Self) {
			ask = append(ask, m.puuid)
		}
	}
	t.mu.Unlock()
	got := map[string][2]string{}
	if len(ask) > 0 {
		got = s.names(ask)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, n := range got {
		t.pnames[id] = n
	}
	for i := range p.Members {
		m := &p.Members[i]
		if n, ok := t.pnames[m.puuid]; ok && (!m.hidden || m.Self) {
			m.Name, m.Tag = n[0], n[1]
		}
	}
	t.party = p
}

// ── the session ─────────────────────────────────────────────────────────────

type session struct {
	puuid, access, entitlement, version, glz, pd string
}

var remote = &http.Client{Timeout: 6 * time.Second}

// riotPlatform is the client platform header the game itself sends (PC).
const riotPlatform = "ew0KCSJwbGF0Zm9ybVR5cGUiOiAiUEMiLA0KCSJwbGF0Zm9ybU9TIjogIldpbmRvd3MiLA0KCSJwbGF0Zm9ybU9TVmVyc2lvbiI6ICIxMC4wLjE5MDQyLjEuMjU2LjY0Yml0IiwNCgkicGxhdGZvcm1DaGlwc2V0IjogIlVua25vd24iDQp9"

func (st *State) session() (*session, error) {
	if st.port == "" {
		return nil, ErrNoGame
	}
	var ent struct {
		AccessToken string `json:"accessToken"`
		Token       string `json:"token"`
		Subject     string `json:"subject"`
	}
	if err := get(st.port, st.password, "/entitlements/v1/token", 64<<10, &ent); err != nil || ent.AccessToken == "" || ent.Token == "" {
		return nil, ErrNoGame
	}
	h := readHosts()
	if h.version == "" || h.glz == "" || h.pd == "" {
		return nil, errors.New("valorant: servers not known yet")
	}
	return &session{puuid: ent.Subject, access: ent.AccessToken, entitlement: ent.Token, version: h.version, glz: h.glz, pd: h.pd}, nil
}

func (s *session) do(method, url string, body any, max int64, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.access)
	req.Header.Set("X-Riot-Entitlements-JWT", s.entitlement)
	req.Header.Set("X-Riot-ClientVersion", s.version)
	req.Header.Set("X-Riot-ClientPlatform", riotPlatform)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := remote.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, max)).Decode(out)
}

// ── which servers: the game's own log ─────────────────────────────────────────

type hosts struct{ version, glz, pd string }

var (
	hostMu    sync.Mutex
	hostCache hosts
	hostSize  int64

	reVersion = regexp.MustCompile(`CI server version: (release-[A-Za-z0-9.\-]+)`)
	reGLZ     = regexp.MustCompile(`https://(glz-[a-z0-9]+-1\.[a-z0-9]+\.a\.pvp\.net)`)
	rePD      = regexp.MustCompile(`https://(pd\.[a-z0-9]+\.a\.pvp\.net)`)
)

// readHosts reads the client version and the account's game servers from
// ShooterGame.log, which the client writes as it talks to them. Re-read when
// the log has grown and something is still missing (the game server host
// shows up with the first agent select of the session).
func readHosts() hosts {
	hostMu.Lock()
	defer hostMu.Unlock()
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return hosts{}
	}
	path := filepath.Join(base, "VALORANT", "Saved", "Logs", "ShooterGame.log")
	fi, err := os.Stat(path)
	if err != nil {
		return hosts{}
	}
	if fi.Size() < hostSize {
		hostCache = hosts{} // a new session's log
	}
	full := hostCache.version != "" && hostCache.glz != "" && hostCache.pd != ""
	if full || fi.Size() == hostSize || fi.Size() > 64<<20 {
		return hostCache
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return hostCache
	}
	hostSize = fi.Size()
	if m := reVersion.FindSubmatch(b); m != nil {
		hostCache.version = string(m[1])
	}
	if all := reGLZ.FindAllSubmatch(b, -1); len(all) > 0 {
		hostCache.glz = "https://" + string(all[len(all)-1][1])
	}
	if all := rePD.FindAllSubmatch(b, -1); len(all) > 0 {
		hostCache.pd = "https://" + string(all[len(all)-1][1])
	}
	return hostCache
}

// ── reads ─────────────────────────────────────────────────────────────────────

type identity struct {
	PlayerCardID           string `json:"PlayerCardID"`
	PlayerTitleID          string `json:"PlayerTitleID"`
	AccountLevel           int    `json:"AccountLevel"`
	PreferredLevelBorderID string `json:"PreferredLevelBorderID"`
	Incognito              bool   `json:"Incognito"`
	HideAccountLevel       bool   `json:"HideAccountLevel"`
}

func (s *session) player(puuid, team, agent string, locked bool, id identity) Player {
	self := puuid == s.puuid
	p := Player{
		Team: team, Agent: uuidOr(agent), Locked: locked, Self: self,
		Card: uuidOr(id.PlayerCardID), Title: uuidOr(id.PlayerTitleID), Border: uuidOr(id.PreferredLevelBorderID),
		puuid: puuid, hidden: id.Incognito,
	}
	if self || !id.HideAccountLevel {
		p.Level = id.AccountLevel
	}
	return p
}

func (s *session) pregame() (*Match, error) {
	var who struct {
		MatchID string `json:"MatchID"`
	}
	if err := s.do(http.MethodGet, s.glz+"/pregame/v1/players/"+s.puuid, nil, 64<<10, &who); err != nil || who.MatchID == "" {
		return nil, errors.New("no agent select")
	}
	var r struct {
		ID       string `json:"ID"`
		AllyTeam *struct {
			TeamID  string `json:"TeamID"`
			Players []struct {
				Subject                 string   `json:"Subject"`
				CharacterID             string   `json:"CharacterID"`
				CharacterSelectionState string   `json:"CharacterSelectionState"`
				PlayerIdentity          identity `json:"PlayerIdentity"`
			} `json:"Players"`
		} `json:"AllyTeam"`
		EnemyTeamSize int    `json:"EnemyTeamSize"`
		MapID         string `json:"MapID"`
		Mode          string `json:"Mode"`
		GamePodID     string `json:"GamePodID"`
		QueueID       string `json:"QueueID"`
	}
	if err := s.do(http.MethodGet, s.glz+"/pregame/v1/matches/"+who.MatchID, nil, 1<<20, &r); err != nil {
		return nil, err
	}
	m := &Match{id: who.MatchID, Map: r.MapID, Mode: r.Mode, Queue: r.QueueID, Pod: r.GamePodID, Enemies: r.EnemyTeamSize}
	if r.AllyTeam != nil {
		for _, p := range r.AllyTeam.Players {
			m.Players = append(m.Players, s.player(p.Subject, r.AllyTeam.TeamID, p.CharacterID, p.CharacterSelectionState == "locked", p.PlayerIdentity))
		}
	}
	return m, nil
}

func (s *session) coreGame() (*Match, error) {
	var who struct {
		MatchID string `json:"MatchID"`
	}
	if err := s.do(http.MethodGet, s.glz+"/core-game/v1/players/"+s.puuid, nil, 64<<10, &who); err != nil || who.MatchID == "" {
		return nil, errors.New("no match")
	}
	var r struct {
		MapID           string `json:"MapID"`
		ModeID          string `json:"ModeID"`
		GamePodID       string `json:"GamePodID"`
		MatchmakingData *struct {
			QueueID string `json:"QueueID"`
		} `json:"MatchmakingData"`
		Players []struct {
			Subject        string   `json:"Subject"`
			TeamID         string   `json:"TeamID"`
			CharacterID    string   `json:"CharacterID"`
			IsCoach        bool     `json:"IsCoach"`
			PlayerIdentity identity `json:"PlayerIdentity"`
		} `json:"Players"`
	}
	if err := s.do(http.MethodGet, s.glz+"/core-game/v1/matches/"+who.MatchID, nil, 1<<20, &r); err != nil {
		return nil, err
	}
	m := &Match{id: who.MatchID, live: true, Map: r.MapID, Mode: r.ModeID, Pod: r.GamePodID}
	if r.MatchmakingData != nil {
		m.Queue = r.MatchmakingData.QueueID
	}
	for _, p := range r.Players {
		if !p.IsCoach {
			m.Players = append(m.Players, s.player(p.Subject, p.TeamID, p.CharacterID, true, p.PlayerIdentity))
		}
	}
	return m, nil
}

func (s *session) party() (*Party, error) {
	var who struct {
		CurrentPartyID string `json:"CurrentPartyID"`
	}
	if err := s.do(http.MethodGet, s.glz+"/parties/v1/players/"+s.puuid, nil, 64<<10, &who); err != nil || who.CurrentPartyID == "" {
		return nil, errors.New("no party")
	}
	var r struct {
		Accessibility   string `json:"Accessibility"`
		MatchmakingData *struct {
			QueueID string `json:"QueueID"`
		} `json:"MatchmakingData"`
		Members []struct {
			Subject        string   `json:"Subject"`
			IsOwner        bool     `json:"IsOwner"`
			IsReady        bool     `json:"IsReady"`
			PlayerIdentity identity `json:"PlayerIdentity"`
		} `json:"Members"`
	}
	if err := s.do(http.MethodGet, s.glz+"/parties/v1/parties/"+who.CurrentPartyID, nil, 1<<20, &r); err != nil {
		return nil, err
	}
	p := &Party{Open: r.Accessibility == "OPEN"}
	if r.MatchmakingData != nil {
		p.Queue = r.MatchmakingData.QueueID
	}
	for _, m := range r.Members {
		pl := s.player(m.Subject, "", "", false, m.PlayerIdentity)
		pl.Ready, pl.Owner = m.IsReady, m.IsOwner
		p.Members = append(p.Members, pl)
	}
	if len(p.Members) > 5 {
		p.Members = p.Members[:5]
	}
	return p, nil
}

// names looks up Riot IDs (only ever asked for players who don't hide theirs).
func (s *session) names(puuids []string) map[string][2]string {
	var r []struct {
		Subject  string `json:"Subject"`
		GameName string `json:"GameName"`
		TagLine  string `json:"TagLine"`
	}
	out := map[string][2]string{}
	if s.do(http.MethodPut, s.pd+"/name-service/v2/players", puuids, 256<<10, &r) != nil {
		return out
	}
	for _, n := range r {
		out[n.Subject] = [2]string{n.GameName, n.TagLine}
	}
	return out
}

// details is the finished match's scoreboard; done is false while Riot is
// still writing it up. Names stay off for anyone who played incognito, and for
// anyone the app never saw in the match.
func (s *session) details(id string, hidden map[string]bool) (*Final, bool) {
	var r struct {
		MatchInfo struct {
			MapID            string `json:"mapId"`
			GamePodID        string `json:"gamePodId"`
			GameLengthMillis int64  `json:"gameLengthMillis"`
			GameStartMillis  int64  `json:"gameStartMillis"`
			QueueID          string `json:"queueID"`
			GameMode         string `json:"gameMode"`
			IsCompleted      bool   `json:"isCompleted"`
		} `json:"matchInfo"`
		Players []struct {
			Subject     string `json:"subject"`
			GameName    string `json:"gameName"`
			TagLine     string `json:"tagLine"`
			TeamID      string `json:"teamId"`
			CharacterID string `json:"characterId"`
			PlayerCard  string `json:"playerCard"`
			IsObserver  bool   `json:"isObserver"`
			Stats       *struct {
				Score        int `json:"score"`
				RoundsPlayed int `json:"roundsPlayed"`
				Kills        int `json:"kills"`
				Deaths       int `json:"deaths"`
				Assists      int `json:"assists"`
			} `json:"stats"`
		} `json:"players"`
		Teams []struct {
			TeamID    string `json:"teamId"`
			Won       bool   `json:"won"`
			RoundsWon int    `json:"roundsWon"`
		} `json:"teams"`
	}
	// Not there yet (404) or a blip: try again on the next round.
	if err := s.do(http.MethodGet, s.pd+"/match-details/v1/matches/"+id, nil, 8<<20, &r); err != nil {
		return nil, false
	}
	if !r.MatchInfo.IsCompleted {
		return nil, false
	}
	team := func(t string) string {
		if t == "Red" || t == "Blue" {
			return t
		}
		return "" // deathmatch: every player is their own team
	}
	f := &Final{
		Map: r.MatchInfo.MapID, Queue: r.MatchInfo.QueueID, Mode: r.MatchInfo.GameMode, Pod: r.MatchInfo.GamePodID,
		EndedAt: r.MatchInfo.GameStartMillis + r.MatchInfo.GameLengthMillis, LengthMs: r.MatchInfo.GameLengthMillis,
	}
	for _, t := range r.Teams {
		if team(t.TeamID) != "" {
			f.Teams = append(f.Teams, FinalTeam{Team: t.TeamID, Won: t.Won, Rounds: t.RoundsWon})
		}
	}
	for _, p := range r.Players {
		if p.IsObserver || p.Stats == nil {
			continue
		}
		fp := FinalPlayer{
			Team: team(p.TeamID), Agent: uuidOr(p.CharacterID), Card: uuidOr(p.PlayerCard), Self: p.Subject == s.puuid,
			Kills: p.Stats.Kills, Deaths: p.Stats.Deaths, Assists: p.Stats.Assists, Score: p.Stats.Score, Rounds: p.Stats.RoundsPlayed,
		}
		if hide, seen := hidden[p.Subject]; fp.Self || (seen && !hide) {
			fp.Name, fp.Tag = p.GameName, p.TagLine
		}
		f.Players = append(f.Players, fp)
	}
	return f, true
}

var reUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func uuidOr(s string) string {
	if reUUID.MatchString(s) && s != "00000000-0000-0000-0000-000000000000" {
		return strings.ToLower(s)
	}
	return ""
}
