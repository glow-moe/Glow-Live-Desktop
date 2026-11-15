package orchestrator

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/poster"
	"github.com/glow-moe/glow-collector/internal/snapshot"
	"github.com/glow-moe/glow-collector/internal/tft"
)

// Local only, not committed: sends the new TFT frames to glow.moe with this
// PC's L!VE account, and the status to Discord, for a while.
func TestTFTLiveProbe(t *testing.T) {
	if os.Getenv("PROBE_TFT") == "" {
		t.Skip()
	}
	cfg, err := config.Load()
	if err != nil || cfg.Token == "" {
		t.Fatal("no linked config")
	}
	var dc *discord.Client
	if c, err := discord.Connect("1568967014276665505"); err == nil {
		dc = c
		defer c.Close()
	}
	end := time.Now().Add(30 * time.Minute)
	last := ""
	for time.Now().Before(end) {
		s, ok := tft.Current()
		if !ok {
			time.Sleep(5 * time.Second)
			continue
		}
		snap := snapshot.FromTFT(s, time.Now())
		b, _ := json.Marshal(snap)
		err := poster.Post(cfg.Endpoint, cfg.Token, 0, json.RawMessage(b))
		if dc != nil {
			_ = dc.SetActivity(tftActivity(snap, s.StartedAt, "melocet"))
		}
		line := s.Phase + " " + snap.Clock
		if err != nil {
			line += " ERR " + err.Error()
		}
		if line != last {
			t.Log(time.Now().Format("15:04:05"), line, len(s.Units), "units")
			last = line
		}
		time.Sleep(3 * time.Second)
	}
	if dc != nil {
		_ = dc.Clear()
	}
}
