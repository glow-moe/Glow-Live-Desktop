// Package fortnite works out what's happening in Fortnite from the game's own
// log (FortniteGame.log), the way roblox reads the Roblox client's: which
// mode, the match phase, and Fortnite's own status line, the one your Epic
// friends see ("Fortnite OG Zero Solo - 73 Left"). Read-only; the game server
// address in the log is never used or shared.
package fortnite

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

	"github.com/glow-moe/glow-collector/internal/logtail"
	"github.com/glow-moe/glow-collector/internal/proc"
)

// State is what the log has said so far.
type State struct {
	InMatch  bool
	Map      string    // map asset, "Athena_Terrain_S10"
	Playlist string    // "Playlist_FigmentNoBuildSolo"
	Phase    string    // Setup, Warmup, Aircraft, SafeZones, EndGame
	Status   string    // Fortnite's own status line
	Island   string    // the selected experience or island code
	JoinedAt time.Time // when the match loaded
	Last     *Final    // the match before, once you're back in the lobby
	Loadout  Loadout   // what you have equipped
	Name     string    // your Epic display name (glow.moe looks up public stats by it)
	ID       string    // your Epic account id
	Party    []Member  // everyone in your lobby, you included, in join order

	ids map[string]bool // full Epic account ids the log has named

	// a loadout block being read ("--- [LoadoutSlot.X]=..." lines under a
	// "Local Player ... loadout changed" line)
	inBlock bool
	slot    string
}

// Member is someone in the lobby: the name Fortnite shows (a console name
// for console players) and their Epic account id, which names them for sure.
type Member struct {
	Name string
	ID   string // full id when the log had it, else the short "7dbbe...f57b3"
}

// Loadout is the local player's equipped cosmetics, as Fortnite item ids
// ("character_hammervice"); the site names them.
type Loadout struct {
	Character string
	Backpack  string
	Pickaxe   string
	Glider    string
	Contrail  string
	Pet       string
	PetName   string // the name you gave your sidekick
}

// Final is a finished match: the mode and the last status line it had (which
// carries how many were left when you went out).
type Final struct {
	Playlist string
	Status   string
	Left     int
	JoinedAt time.Time
	EndedAt  time.Time
}

var (
	tsRe       = regexp.MustCompile(`^\[(\d{4})\.(\d{2})\.(\d{2})-(\d{2})\.(\d{2})\.(\d{2}):(\d{3})\]`)
	loadMapRe  = regexp.MustCompile(`LogLoad: LoadMap: (\S+)`)
	phaseRe    = regexp.MustCompile(`NewPhase = EAthenaGamePhase::([A-Za-z]+)`)
	playlistRe = regexp.MustCompile(`PlaylistName is (Playlist_[A-Za-z0-9_]+)`)
	statusRe   = regexp.MustCompile(`LogEOSPresence: Updating Presence to \[[A-Za-z]+\].*RichText=\[([^\]]*)\]`)
	islandRe   = regexp.MustCompile(`Link id changing from .* to \[Mnemonic=\[([^\]]+)\]`)
	leftRe     = regexp.MustCompile(`(\d+) Left\s*$`)
	slotRe     = regexp.MustCompile(`^--- \[LoadoutSlot\.([A-Za-z.]+)\]=[A-Za-z]+:([a-z0-9_]{2,80})`)
	nameRe     = regexp.MustCompile(`MatchmakingLog: \[[0-9a-f]+\] (.{3,32}?) is now Eligible for matchmaking`)
	joinRe     = regexp.MustCompile(`LogParty: Verbose: Adding \[(.{1,32}?)\] Id \[MCP:([0-9a-f.]+)\] as a new member`)
	idRe       = regexp.MustCompile(`(?:TargetAccountId|LocalAccountId)=\[([0-9a-f]{32})\]`)
	localIDRe  = regexp.MustCompile(`LocalAccountId=\[([0-9a-f]{32})\]`)
	partRe     = regexp.MustCompile(`LogParty: Verbose: Removing \[(.{1,32}?)\] Id \[MCP:[0-9a-f.]+\] from`)
	petNameRe  = regexp.MustCompile(`^\s*\{Channel: CustomName - Variant: Text - Data: ([^}]{1,32})\}`)
)

// feed moves the state along the given log lines.
func (s *State) feed(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if s.inBlock {
			if m := slotRe.FindStringSubmatch(line); m != nil {
				s.slot = m[1]
				s.equip(m[1], m[2])
				continue
			}
			if m := petNameRe.FindStringSubmatch(line); m != nil && s.slot == "Mimosa.Main" {
				s.Loadout.PetName = strings.TrimSpace(m[1])
				continue
			}
			if strings.HasPrefix(line, "[") {
				s.inBlock = false
			} else {
				continue
			}
		}
		if strings.Contains(line, "LogFortPlayerLoadout:") && strings.Contains(line, "Local Player") && strings.Contains(line, "loadout changed") {
			s.inBlock, s.slot, s.Loadout = true, "", Loadout{}
			continue
		}
		if m := loadMapRe.FindStringSubmatch(line); m != nil {
			path := m[1]
			if i := strings.IndexByte(path, '?'); i >= 0 {
				path = path[:i]
			}
			if strings.Contains(path, "/Game/Maps/Frontend") {
				// Back in the lobby: the match (if any) is over.
				if s.InMatch {
					f := &Final{Playlist: s.Playlist, Status: s.Status, JoinedAt: s.JoinedAt, EndedAt: stamp(line)}
					if m := leftRe.FindStringSubmatch(s.Status); m != nil {
						f.Left, _ = strconv.Atoi(m[1])
					}
					s.Last = f
				}
				s.InMatch, s.Map, s.Playlist, s.Phase, s.JoinedAt = false, "", "", "", time.Time{}
				continue
			}
			// "<server>:<port>/<plugin>/<map>": a match. The server stays here.
			if i := strings.IndexByte(path, '/'); i > 0 && strings.Contains(path[:i], ":") {
				s.InMatch, s.Map, s.Playlist, s.Phase, s.JoinedAt = true, filepath.Base(path[i:]), "", "", stamp(line)
			}
			continue
		}
		if m := phaseRe.FindStringSubmatch(line); m != nil && s.InMatch {
			s.Phase = m[1]
			continue
		}
		if m := playlistRe.FindStringSubmatch(line); m != nil && s.InMatch {
			s.Playlist = m[1]
			continue
		}
		if m := statusRe.FindStringSubmatch(line); m != nil {
			if t := strings.Join(strings.Fields(m[1]), " "); t != "" {
				s.Status = t
			}
			continue
		}
		if m := islandRe.FindStringSubmatch(line); m != nil {
			s.Island = m[1]
			continue
		}
		if m := nameRe.FindStringSubmatch(line); m != nil {
			s.Name = m[1]
			continue
		}
		if m := joinRe.FindStringSubmatch(line); m != nil {
			s.Party = append(without(s.Party, m[1]), Member{Name: m[1], ID: m[2]})
			continue
		}
		if m := partRe.FindStringSubmatch(line); m != nil {
			s.Party = without(s.Party, m[1])
			continue
		}
		if strings.Contains(line, "AccountId=[") {
			if m := localIDRe.FindStringSubmatch(line); m != nil {
				s.ID = m[1]
			}
			for _, m := range idRe.FindAllStringSubmatch(line, -1) {
				if s.ids == nil {
					s.ids = map[string]bool{}
				}
				if len(s.ids) < 200 {
					s.ids[m[1]] = true
				}
			}
		}
	}
}

func without(list []Member, name string) []Member {
	out := list[:0:0]
	for _, x := range list {
		if x.Name != name {
			out = append(out, x)
		}
	}
	return out
}

// Others is the lobby minus you, each with their full account id when the
// log named it ("7dbbe...f57b3" matched against the full ids it has seen).
func (s State) Others() []Member {
	var out []Member
	for _, m := range s.Party {
		if m.Name == s.Name || (s.ID != "" && full(m.ID, s.ID)) {
			continue
		}
		if len(m.ID) != 32 {
			id := ""
			for f := range s.ids {
				if full(m.ID, f) {
					id = f
					break
				}
			}
			m.ID = id
		}
		out = append(out, m)
	}
	return out
}

// full: does the short id ("7dbbe...f57b3") stand for this full one?
func full(short, id string) bool {
	if short == id {
		return true
	}
	head, tail, ok := strings.Cut(short, "...")
	return ok && head != "" && tail != "" && strings.HasPrefix(id, head) && strings.HasSuffix(id, tail)
}

// equip puts one slot's item in the loadout (other slots are not shown).
func (s *State) equip(slot, id string) {
	switch slot {
	case "Character":
		s.Loadout.Character = id
	case "Backpack":
		s.Loadout.Backpack = id
	case "Pickaxe":
		s.Loadout.Pickaxe = id
	case "Glider":
		s.Loadout.Glider = id
	case "Contrails":
		s.Loadout.Contrail = id
	case "Mimosa.Main":
		s.Loadout.Pet = id
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
	return filepath.Join(base, "FortniteGame", "Saved", "Logs", "FortniteGame.log")
}

var (
	mu       sync.Mutex
	tail     logtail.Follower
	state    State
	headRead bool
)

// Current is Fortnite's state, or ok=false when the game isn't open.
func Current() (State, bool) {
	if !proc.Running("FortniteClient-Win64-Shipping.exe", "FortniteClient-Win64-Shipping_EAC_EOS.exe") {
		return State{}, false
	}
	p := logPath()
	if p == "" {
		return State{}, false
	}
	mu.Lock()
	defer mu.Unlock()
	err := tail.Read(p, func(lines io.Reader, fresh bool) {
		if fresh {
			state, headRead = State{}, false
		}
		state.feed(lines)
	})
	if err != nil {
		return State{}, false
	}
	// The name is written once, at startup: when the first look started
	// further in, read it from the head of the log (once per log).
	if state.Name == "" && !headRead {
		headRead = true
		state.Name = nameFromHead(p)
	}
	return state, true
}

// nameFromHead finds the Epic display name in the first few MB of the log.
func nameFromHead(path string) string {
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()
	buf := make([]byte, 8<<20)
	n, _ := io.ReadFull(fh, buf)
	if m := nameRe.FindSubmatch(buf[:n]); m != nil {
		return string(m[1])
	}
	return ""
}
