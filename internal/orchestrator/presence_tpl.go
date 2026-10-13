package orchestrator

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/glow-moe/glow-collector/internal/discord"
)

// Discord wording from glow.moe. Every status is still built here first (that
// is the fallback when the site sends nothing), then the L!VE settings'
// "presence" map can re-word it per kind ("lol-lobby", "steam", "music", ...).
// A template field lists alternatives: the first whose {vars} are all filled
// wins, so "Locked in {champ}" can fall back to "{queue}". A field the template
// leaves out keeps the built-in value; "" as an alternative means "leave empty".
// {_details}, {_state}, {_largeImage}, {_largeText} ... are the built-in values,
// {user} the glow username and {glow} the glow badge image.

type presenceTpl struct {
	Details    []string    `json:"details"`
	State      []string    `json:"state"`
	LargeImage []string    `json:"largeImage"`
	LargeText  []string    `json:"largeText"`
	SmallImage []string    `json:"smallImage"`
	SmallText  []string    `json:"smallText"`
	Buttons    []tplButton `json:"buttons"`
	// NoTime drops the elapsed timer / progress bar.
	NoTime bool `json:"noTime"`
}

type tplButton struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

var tplVarRE = regexp.MustCompile(`\{([A-Za-z0-9_]{1,32})\}`)

// fill renders one line; false when a var it uses is empty or unknown.
func fill(t string, vars map[string]string) (string, bool) {
	ok := true
	out := tplVarRE.ReplaceAllStringFunc(t, func(m string) string {
		v := strings.TrimSpace(vars[m[1:len(m)-1]])
		if v == "" {
			ok = false
		}
		return v
	})
	return strings.TrimSpace(out), ok
}

// pickText is the first alternative that renders to 2+ characters (Discord
// drops the whole activity for a one-character field), capped at 128.
func pickText(alts []string, vars map[string]string) (string, bool) {
	for _, a := range alts {
		if a == "" {
			return "", true
		}
		if s, ok := fill(a, vars); ok && utf8.RuneCountInString(s) >= 2 {
			if utf8.RuneCountInString(s) > 128 {
				s = string([]rune(s)[:127]) + "…"
			}
			return s, true
		}
	}
	return "", false
}

// pickImage is the first alternative that renders to an https URL Discord takes.
func pickImage(alts []string, vars map[string]string) (string, bool) {
	for _, a := range alts {
		if a == "" {
			return "", true
		}
		if s, ok := fill(a, vars); ok && strings.HasPrefix(s, "https://") && len(s) <= 300 && !strings.ContainsAny(s, " \"<>") {
			return s, true
		}
	}
	return "", false
}

// applyTpl re-words a built activity with one template.
func applyTpl(a discord.Activity, t presenceTpl, vars map[string]string) discord.Activity {
	v := make(map[string]string, len(vars)+8)
	for k, x := range vars {
		v[k] = x
	}
	v["_details"], v["_state"] = a.Details, a.State
	if a.Assets != nil {
		v["_largeImage"], v["_largeText"] = a.Assets.LargeImage, a.Assets.LargeText
		v["_smallImage"], v["_smallText"] = a.Assets.SmallImage, a.Assets.SmallText
	}
	v["glow"] = glowIcon

	if s, ok := pickText(t.Details, v); ok && t.Details != nil {
		a.Details = s
	}
	if s, ok := pickText(t.State, v); ok && t.State != nil {
		a.State = s
	}
	if t.LargeImage != nil || t.LargeText != nil || t.SmallImage != nil || t.SmallText != nil {
		as := discord.Assets{}
		if a.Assets != nil {
			as = *a.Assets
		}
		if s, ok := pickImage(t.LargeImage, v); ok && t.LargeImage != nil {
			as.LargeImage = s
		}
		if s, ok := pickText(t.LargeText, v); ok && t.LargeText != nil {
			as.LargeText = s
		}
		if s, ok := pickImage(t.SmallImage, v); ok && t.SmallImage != nil {
			as.SmallImage = s
		}
		if s, ok := pickText(t.SmallText, v); ok && t.SmallText != nil {
			as.SmallText = s
		}
		a.Assets = &as
	}
	if t.Buttons != nil {
		var btns []discord.Button
		for _, b := range t.Buttons {
			if len(btns) == 2 {
				break
			}
			label, ok1 := fill(b.Label, v)
			url, ok2 := fill(b.URL, v)
			if ok1 && ok2 && label != "" && utf8.RuneCountInString(label) <= 32 &&
				strings.HasPrefix(url, "https://") && len(url) <= 512 && !strings.ContainsAny(url, " \"<>") {
				btns = append(btns, discord.Button{Label: label, URL: url})
			}
		}
		a.Buttons = btns
	}
	if t.NoTime {
		a.Timestamps = nil
	}
	return a
}

// styled applies the site's template for this kind, if it sent one.
func (o *Orchestrator) styled(kind string, a discord.Activity, vars map[string]string) discord.Activity {
	o.mu.Lock()
	t, ok := o.settings.Presence[kind]
	user := o.username
	o.mu.Unlock()
	if !ok {
		return a
	}
	if vars == nil {
		vars = map[string]string{}
	}
	vars["user"] = user
	return applyTpl(a, t, vars)
}

// yes is "yes" for true and "" for false, so a template can test a flag.
func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

var discordIDRE = regexp.MustCompile(`^[0-9]{17,20}$`)

// appFor is the Discord application for a status kind: the site's pick when it
// sent a valid one, else the built-in.
func (o *Orchestrator) appFor(kind, builtin string) string {
	o.mu.Lock()
	id := o.settings.DiscordApps[kind]
	o.mu.Unlock()
	if discordIDRE.MatchString(id) {
		return id
	}
	return builtin
}
