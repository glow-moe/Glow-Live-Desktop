// Package tft reads Teamfight Tactics from its own client (TFTClient.exe, out
// of the League client since late 2026). Two sources, both read only:
//
//   - your presence in the Riot Client (riotclient): the queue and whether
//     you're in a game, plus your Riot ID;
//   - the game's log (TFT.log): the lobby state, the set, and the champions you
//     buy and star up, from the voice line each one plays.
//
// The log also carries account ids, party ids and the game server; none of
// that is kept, only the fields below. Selling a champion isn't in the log, so
// Units is what you bought this game, not exactly your board.
package tft

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/jsonx"
	"github.com/glow-moe/glow-collector/internal/logtail"
	"github.com/glow-moe/glow-collector/internal/proc"
	"github.com/glow-moe/glow-collector/internal/riotclient"
)

// Phases of State.Phase.
const (
	PhaseMenus    = "menus"
	PhaseQueue    = "queue"
	PhaseInGame   = "inGame"
	PhasePostGame = "postGame"
)

// MaxUnits caps the champions kept for one game.
const MaxUnits = 12

// Unit is a champion you bought this game and the most stars it reached.
type Unit struct {
	Champ string // "Varus", "Akali"
	Stars int
	seen  int // when it was last bought or starred up, for Best
}

// State is the game as far as the client and its log tell.
type State struct {
	Name, Tag string
	Phase     string
	QueueID   int
	Set       int
	Units     []Unit    // in the order you first bought them
	StartedAt time.Time // when this game loaded; zero out of a game
	// Your Little Legend as the client names its art
	// ("StyleTwoAhri_Base_Classic_Tier1"), seen in the lobby.
	Companion string
	// Your arena as its art is named ("Backstage_Day"), from the world that
	// loads before a game starts; kept for later games, which load it silently.
	Arena string

	tick int
}

var (
	tsRe    = regexp.MustCompile(`^\[(\d{4})\.(\d{2})\.(\d{2})-(\d{2})\.(\d{2})\.(\d{2}):(\d{3})\]`)
	lobbyRe = regexp.MustCompile(`Entering LobbyState\[ETFTLobbyState::([A-Za-z]+)\]`)
	queueRe = regexp.MustCompile(`ActiveQueueId=\[(\d{1,5})\]`)
	actorRe = regexp.MustCompile(`\{DA_(\d{1,2})_([A-Z][A-Za-z0-9]{1,30})`)
	starRe  = regexp.MustCompile(`VO\.Unit\.StarUp\.(\d)Star`)
	modeRe  = regexp.MustCompile(`queueId_(\d{1,5})`)
	// Only lobby lines ("[Standalone]"): in a game every player's Little
	// Legend is on the map.
	// "/Arena_Backstage/Day/SubLevels/...": the arena and its time of day.
	arenaRe  = regexp.MustCompile(`/Arena_([A-Za-z0-9]{2,40})/([A-Za-z0-9]{1,30})/`)
	petRe    = regexp.MustCompile(`\[Standalone\].*BP_Pet[A-Za-z0-9_]*_C_\d+ \{DA_([A-Za-z0-9]+_[A-Za-z0-9_]{1,80}_Tier\d)\}`)
	soundMod = "TFTAudioEventSubsystem"
)

// feed moves the state along the given log lines.
func (s *State) feed(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if m := lobbyRe.FindStringSubmatch(line); m != nil {
			s.enter(m[1], stamp(line))
			continue
		}
		if m := queueRe.FindStringSubmatch(line); m != nil && strings.Contains(line, "TFTLobbySubsystem") {
			s.QueueID, _ = strconv.Atoi(m[1])
			continue
		}
		if m := petRe.FindStringSubmatch(line); m != nil {
			s.Companion = m[1]
			continue
		}
		// Only while a game loads: in one, opponents' arenas load too.
		if m := arenaRe.FindStringSubmatch(line); m != nil && s.Phase != PhaseInGame && m[1] != "Shared" && strings.Contains(line, "AddToWorld") {
			s.Arena = m[1]
			if m[2] != "SubLevels" && m[2] != "Maps" {
				s.Arena += "_" + m[2]
			}
			continue
		}
		if s.Phase == PhaseInGame && strings.Contains(line, soundMod) {
			s.sound(line)
		}
	}
}

// enter is a new lobby state: InGame starts a game over, the end of game
// screens keep what the game had.
func (s *State) enter(state string, at time.Time) {
	switch {
	case state == "InGame":
		s.Phase, s.Units, s.StartedAt = PhaseInGame, nil, at
	case strings.HasPrefix(state, "EndOfGame"):
		s.Phase, s.StartedAt = PhasePostGame, time.Time{}
	case strings.Contains(state, "Matchmak") || strings.Contains(state, "Queue") || strings.Contains(state, "ChampSelect"):
		s.Phase, s.Units, s.StartedAt = PhaseQueue, nil, time.Time{}
	default:
		s.Phase, s.Units, s.StartedAt = PhaseMenus, nil, time.Time{}
	}
}

// sound reads a unit voice line: a purchase adds the champion, a star up
// raises it. Lines that were cut off by another voice line still count.
func (s *State) sound(line string) {
	purchase := strings.Contains(line, "VO.Unit.Purchase")
	star := starRe.FindStringSubmatch(line)
	if !purchase && star == nil {
		return
	}
	a := actorRe.FindStringSubmatch(line)
	if a == nil {
		return
	}
	if set, _ := strconv.Atoi(a[1]); set > 0 {
		s.Set = set
	}
	stars := 1
	if star != nil {
		stars, _ = strconv.Atoi(star[1])
	}
	s.tick++
	for i := range s.Units {
		if s.Units[i].Champ == a[2] {
			s.Units[i].Stars = max(s.Units[i].Stars, stars)
			s.Units[i].seen = s.tick
			return
		}
	}
	if len(s.Units) < MaxUnits {
		s.Units = append(s.Units, Unit{Champ: a[2], Stars: stars, seen: s.tick})
	}
}

// Best is up to n champions, most stars first, then the latest bought or
// starred up (one sold earlier sinks).
func (s State) Best(n int) []Unit {
	out := append([]Unit(nil), s.Units...)
	ahead := func(a, b Unit) bool { return a.Stars > b.Stars || (a.Stars == b.Stars && a.seen > b.seen) }
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && ahead(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// presence fills what the log doesn't have from the Riot Client's presence
// blob: the queue, and "in game" when the log is behind.
func (s *State) presence(packed []byte) {
	var p struct {
		Activity struct {
			Mode     string `json:"mode"`
			Progress struct {
				Value string `json:"value"`
			} `json:"progress"`
		} `json:"activity"`
	}
	if jsonx.Unmarshal(packed, &p) != nil {
		return
	}
	if m := modeRe.FindStringSubmatch(p.Activity.Mode); m != nil && s.QueueID == 0 {
		s.QueueID, _ = strconv.Atoi(m[1])
	}
	if p.Activity.Progress.Value == "InGame" && s.Phase != PhaseInGame && s.Phase != PhasePostGame {
		s.Phase = PhaseInGame
	}
}

// stamp is the line's own time (the log writes UTC), or zero.
func stamp(line string) time.Time {
	m := tsRe.FindStringSubmatch(line)
	if m == nil {
		return time.Time{}
	}
	n := make([]int, 7)
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
	}
	return time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], n[6]*int(time.Millisecond), time.UTC)
}

func logPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "TFT", "Saved", "Logs", "TFT.log")
}

var (
	mu       sync.Mutex
	tail     logtail.Follower
	state    State
	headRead bool
)

// Current is the TFT state, or ok=false when the game isn't open.
func Current() (State, bool) {
	if !proc.Running("TFTClient-Win64-Shipping.exe") {
		return State{}, false
	}
	mu.Lock()
	defer mu.Unlock()
	if p := logPath(); p != "" {
		_ = tail.Read(p, func(lines io.Reader, fresh bool) {
			if fresh {
				state, headRead = State{}, false
			}
			state.feed(lines)
		})
		// The Little Legend and the arena are written early (the lobby, the
		// first game's load). When the first look started further in, read
		// them from the head of the log, once per log.
		if (state.Companion == "" || state.Arena == "") && !headRead {
			headRead = true
			early := fromHead(p)
			if state.Companion == "" {
				state.Companion = early.Companion
			}
			if state.Arena == "" {
				state.Arena = early.Arena
			}
		}
	}
	out := state
	out.Units = append([]Unit(nil), state.Units...)
	if out.Phase == "" {
		out.Phase = PhaseMenus
	}
	if l, err := riotclient.Open(); err == nil {
		if self, err := l.Self(); err == nil {
			out.Name, out.Tag = self.Name, self.Tag
			if p, ok := l.Own(self, "teamfighttactics"); ok {
				out.presence(riotclient.Decode(p.PackedData))
			}
		}
	}
	return out, true
}

// headLimit is how much of the log fromHead reads.
const headLimit = 32 << 20

// fromHead runs the start of the log through a fresh state, for what it says
// before the part the follower read.
func fromHead(path string) State {
	var s State
	fh, err := os.Open(path)
	if err != nil {
		return s
	}
	defer fh.Close()
	s.feed(io.LimitReader(fh, headLimit))
	return s
}
