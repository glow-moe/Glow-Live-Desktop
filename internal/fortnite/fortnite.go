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
)

// feed moves the state along the given log lines.
func (s *State) feed(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
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
		}
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
	mu    sync.Mutex
	tail  logtail.Follower
	state State
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
			state = State{}
		}
		state.feed(lines)
	})
	if err != nil {
		return State{}, false
	}
	return state, true
}
