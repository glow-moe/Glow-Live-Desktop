package snapshot

import "strings"

// hiddenName replaces a summoner the owner chose to hide. Matches glow.moe's
// server-side mask token so the localhost overlay reads identically.
const hiddenName = "•••"

// Mask applies the owner's L!VE name-privacy settings to a League snapshot in
// place, mirroring glow.moe's maskLeagueSnapshot (src/lib/live/mask-snapshot.ts).
//
// hideOtherNames hides every OTHER player on both sides: teammates give a
// stream sniper the lobby just as surely as enemies do, so "hide enemy names"
// grew into "hide other players" (2026-09-17). hideMyName hides the owner's own
// row and the me card. Feed lines are scrubbed of every hidden name, so nobody
// leaks through a kill message. This runs on the collector so the local overlay
// never serves names the owner hid.
func (s *Snapshot) Mask(hideMyName, hideOtherNames bool) {
	if !hideMyName && !hideOtherNames {
		return
	}
	hidden := map[string]bool{}
	hideRow := func(p *Player) {
		if n := gameName(p.Summoner); n != "" {
			hidden[n] = true
		}
		p.Summoner = hiddenName
	}
	for i := range s.Blue {
		if s.Blue[i].IsMe {
			if hideMyName {
				hideRow(&s.Blue[i])
			}
		} else if hideOtherNames {
			hideRow(&s.Blue[i])
		}
	}
	for i := range s.Red {
		if s.Red[i].IsMe {
			if hideMyName {
				hideRow(&s.Red[i])
			}
		} else if hideOtherNames {
			hideRow(&s.Red[i])
		}
	}
	if hideMyName {
		if n := strings.TrimSpace(s.Me.RiotName); n != "" {
			hidden[n] = true
		}
		s.Me.RiotName = hiddenName
		s.Me.RiotTag = ""
	}
	if len(hidden) == 0 {
		return
	}
	for i := range s.Feed {
		f := &s.Feed[i]
		if hidden[strings.TrimSpace(f.Who)] {
			f.Who = hiddenName
		}
		for n := range hidden {
			f.Text = replaceName(f.Text, n)
		}
	}
}

// gameName is the name as it appears in feed lines: the riot id without #tag.
func gameName(summoner string) string {
	if i := strings.Index(summoner, "#"); i >= 0 {
		summoner = summoner[:i]
	}
	return strings.TrimSpace(summoner)
}

func isBoundary(b byte, edge bool) bool {
	return edge || b == ' ' || b == ',' || b == '.' || b == '!' || b == '(' || b == ')'
}

// replaceName swaps whole-name occurrences only ("slew Ashe" must not touch
// "slew Ashen"). Names can contain spaces, so this scans instead of splitting.
func replaceName(text, name string) string {
	if name == "" {
		return text
	}
	var out strings.Builder
	i := 0
	for {
		j := strings.Index(text[i:], name)
		if j < 0 {
			out.WriteString(text[i:])
			return out.String()
		}
		j += i
		end := j + len(name)
		before := isBoundary(0, j == 0) || isBoundary(text[j-1], false)
		after := end == len(text) || isBoundary(text[end], false)
		out.WriteString(text[i:j])
		if before && after {
			out.WriteString(hiddenName)
		} else {
			out.WriteString(text[j:end])
		}
		i = end
	}
}
