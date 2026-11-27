package valorant

import "sort"

// roundResult is one round of match-details, the fields the feed uses.
type roundResult struct {
	RoundNum    int    `json:"roundNum"`
	RoundResult string `json:"roundResult"`
	WinningTeam string `json:"winningTeam"`
	BombPlanter string `json:"bombPlanter"`
	BombDefuser string `json:"bombDefuser"`
	PlayerStats []struct {
		Subject string `json:"subject"`
		Kills   []struct {
			RoundTime       int    `json:"roundTime"`
			Killer          string `json:"killer"`
			Victim          string `json:"victim"`
			FinishingDamage struct {
				DamageType string `json:"damageType"`
				DamageItem string `json:"damageItem"`
			} `json:"finishingDamage"`
		} `json:"kills"`
		Damage []struct {
			Receiver  string `json:"receiver"`
			Headshots int    `json:"headshots"`
		} `json:"damage"`
	} `json:"playerStats"`
}

// maxFeed caps a round's kills (ten players, a few revives at most).
const maxFeed = 16

// rounds turns match-details' rounds into the feed: each round's kills in
// order, your kills and headshots, and a clutch when your team won it with
// you as its last one alive. players and subjects are in the same order.
// Deathmatch (no teams) has none.
func rounds(in []roundResult, players []FinalPlayer, subjects []string) ([]FinalRound, []string) {
	idx := map[string]int{}
	me, myTeam := -1, ""
	for i, s := range subjects {
		idx[s] = i
		if players[i].Self {
			me, myTeam = i, players[i].Team
		}
	}
	if me < 0 || myTeam == "" || len(in) > 40 {
		return nil, nil
	}
	who := func(s string) int {
		if i, ok := idx[s]; ok {
			return i
		}
		return -1
	}
	var weapons []string
	weapon := func(t, item string) int {
		if t != "Weapon" || !reUUID.MatchString(item) {
			return -1
		}
		for i, w := range weapons {
			if w == item {
				return i
			}
		}
		weapons = append(weapons, item)
		return len(weapons) - 1
	}
	var out []FinalRound
	for _, r := range in {
		fr := FinalRound{N: r.RoundNum + 1, Won: r.WinningTeam == myTeam, How: r.RoundResult}
		fr.Planted = r.BombPlanter != "" && who(r.BombPlanter) == me
		fr.Defused = r.BombDefuser != "" && who(r.BombDefuser) == me
		type kill struct {
			t, killer, victim, weapon int
		}
		var ks []kill
		for _, ps := range r.PlayerStats {
			for _, k := range ps.Kills {
				ks = append(ks, kill{k.RoundTime, who(k.Killer), who(k.Victim), weapon(k.FinishingDamage.DamageType, k.FinishingDamage.DamageItem)})
			}
			if who(ps.Subject) == me {
				for _, d := range ps.Damage {
					fr.HS += d.Headshots
				}
			}
		}
		sort.SliceStable(ks, func(a, b int) bool { return ks[a].t < ks[b].t })
		// Who's standing as the round plays out: the clutch is the most
		// enemies left when you became your team's last one.
		alive := map[int]bool{}
		for i, p := range players {
			if p.Team != "" {
				alive[i] = true
			}
		}
		left := func(team string, mine bool) int {
			n := 0
			for i, ok := range alive {
				if ok && (players[i].Team == team) == mine {
					n++
				}
			}
			return n
		}
		clutch := 0
		for _, k := range ks {
			if k.killer == me && k.victim >= 0 && players[k.victim].Team != myTeam {
				fr.Kills++
			}
			if k.victim == me {
				fr.Died = true
			}
			if k.victim >= 0 {
				alive[k.victim] = false
			}
			if clutch == 0 && alive[me] && left(myTeam, true) == 1 {
				if e := left(myTeam, false); e >= 2 {
					clutch = e
				}
			}
			if len(fr.Feed) < maxFeed {
				fr.Feed = append(fr.Feed, [4]int{k.t, k.killer, k.victim, k.weapon})
			}
		}
		if fr.Won && clutch > 0 {
			fr.Clutch = clutch
		}
		out = append(out, fr)
	}
	return out, weapons
}
