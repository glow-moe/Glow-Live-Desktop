package snapshot

// hiddenName replaces a summoner the owner chose to hide. Matches glow.moe's
// server-side mask token so the localhost overlay reads identically.
const hiddenName = "•••"

// Mask applies the owner's L!VE name-privacy settings to a League snapshot in
// place, mirroring glow.moe's maskLeagueSnapshot EXACTLY (src/lib/live/mask.ts).
// "Your team" is whichever side (blue/red) holds the isMe row; the other side is
// the enemy. This runs on the collector now so the local overlay never serves
// names the owner hid - the same guarantee the server read used to provide.
func (s *Snapshot) Mask(hideMyName, hideEnemyNames bool) {
	if !hideMyName && !hideEnemyNames {
		return
	}
	meBlue := false
	for i := range s.Blue {
		if s.Blue[i].IsMe {
			meBlue = true
			break
		}
	}
	for i := range s.Blue {
		if s.Blue[i].IsMe {
			if hideMyName {
				s.Blue[i].Summoner = hiddenName
			}
		} else if hideEnemyNames && !meBlue {
			s.Blue[i].Summoner = hiddenName
		}
	}
	for i := range s.Red {
		if s.Red[i].IsMe {
			if hideMyName {
				s.Red[i].Summoner = hiddenName
			}
		} else if hideEnemyNames && meBlue {
			s.Red[i].Summoner = hiddenName
		}
	}
}
