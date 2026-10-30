package orchestrator

import (
	"strings"
	"time"

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
	UpdatedAt int64          `json:"updatedAt"`
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
	p := fortniteSnap{Game: "fortnite", InMatch: f.InMatch, Map: f.Map, Playlist: f.Playlist, Phase: f.Phase, Status: f.Status, Island: f.Island, StartedAt: ms(f.JoinedAt), UpdatedAt: time.Now().UnixMilli()}
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
