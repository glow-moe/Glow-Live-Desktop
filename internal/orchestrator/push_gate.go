package orchestrator

import (
	"encoding/json"
	"hash/fnv"
	"regexp"
	"time"

	"github.com/glow-moe/glow-collector/internal/snapshot"
)

// What goes out to glow.moe, and when. The loop ticks every ~1.5s; most of
// those ticks (a lobby, the client, a Steam game) carry the same frame as the
// last one, so:
//   - a frame identical to the last one sent is skipped, with a resend every
//     heartbeat so the site doesn't count you as gone (it waits 15s);
//   - outside a match, at most one frame every outOfGameEvery;
//   - the raw League reads only go out when they changed (the site keeps the
//     rest), with a full set every rawFullEvery and after a failed push.
//
// In a match every tick still goes out: that's what the live view is for.
const (
	heartbeat      = 10 * time.Second
	outOfGameEvery = 3 * time.Second
	rawFullEvery   = 60 * time.Second
)

type pushGate struct {
	hash        uint64
	at          time.Time
	rawSent     map[string]uint64 // raw key -> hash of the body the site has
	rawFullAt   time.Time
	rawPending  map[string]uint64 // what rawSent becomes if this push lands
	rawFull     bool
	pendingHash uint64
}

var updatedAtRE = regexp.MustCompile(`"updatedAt":\d+`)

// frameHash fingerprints a frame, ignoring its timestamp.
func frameHash(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(updatedAtRE.ReplaceAll(b, nil))
	return h.Sum64()
}

func bodyHash(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}

// rawDelta trims a frame's raw reads to what the site doesn't have yet: new or
// changed keys, null for a key that's gone, and "_full" on a full resend (the
// site then drops anything not in it). Call with o.mu held.
func (g *pushGate) rawDelta(raw map[string]json.RawMessage, now time.Time) map[string]json.RawMessage {
	next := make(map[string]uint64, len(raw))
	for k, v := range raw {
		next[k] = bodyHash(v)
	}
	g.rawPending = next
	g.rawFull = g.rawSent == nil || now.Sub(g.rawFullAt) >= rawFullEvery
	if g.rawFull {
		if len(raw) == 0 && g.rawSent == nil {
			return nil
		}
		out := make(map[string]json.RawMessage, len(raw)+1)
		for k, v := range raw {
			out[k] = v
		}
		out["_full"] = json.RawMessage("true")
		return out
	}
	out := map[string]json.RawMessage{}
	for k, v := range raw {
		if g.rawSent[k] != next[k] {
			out[k] = v
		}
	}
	for k := range g.rawSent {
		if _, ok := next[k]; !ok {
			out[k] = json.RawMessage("null")
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// gate prepares a frame: it trims the raw reads and says whether to send it.
// The returned bytes are what goes out.
func (o *Orchestrator) gate(snap any, inGame bool, now time.Time) ([]byte, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	g := &o.pushGate
	// The frame's own fingerprint leaves the raw reads out: those are compared
	// key by key in rawDelta, and any change there sends the frame.
	var h uint64
	rawChanged := false
	if s, ok := snap.(snapshot.Snapshot); ok && s.Game == "lol" && !s.Live {
		bare := s
		bare.Raw = nil
		if b, err := json.Marshal(bare); err == nil {
			h = frameHash(b)
		}
		s.Raw = g.rawDelta(s.Raw, now)
		rawChanged = len(s.Raw) > 0
		snap = s
	} else {
		// Another kind of frame: back in a lobby later, start with a full set.
		g.rawSent, g.rawPending, g.rawFull = nil, nil, false
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return nil, false
	}
	if h == 0 {
		h = frameHash(b)
	}
	g.pendingHash = h
	if !g.at.IsZero() {
		if !inGame && now.Sub(g.at) < outOfGameEvery {
			return nil, false
		}
		if h == g.hash && !rawChanged && now.Sub(g.at) < heartbeat {
			return nil, false
		}
	}
	return b, true
}

// sent records a frame that reached the site.
func (o *Orchestrator) sent(b []byte, now time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	g := &o.pushGate
	g.hash, g.at = g.pendingHash, now
	if g.rawPending != nil {
		g.rawSent = g.rawPending
		if g.rawFull {
			g.rawFullAt = now
		}
	}
	g.rawPending = nil
}

// failed forgets what the site has, so the next frame resends everything.
func (o *Orchestrator) failed() {
	o.mu.Lock()
	o.pushGate = pushGate{}
	o.mu.Unlock()
}
