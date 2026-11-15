package orchestrator

import (
	"fmt"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/cs2"
	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/snapshot"
)

// A big play (an ace, a penta kill) takes over the Discord status line for
// flashFor, then the normal one comes back. glow.moe keeps the moment itself
// (its server/live-moments); this is only the status. Tick goroutine only.
const flashFor = 30 * time.Second

type momentFlash struct {
	text  string
	until time.Time
}

// flashMoment shows text for flashFor the first time key is seen (a key names
// one moment, so the frames after it don't start it again).
func (o *Orchestrator) flashMoment(key, text string, now time.Time) {
	if o.flashSeen == nil {
		o.flashSeen = map[string]time.Time{}
	}
	if _, seen := o.flashSeen[key]; seen {
		return
	}
	for k, at := range o.flashSeen {
		if now.Sub(at) > 12*time.Hour {
			delete(o.flashSeen, k)
		}
	}
	o.flashSeen[key] = now
	o.flash = momentFlash{text: text, until: now.Add(flashFor)}
}

// withFlash puts a running flash on the status line.
func (o *Orchestrator) withFlash(a discord.Activity, now time.Time) discord.Activity {
	if o.flash.text != "" && now.Before(o.flash.until) {
		a.State = o.flash.text
	}
	return a
}

// cs2Ace flashes an ace: five kills in one round of Competitive or Premier.
func (o *Orchestrator) cs2Ace(s cs2.Summary, now time.Time) {
	if !s.Self || s.RoundKills < 5 || (s.Mode != "competitive" && s.Mode != "premier") {
		return
	}
	// Kills before this round stay the same through the round-end frames.
	o.flashMoment(fmt.Sprintf("cs2:%s:%d", s.Map, s.Kills-s.RoundKills), "ACE on "+cs2MapName(s.Map), now)
}

// leaguePenta flashes the member's own penta kill from the match feed.
func (o *Orchestrator) leaguePenta(s snapshot.Snapshot, now time.Time) {
	me := strings.TrimSpace(s.Me.RiotName)
	if me == "" {
		return
	}
	for _, e := range s.Feed {
		if e.Text == "Penta Kill" && strings.EqualFold(strings.TrimSpace(e.Who), me) {
			o.flashMoment("lol:"+s.Me.ChampKey+":"+e.Time, "PENTA KILL as "+s.Me.ChampName, now)
		}
	}
}
