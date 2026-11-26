package fortnite

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Save the World, from the same log: the map and mission kind a mission
// loads, the wave a Storm Shield is on, the three mission medals as they
// fill (combat, building, utility) and, when it ends, everyone's score with
// your XP and commander level. Hero, power level and rewards aren't in the log.

// StwRun is the mission you're in.
type StwRun struct {
	Map     string // the zone's terrain ("Suburban"), "" at Homebase
	Mission string // "Storm Shield Defense" when the map says so, else ""
	Wave    int    // the wave a Storm Shield Defense is on, 0 before or without waves
	Medals  [3]StwMedal
}

// StwMedal is one mission medal: the score and how many of its four tiers
// (bronze, silver, gold, platinum) it has, with the score the next tier needs.
type StwMedal struct {
	Score int
	Tier  int
	Next  int
}

// StwMedalKinds names Medals in order.
var StwMedalKinds = [3]string{"combat", "building", "utility"}

// StwResult is a finished mission: everyone's score (yours marked) and your
// XP and commander level when the results screen logged them.
type StwResult struct {
	Map, Mission string
	Players      []StwScore
	EndedAt      time.Time
	// The results screen's XP, by player name (read before your own name may
	// be known: the app can start mid-log). Mine picks yours.
	xp map[string]StwXP
}

// StwXP is what a mission gave one player: XP and their commander level after.
type StwXP struct{ XP, Level, LevelUp int }

// StwScore is one player's end-of-mission report.
type StwScore struct {
	Name                             string
	ID                               string // the Epic account id the report named (may be "5cea7...145f2")
	Me                               bool
	Combat, Building, Utility, Total int
}

var (
	stwWaveRe   = regexp.MustCompile(`Snapshot: Start of Wave (\d{1,2})\b`)
	stwBoundsRe = regexp.MustCompile(`^\s+> BarBounds = \(([\d,]+), ([\d,]+)\)`)
	stwCountRe  = regexp.MustCompile(`^\s+> AchievedCount = ([\d,]+)`)
	stwBadgeRe  = regexp.MustCompile(`^\s+> BageItemDef = (Combat|Building|Utility)Score_MissionBadge_(Bronze|Silver|Gold|Platinum)`)
	stwReportRe = regexp.MustCompile(`Start Report for Player \[(.{1,40}?)\] \[MCP:([0-9a-f.]+)\]`)
	stwScoreRe  = regexp.MustCompile(`ZoneScore:(Combat|Building|Utility|Total)Score \[(\d+)\]`)
	stwXPRe     = regexp.MustCompile(`LogInGameRewardsSTW: (Player Name|Initial Level|Delta Level|Delta (?:Base|Bonus|Boost|Rest|Group Boost) XP): (.+?)\s*$`)
)

var stwTiers = map[string]int{"Bronze": 0, "Silver": 1, "Gold": 2, "Platinum": 3}

func atoiComma(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	return n
}

// stwMap reads a mission's map path: the terrain and, for an Outpost, that
// it's a Storm Shield Defense. Homebase and anything outside STW_Zones give "".
func stwMap(path string) (terrain, mission string) {
	base := path
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	if !strings.Contains(path, "/STW_Zones/") || !strings.HasPrefix(base, "Zone_") {
		return "", ""
	}
	if strings.HasPrefix(base, "Zone_Outpost") {
		return "", "Storm Shield Defense"
	}
	parts := strings.Split(strings.TrimPrefix(base, "Zone_"), "_")
	last := parts[len(parts)-1]
	if len(last) < 3 || len(last) > 24 {
		return "", ""
	}
	return last, ""
}

// stwStart is a new map loading: a fresh run (nothing carries over).
func (s *State) stwStart(path string) {
	s.Stw = StwRun{}
	s.Stw.Map, s.Stw.Mission = stwMap(path)
	s.stwBar = stwBar{}
	s.stwReports, s.stwAuthoritative = nil, false
}

// stwBar is a medal progress block being read (bounds, count, then badge).
type stwBar struct {
	lo, hi, count int
	haveCount     bool
}

// stwFeed takes the Save the World lines; true when the line was one.
func (s *State) stwFeed(line string) bool {
	if strings.HasPrefix(line, " ") {
		if m := stwBoundsRe.FindStringSubmatch(line); m != nil {
			s.stwBar = stwBar{lo: atoiComma(m[1]), hi: atoiComma(m[2])}
			return true
		}
		if m := stwCountRe.FindStringSubmatch(line); m != nil {
			s.stwBar.count, s.stwBar.haveCount = atoiComma(m[1]), true
			return true
		}
		if m := stwBadgeRe.FindStringSubmatch(line); m != nil && s.stwBar.haveCount {
			k := map[string]int{"Combat": 0, "Building": 1, "Utility": 2}[m[1]]
			b, med := s.stwBar, &s.Stw.Medals[k]
			med.Score = b.count
			if t := stwTiers[m[2]] + 1; b.count >= b.hi && t > med.Tier {
				med.Tier = t
			}
			if b.count >= b.lo && b.count < b.hi {
				med.Next = b.hi
			} else if med.Tier == 4 {
				med.Next = 0
			}
			s.stwBar = stwBar{}
			return true
		}
		return false
	}
	if !s.InMatch {
		return false
	}
	if m := stwWaveRe.FindStringSubmatch(line); m != nil {
		s.Stw.Wave, _ = strconv.Atoi(m[1])
		return true
	}
	if !strings.Contains(line, "ZoneEndScoreReports") && !strings.Contains(line, "ZoneScore:") && !strings.Contains(line, "LogInGameRewardsSTW:") {
		return false
	}
	switch {
	case strings.Contains(line, "Start Server Authoritative Reports"):
		s.stwAuthoritative, s.stwReports = true, nil
	case strings.Contains(line, "End Server Authoritative Reports"):
		s.stwAuthoritative = false
		if len(s.stwReports) > 0 {
			s.StwLast = &StwResult{Map: s.Stw.Map, Mission: s.Stw.Mission, Players: s.stwReports, EndedAt: stamp(line)}
		}
		s.stwReports = nil
	}
	if !s.stwAuthoritative {
		if m := stwXPRe.FindStringSubmatch(line); m != nil && s.StwLast != nil {
			s.stwXP(m[1], m[2])
		}
		return true
	}
	if m := stwReportRe.FindStringSubmatch(line); m != nil && len(s.stwReports) < 4 {
		s.stwReports = append(s.stwReports, StwScore{Name: m[1], ID: m[2]})
		return true
	}
	if m := stwScoreRe.FindStringSubmatch(line); m != nil && len(s.stwReports) > 0 {
		r := &s.stwReports[len(s.stwReports)-1]
		n, _ := strconv.Atoi(m[2])
		switch m[1] {
		case "Combat":
			r.Combat = n
		case "Building":
			r.Building = n
		case "Utility":
			r.Utility = n
		case "Total":
			r.Total = n
		}
	}
	return true
}

// stwXP reads the results screen's XP block, by player. "Delta XP" is the
// bar's new value (it restarts at a level up), so the XP earned is the sum of
// the parts.
func (s *State) stwXP(key, val string) {
	l := s.StwLast
	if key == "Player Name" {
		s.stwXPName = strings.TrimSpace(val)
		return
	}
	if s.stwXPName == "" {
		return
	}
	if l.xp == nil {
		l.xp = map[string]StwXP{}
	}
	x := l.xp[s.stwXPName]
	n := atoiComma(val)
	switch key {
	case "Initial Level":
		s.stwLevelBefore = n
	case "Delta Level":
		x.Level = n
		if s.stwLevelBefore > 0 && n >= s.stwLevelBefore {
			x.LevelUp = n - s.stwLevelBefore
		}
	default: // what the mission gave: base, bonus, boost, rest and group boost XP
		x.XP += n
	}
	l.xp[s.stwXPName] = x
}

// Mine is the result as you saw it: your report marked by your account id or
// name, and your XP.
func (r StwResult) Mine(id, name string) (StwResult, StwXP) {
	out := r
	out.Players = append([]StwScore(nil), r.Players...)
	var mine StwXP
	for i, p := range out.Players {
		if (id != "" && full(p.ID, id)) || (name != "" && p.Name == name) {
			out.Players[i].Me = true
			mine = r.xp[p.Name]
		}
	}
	return out, mine
}
