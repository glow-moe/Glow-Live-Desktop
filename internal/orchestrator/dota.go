package orchestrator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/dota"
	"github.com/glow-moe/glow-collector/internal/steam"
)

// dotaPush is the frame glow.moe gets: the raw GSI blocks, mapped on the site
// (src/lib/dota/gsi.ts there).
type dotaPush struct {
	Game      string          `json:"game"`
	Live      bool            `json:"live"`
	UpdatedAt int64           `json:"updatedAt"`
	Raw       json.RawMessage `json:"raw"`
}

// ensureDota installs the GSI cfg and starts the receiver, retrying every 5
// minutes until Dota is installed (and the port is free).
func (o *Orchestrator) ensureDota() *dota.Receiver {
	o.mu.Lock()
	r, tried := o.dotaRecv, o.dotaTriedAt
	o.mu.Unlock()
	if r != nil || time.Since(tried) < 5*time.Minute {
		return r
	}
	o.mu.Lock()
	o.dotaTriedAt = time.Now()
	o.mu.Unlock()
	token, err := dota.Install()
	if err != nil || token == "" {
		return nil
	}
	if r, err = dota.Listen(token); err != nil {
		return nil
	}
	o.mu.Lock()
	o.dotaRecv = r
	o.mu.Unlock()
	return r
}

func dotaHeroName(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func dotaClock(sec int) string {
	sign := ""
	if sec < 0 {
		sign, sec = "-", -sec
	}
	return fmt.Sprintf("%s%d:%02d", sign, sec/60, sec%60)
}

func dotaDetail(s dota.Summary) string {
	if s.Draft || s.Hero == "" {
		return "Dota 2 · picking"
	}
	return fmt.Sprintf("%s · %d/%d/%d · %s", dotaHeroName(s.Hero), s.Kills, s.Deaths, s.Assists, dotaClock(s.Clock))
}

// dotaActivity is the built-in Discord status; the site can re-word it ("dota").
func dotaActivity(s dota.Summary, username string, named bool) discord.Activity {
	details, state := "Dota 2", "Picking a hero"
	if !s.Draft && s.Hero != "" {
		details = dotaHeroName(s.Hero)
		state = fmt.Sprintf("%d / %d / %d  ·  Lv %d", s.Kills, s.Deaths, s.Assists, s.Level)
	}
	if named && details == "Dota 2" {
		details = "In a match"
	}
	large := steam.CoverImage(dota.AppID)
	if large == "" {
		large = glowIcon
	}
	a := discord.Activity{
		Details: details,
		State:   state,
		Assets:  &discord.Assets{LargeImage: large, LargeText: "Dota 2", SmallImage: glowIcon, SmallText: "glow.moe"},
	}
	if s.Clock > 0 && !s.Draft {
		a.Timestamps = &discord.Timestamps{Start: (time.Now().Unix() - int64(s.Clock)) * 1000}
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "Live match", URL: "https://glow.moe/" + username + "/dota"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

func dotaVars(s dota.Summary) map[string]string {
	return map[string]string{
		"hero": s.Hero, "heroName": dotaHeroName(s.Hero), "level": itoa(s.Level),
		"kills": strconv.Itoa(s.Kills), "deaths": strconv.Itoa(s.Deaths), "assists": strconv.Itoa(s.Assists),
		"clock": dotaClock(s.Clock), "draft": yes(s.Draft),
	}
}
