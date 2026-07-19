// Package snapshot maps Riot's Live Client Data into the normalized
// LeagueSnapshot the glow.moe page renders. JSON tags mirror the web schema
// (src/lib/league/types.ts) exactly. Cosmetic colors reuse the design palette;
// the page themeSwaps the amber (#f5c211) to the profile accent.
package snapshot

import (
	"fmt"
	"math"
	"strings"

	"github.com/glow-moe/glow-collector/internal/ddragon"
	"github.com/glow-moe/glow-collector/internal/lcu"
	"github.com/glow-moe/glow-collector/internal/live"
)

// Bar is a cur/max pair (HP, mana). Named so the JSON tags are lowercase to
// match the web schema - an anonymous struct's fields would serialize as Cur/Max.
type Bar struct {
	Cur int `json:"cur"`
	Max int `json:"max"`
}

type StatChip struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Color string `json:"color"`
}

type Ability struct {
	Key         string `json:"key"`
	Lvl         int    `json:"lvl"`
	SpellImgKey string `json:"spellImgKey"`
}

type Shard struct {
	Label string `json:"label"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

type Perk struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type Runes struct {
	Keystone      string  `json:"keystone"`
	KeystoneIcon  string  `json:"keystoneIcon"`
	PrimaryTree   string  `json:"primaryTree"`
	SecondaryTree string  `json:"secondaryTree"`
	Shards        []Shard `json:"shards"`
	// Full rune page in slot order (keystone, 3 primary minors, 2 secondary).
	Perks []Perk `json:"perks"`
}

type Player struct {
	Summoner  string   `json:"summoner"`
	Champ     string   `json:"champ"`
	ChampKey  string   `json:"champKey"`
	Skin      int      `json:"skin"`
	Kda       string   `json:"kda"`
	Cs        int      `json:"cs"`
	Vision    int      `json:"vision"`
	Lvl       int      `json:"lvl"`
	SpellKeys []string `json:"spellKeys"`
	Items     []int    `json:"items"`
	IsDead    bool     `json:"isDead"`
	Respawn   int      `json:"respawn"`
	IsMe      bool     `json:"isMe"`
}

type FeedEvent struct {
	Icon  string `json:"icon"`
	Who   string `json:"who"`
	Text  string `json:"text"`
	Color string `json:"color"`
	Dot   string `json:"dot"`
	Time  string `json:"time"`
}

type Me struct {
	ChampName    string  `json:"champName"`
	ChampKey     string  `json:"champKey"`
	Skin         int     `json:"skin"`
	SkinName     string  `json:"skinName"`
	SkinVideoUrl string  `json:"skinVideoUrl,omitempty"`
	RiotName  string     `json:"riotName"`
	RiotTag   string     `json:"riotTag"`
	Level     int        `json:"level"`
	Gold      int        `json:"gold"`
	Hp        Bar        `json:"hp"`
	Mp        Bar        `json:"mp"`
	Stats     []StatChip `json:"stats"`
	Abilities []Ability  `json:"abilities"`
	SpellKeys []string   `json:"spellKeys"`
	Runes     Runes      `json:"runes"`
	Items     []int      `json:"items"`
}

// LobbyMember is a person in a party lobby.
type LobbyMember struct {
	Name    string   `json:"name"`
	Tag     string   `json:"tag"`
	Level   int      `json:"level"`
	IconURL string   `json:"iconUrl"`
	Leader  bool     `json:"leader"`
	You     bool     `json:"you"`
	Roles   []string `json:"roles"`
}

// DraftPlayer is one cell in champ select (either team).
type DraftPlayer struct {
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	ChampKey  string   `json:"champKey"`
	ChampName string   `json:"champName"`
	HoverKey  string   `json:"hoverKey"`
	SpellKeys []string `json:"spellKeys"`
	Status    string   `json:"status"`
	You       bool     `json:"you"`
}

// Lobby is the out-of-game state (client / lobby / queue / champ select).
type Lobby struct {
	Phase     string `json:"phase"`
	Label     string `json:"label"`
	Summoner  string `json:"summoner"`
	Level     int    `json:"level"`
	IconURL   string `json:"iconUrl"`
	ChampKey  string `json:"champKey"`
	ChampName string `json:"champName"`

	QueueName string        `json:"queueName,omitempty"`
	MapName   string        `json:"mapName,omitempty"`
	ModeLabel string        `json:"modeLabel,omitempty"`
	IsRanked  bool          `json:"isRanked,omitempty"`
	LobbySize int           `json:"lobbySize,omitempty"`
	PartyID   string        `json:"partyId,omitempty"`
	Members   []LobbyMember `json:"members,omitempty"`

	PhaseLabel string        `json:"phaseLabel,omitempty"`
	TimeLeft   int           `json:"timeLeft,omitempty"`
	BlueBans   []string      `json:"blueBans,omitempty"`
	RedBans    []string      `json:"redBans,omitempty"`
	BlueTeam   []DraftPlayer `json:"blueTeam,omitempty"`
	RedTeam    []DraftPlayer `json:"redTeam,omitempty"`

	Client *ClientProfile `json:"client,omitempty"`
}

// ClientRank is one ranked queue on the client profile (raw; web derives colors).
type ClientRank struct {
	Queue     string `json:"queue"`
	Tier      string `json:"tier"`
	Division  string `json:"division"`
	LP        int    `json:"lp"`
	Wins      int    `json:"wins"`
	Losses    int    `json:"losses"`
	HotStreak bool   `json:"hotStreak"`
}

// ClientChallenge is one pinned challenge crystal.
type ClientChallenge struct {
	Name string `json:"name"`
	Tier string `json:"tier"`
}

// ClientChallengeProgress is a featured challenge's progress to its next level.
type ClientChallengeProgress struct {
	Name      string  `json:"name,omitempty"`
	Level     string  `json:"level"`
	Current   float64 `json:"current"`
	Threshold float64 `json:"threshold"`
}

// ClientMastery is a top-mastery champion card.
type ClientMastery struct {
	ChampKey  string `json:"champKey"`
	ChampName string `json:"champName"`
	Level     int    `json:"level"`
	Points    int    `json:"points"`
	Tokens    int    `json:"tokens"`
}

// ClientMatch is a recent-match row.
type ClientMatch struct {
	ChampKey    string `json:"champKey"`
	ChampName   string `json:"champName"`
	Queue       string `json:"queue"`
	DurationSec int    `json:"durationSec"`
	Kills       int    `json:"kills"`
	Deaths      int    `json:"deaths"`
	Assists     int    `json:"assists"`
	Win         bool   `json:"win"`
}

// ClientProfile is the resolved "In the client" card (ids → urls/keys).
type ClientProfile struct {
	GameName        string            `json:"gameName"`
	TagLine         string            `json:"tagLine"`
	Level           int               `json:"level"`
	IconURL         string            `json:"iconUrl"`
	XpSince         int               `json:"xpSince,omitempty"`
	XpTo            int               `json:"xpTo,omitempty"`
	BannerChampKey   string           `json:"bannerChampKey,omitempty"`
	BackgroundSplash string           `json:"backgroundSplash,omitempty"`
	Availability    string            `json:"availability,omitempty"`
	StatusMessage   string            `json:"statusMessage,omitempty"`
	Title           string            `json:"title,omitempty"`
	HonorLevel      int               `json:"honorLevel,omitempty"`
	Ranks           []ClientRank      `json:"ranks,omitempty"`
	ChallengeScore  int               `json:"challengeScore,omitempty"`
	OverallLevel    string            `json:"overallLevel,omitempty"`
	ChallengeTokens []ClientChallenge         `json:"challengeTokens,omitempty"`
	Challenges      []ClientChallengeProgress `json:"challenges,omitempty"`
	MasteryScore    int                       `json:"masteryScore,omitempty"`
	Mastery         []ClientMastery   `json:"mastery,omitempty"`
	Matches         []ClientMatch     `json:"matches,omitempty"`
}

type Snapshot struct {
	Game      string      `json:"game"`
	Patch     string      `json:"patch"`
	Live      bool        `json:"live"`
	Phase     string      `json:"phase"`
	UpdatedAt int64       `json:"updatedAt"`
	Clock     string      `json:"clock"`
	MapName   string      `json:"mapName"`
	Me        Me          `json:"me"`
	Blue      []Player    `json:"blue"`
	Red       []Player    `json:"red"`
	Feed      []FeedEvent `json:"feed"`
	// Lobby is set instead of the match fields when out of game.
	Lobby *Lobby `json:"lobby,omitempty"`
}

// FromLobby builds an out-of-game snapshot from the client (LCU) state.
func FromLobby(l *lcu.Lobby, patch string, now int64) Snapshot {
	lob := &Lobby{
		Phase:    l.Phase,
		Label:    l.Label,
		Summoner: l.Summoner,
		Level:    l.Level,
	}
	if l.IconID > 0 {
		lob.IconURL = ddragon.ProfileIconURL(l.IconID)
	}
	if l.ChampID > 0 {
		lob.ChampKey = ddragon.ChampKeyByID(l.ChampID)
		lob.ChampName = lob.ChampKey
	}

	// party lobby
	lob.QueueName, lob.MapName, lob.ModeLabel = l.QueueName, l.MapName, l.ModeLabel
	lob.IsRanked, lob.LobbySize, lob.PartyID = l.IsRanked, l.MaxSize, l.PartyID
	for _, m := range l.Members {
		icon := ""
		if m.IconID > 0 {
			icon = ddragon.ProfileIconURL(m.IconID)
		}
		lob.Members = append(lob.Members, LobbyMember{
			Name: m.Name, Tag: m.Tag, Level: m.Level, IconURL: icon,
			Leader: m.Leader, You: m.You, Roles: m.Roles,
		})
	}

	// champ select
	lob.PhaseLabel, lob.TimeLeft = l.PhaseLabel, l.TimeLeft
	lob.BlueBans = banKeys(l.BlueBans)
	lob.RedBans = banKeys(l.RedBans)
	lob.BlueTeam = draftPlayers(l.Blue)
	lob.RedTeam = draftPlayers(l.Red)

	// in the client (phase None)
	if l.Client != nil {
		lob.Client = mapClient(l.Client)
	}

	return Snapshot{
		Game:      "lol",
		Patch:     patch,
		Live:      false,
		Phase:     l.Phase,
		UpdatedAt: now,
		Lobby:     lob,
	}
}

// mapClient resolves the LCU client profile into the pushed shape (champ ids →
// keys/names, icon id → url).
func mapClient(c *lcu.ClientProfile) *ClientProfile {
	out := &ClientProfile{
		GameName: c.GameName, TagLine: c.TagLine, Level: c.Level,
		XpSince: c.XpSince, XpTo: c.XpTo,
		Availability: c.Availability, StatusMessage: c.StatusMessage,
		Title: c.Title, HonorLevel: c.HonorLevel,
		ChallengeScore: c.ChallengeScore, OverallLevel: c.OverallLevel,
		MasteryScore: c.MasteryScore,
	}
	if c.IconID > 0 {
		out.IconURL = ddragon.ProfileIconURL(c.IconID)
	}
	if c.BannerChampID > 0 {
		out.BannerChampKey = ddragon.ChampKeyByID(c.BannerChampID)
	}
	// Selected profile background: skin id → champion + skin index → splash art.
	if c.BannerSkinID > 0 {
		if key := ddragon.ChampKeyByID(c.BannerSkinID / 1000); key != "" {
			out.BackgroundSplash = ddragon.SplashURL(key, c.BannerSkinID%1000)
		}
	}
	for _, r := range c.Ranks {
		out.Ranks = append(out.Ranks, ClientRank{
			Queue: r.Queue, Tier: r.Tier, Division: r.Division,
			LP: r.LP, Wins: r.Wins, Losses: r.Losses, HotStreak: r.HotStreak,
		})
	}
	for _, t := range c.ChallengeTokens {
		out.ChallengeTokens = append(out.ChallengeTokens, ClientChallenge{Name: t.Name, Tier: t.Tier})
	}
	for _, ch := range c.Challenges {
		out.Challenges = append(out.Challenges, ClientChallengeProgress{
			Name: ch.Name, Level: ch.Level, Current: ch.Current, Threshold: ch.Threshold,
		})
	}
	for _, m := range c.Mastery {
		key := ddragon.ChampKeyByID(m.ChampID)
		out.Mastery = append(out.Mastery, ClientMastery{
			ChampKey: key, ChampName: fixChampName(key),
			Level: m.Level, Points: m.Points, Tokens: m.Tokens,
		})
	}
	for _, mt := range c.Matches {
		key := ddragon.ChampKeyByID(mt.ChampID)
		out.Matches = append(out.Matches, ClientMatch{
			ChampKey: key, ChampName: fixChampName(key), Queue: mt.Queue,
			DurationSec: mt.DurationSec, Kills: mt.Kills, Deaths: mt.Deaths,
			Assists: mt.Assists, Win: mt.Win,
		})
	}
	return out
}

func banKeys(ids []int) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			out = append(out, ddragon.ChampKeyByID(id))
		} else {
			out = append(out, "")
		}
	}
	return out
}

func draftPlayers(cells []lcu.Draftee) []DraftPlayer {
	out := make([]DraftPlayer, 0, len(cells))
	for _, c := range cells {
		dp := DraftPlayer{
			Name:   c.Name,
			Role:   c.Role,
			Status: c.Status,
			You:    c.You,
			SpellKeys: []string{
				ddragon.SpellKeyByID(c.Spell1),
				ddragon.SpellKeyByID(c.Spell2),
			},
		}
		if c.ChampID > 0 {
			dp.ChampKey = ddragon.ChampKeyByID(c.ChampID)
			dp.ChampName = dp.ChampKey
		}
		if c.HoverID > 0 {
			dp.HoverKey = ddragon.ChampKeyByID(c.HoverID)
		}
		out = append(out, dp)
	}
	return out
}

const (
	amber  = "#f5c211"
	blue   = "#9cc4f5"
	red    = "#ff9aa1"
	redDot = "#ff6b73"
	mute   = "#a99f8c"
	muteD  = "#7d7391"
)

// Build converts one live payload into a snapshot. now is epoch millis.
func Build(d *live.AllGameData, patch string, now int64) Snapshot {
	selfID := d.ActivePlayer.RiotID
	if selfID == "" {
		selfID = d.ActivePlayer.SummonerName
	}
	isSelf := func(p live.Player) bool {
		return (p.RiotID != "" && p.RiotID == selfID) ||
			p.SummonerName == d.ActivePlayer.SummonerName
	}

	var blue, red []Player
	var selfChampKey, selfTeam string
	// Events refer to players by their in-game name, which is riotIdGameName for
	// humans but "<Champ> Bot" for bots - index both so team lookup always hits.
	teamOf := map[string]string{}
	for _, p := range d.AllPlayers {
		me := isSelf(p)
		row := mapPlayer(p, me)
		if p.RiotIDName != "" {
			teamOf[p.RiotIDName] = p.Team
		}
		if p.SummonerName != "" {
			teamOf[p.SummonerName] = p.Team
		}
		if me {
			selfChampKey = row.ChampKey
			selfTeam = p.Team
		}
		if p.Team == "ORDER" {
			blue = append(blue, row)
		} else {
			red = append(red, row)
		}
	}

	// Feed events name the active player by game name ("Guts"), not the tagged
	// summonerName ("Guts#aykms") - match on that so own kills read as "mine".
	selfFeedName := d.ActivePlayer.RiotIDName
	if selfFeedName == "" {
		selfFeedName = d.ActivePlayer.SummonerName
		if i := strings.Index(selfFeedName, "#"); i >= 0 {
			selfFeedName = selfFeedName[:i]
		}
	}

	return Snapshot{
		Game:      "lol",
		Patch:     patch,
		Live:      true,
		Phase:     "inGame",
		UpdatedAt: now,
		Clock:     fmtClock(d.GameData.GameTime),
		MapName:   mapLabel(d.GameData),
		Me:        mapMe(d, selfChampKey),
		Blue:      blue,
		Red:       red,
		Feed:      mapFeed(d, selfFeedName, selfTeam, teamOf),
	}
}

func mapPlayer(p live.Player, isMe bool) Player {
	items := make([]int, 0, len(p.Items))
	for _, it := range p.Items {
		items = append(items, it.ItemID)
	}
	name := p.RiotIDName
	if name == "" {
		name = p.SummonerName
	}
	return Player{
		Summoner:  name,
		Champ:     fixChampName(p.ChampionName),
		ChampKey:  champKey(p.RawChampionName, p.ChampionName),
		Skin:      p.SkinID,
		Kda:       fmt.Sprintf("%d/%d/%d", p.Scores.Kills, p.Scores.Deaths, p.Scores.Assists),
		Cs:        p.Scores.CreepScore,
		Vision:    int(math.Round(p.Scores.WardScore)),
		Lvl:       p.Level,
		SpellKeys: []string{ddragon.SpellKey(p.SummonerSpells.SummonerSpellOne.DisplayName), ddragon.SpellKey(p.SummonerSpells.SummonerSpellTwo.DisplayName)},
		Items:     items,
		IsDead:    p.IsDead,
		Respawn:   int(math.Round(p.RespawnTimer)),
		IsMe:      isMe,
	}
}

func mapMe(d *live.AllGameData, champKeyVal string) Me {
	ap := d.ActivePlayer
	cs := ap.ChampionStats
	name, tag := splitRiotID(ap.RiotID, ap.RiotIDName, ap.SummonerName)

	var me Me
	me.ChampName = champName(champKeyVal, d)
	me.ChampKey = champKeyVal
	me.Skin = selfSkin(d)
	me.SkinName = selfSkinName(d)
	me.SkinVideoUrl = ddragon.SkinVideoURL(champKeyVal, me.Skin)
	me.RiotName = name
	me.RiotTag = tag
	me.Level = ap.Level
	me.Gold = int(math.Round(ap.CurrentGold))
	me.Hp.Cur, me.Hp.Max = int(cs.CurrentHealth), int(cs.MaxHealth)
	me.Mp.Cur, me.Mp.Max = int(cs.ResourceValue), int(cs.ResourceMax)
	me.Stats = statChips(cs)
	me.Abilities = abilityRow(champKeyVal, ap.Abilities)
	me.SpellKeys = selfSpells(d)
	me.Runes = runes(ap.FullRunes)
	me.Items = selfItems(d)
	return me
}

func statChips(cs live.ChampionStats) []StatChip {
	return []StatChip{
		{"AD", fmt.Sprintf("%d", int(cs.AttackDamage)), red},
		{"AP", fmt.Sprintf("%d", int(cs.AbilityPower)), "#c79bff"},
		{"Armor", fmt.Sprintf("%d", int(cs.Armor)), amber},
		{"MR", fmt.Sprintf("%d", int(cs.MagicResist)), blue},
		{"MS", fmt.Sprintf("%d", int(cs.MoveSpeed)), "#8ff0c0"},
		{"AS", fmt.Sprintf("%.2f", cs.AttackSpeed), "#F3EFE6"},
		{"Crit", fmt.Sprintf("%d%%", int(cs.CritChance*100)), amber},
		{"Haste", fmt.Sprintf("%d", int(cs.AbilityHaste)), blue},
	}
}

func abilityRow(champKeyVal string, a live.Abilities) []Ability {
	ab := ddragon.ChampionAbilities(champKeyVal)
	return []Ability{
		{"Q", a.Q.AbilityLevel, ab.Q},
		{"W", a.W.AbilityLevel, ab.W},
		{"E", a.E.AbilityLevel, ab.E},
		{"R", a.R.AbilityLevel, ab.R},
	}
}

func selfSpells(d *live.AllGameData) []string {
	for _, p := range d.AllPlayers {
		if p.SummonerName == d.ActivePlayer.SummonerName ||
			(p.RiotID != "" && p.RiotID == d.ActivePlayer.RiotID) {
			return []string{
				ddragon.SpellKey(p.SummonerSpells.SummonerSpellOne.DisplayName),
				ddragon.SpellKey(p.SummonerSpells.SummonerSpellTwo.DisplayName),
			}
		}
	}
	return []string{"SummonerFlash", "SummonerFlash"}
}

func selfSkin(d *live.AllGameData) int {
	for _, p := range d.AllPlayers {
		if p.SummonerName == d.ActivePlayer.SummonerName ||
			(p.RiotID != "" && p.RiotID == d.ActivePlayer.RiotID) {
			return p.SkinID
		}
	}
	return 0
}

func selfSkinName(d *live.AllGameData) string {
	for _, p := range d.AllPlayers {
		if p.SummonerName == d.ActivePlayer.SummonerName ||
			(p.RiotID != "" && p.RiotID == d.ActivePlayer.RiotID) {
			return p.SkinName
		}
	}
	return ""
}

func selfItems(d *live.AllGameData) []int {
	for _, p := range d.AllPlayers {
		if p.SummonerName == d.ActivePlayer.SummonerName ||
			(p.RiotID != "" && p.RiotID == d.ActivePlayer.RiotID) {
			out := make([]int, 0, len(p.Items))
			for _, it := range p.Items {
				out = append(out, it.ItemID)
			}
			return out
		}
	}
	return nil
}

func runes(fr live.FullRunes) Runes {
	shardColors := []string{amber, red, "#8ff0c0"}
	var shards []Shard
	for i, s := range fr.StatRunes {
		if i >= 3 {
			break
		}
		shards = append(shards, Shard{
			Label: shardLabel(s),
			Color: shardColors[i%len(shardColors)],
			Icon:  ddragon.ShardIcon(s.ID),
		})
	}
	var perks []Perk
	for _, r := range fr.GeneralRunes {
		if r.ID == 0 {
			continue
		}
		perks = append(perks, Perk{Name: r.DisplayName, Icon: ddragon.RuneIcon(r.ID)})
	}
	return Runes{
		Keystone:      fr.Keystone.DisplayName,
		KeystoneIcon:  ddragon.RuneIcon(fr.Keystone.ID),
		PrimaryTree:   fr.PrimaryRuneTree.DisplayName,
		SecondaryTree: fr.SecondaryRuneTree.DisplayName,
		Shards:        shards,
		Perks:         perks,
	}
}

// shardLabel turns the live client's stat-shard tooltip key
// ("perk_tooltip_StatModAdaptive") into a friendly label.
func shardLabel(r live.Rune) string {
	raw := r.RawDescription
	if raw == "" {
		raw = r.DisplayName
	}
	key := raw
	if i := strings.LastIndex(key, "StatMod"); i >= 0 {
		key = key[i+len("StatMod"):]
	}
	switch key {
	case "Adaptive":
		return "Adaptive Force"
	case "Tenacity":
		return "Tenacity"
	case "Health", "HealthScaling", "HealthPlus":
		return "Health"
	case "Armor":
		return "Armor"
	case "MagicResist":
		return "Magic Resist"
	case "CDRScaling":
		return "Ability Haste"
	case "AttackSpeed":
		return "Attack Speed"
	case "MovementSpeed":
		return "Move Speed"
	}
	if key == "" {
		return "Rune"
	}
	return key
}

func mapFeed(d *live.AllGameData, selfName, selfTeam string, teamOf map[string]string) []FeedEvent {
	evs := d.Events.Events
	var out []FeedEvent
	// newest first, cap at 8
	for i := len(evs) - 1; i >= 0 && len(out) < 8; i-- {
		if fe, ok := feedLine(evs[i], selfName, selfTeam, teamOf); ok {
			out = append(out, fe)
		}
	}
	return out
}

func cleanName(s string) string { return strings.TrimSuffix(strings.TrimSpace(s), " Bot") }

// feedColor tints a feed line by the actor's side: amber = you, blue = ally,
// red = enemy, muted = neutral/unknown.
func feedColor(actor, selfName, selfTeam string, teamOf map[string]string) (string, string) {
	if actor != "" && actor == selfName {
		return amber, amber
	}
	if t, ok := teamOf[actor]; ok {
		if t == selfTeam {
			return blue, blue
		}
		return red, redDot
	}
	return mute, muteD
}

func feedLine(e live.Event, selfName, selfTeam string, teamOf map[string]string) (FeedEvent, bool) {
	t := fmtClock(e.EventTime)
	color := func(actor string) (string, string) {
		return feedColor(actor, selfName, selfTeam, teamOf)
	}
	switch e.EventName {
	case "ChampionKill":
		c, dot := color(e.KillerName)
		return FeedEvent{"⚔️", cleanName(e.KillerName), "slew " + cleanName(e.VictimName), c, dot, t}, true
	case "FirstBlood":
		c, dot := color(e.Recipient)
		return FeedEvent{"🩸", cleanName(e.Recipient), "drew First Blood", c, dot, t}, true
	case "Multikill":
		c, dot := color(e.KillerName)
		return FeedEvent{"💥", cleanName(e.KillerName), multikill(e.KillStreak), c, dot, t}, true
	case "DragonKill":
		c, dot := color(e.KillerName)
		return FeedEvent{"🐉", cleanName(e.KillerName), "slayed " + dragon(e.DragonType), c, dot, t}, true
	case "BaronKill":
		c, dot := color(e.KillerName)
		return FeedEvent{"🟣", cleanName(e.KillerName), "slayed Baron Nashor", c, dot, t}, true
	case "HeraldKill":
		c, dot := color(e.KillerName)
		return FeedEvent{"🐌", cleanName(e.KillerName), "slayed Rift Herald", c, dot, t}, true
	case "HordeKill":
		c, dot := color(e.KillerName)
		return FeedEvent{"🪱", cleanName(e.KillerName), "took a Voidgrub", c, dot, t}, true
	case "TurretKilled":
		c, dot := color(e.KillerName)
		return FeedEvent{"🗼", cleanName(e.KillerName), "destroyed a turret", c, dot, t}, true
	case "InhibKilled":
		c, dot := color(e.KillerName)
		return FeedEvent{"🏛️", cleanName(e.KillerName), "destroyed an inhibitor", c, dot, t}, true
	case "Ace":
		return FeedEvent{"🅰️", "Team", "scored an Ace", amber, amber, t}, true
	case "GameStart":
		return FeedEvent{"🎮", "Game", "Match started", mute, muteD, t}, true
	}
	return FeedEvent{}, false
}

func multikill(streak int) string {
	switch streak {
	case 2:
		return "Double Kill"
	case 3:
		return "Triple Kill"
	case 4:
		return "Quadra Kill"
	case 5:
		return "Penta Kill"
	}
	return "Multikill"
}

func dragon(t string) string {
	if t == "" {
		return "a Drake"
	}
	return t + " Drake"
}

// champKey derives the ddragon champion key from rawChampionName
// ("game_character_displayname_LeeSin" -> "LeeSin"), falling back to the display
// name with spaces/punctuation stripped.
func champKey(raw, display string) string {
	if i := strings.LastIndex(raw, "_"); i >= 0 && i+1 < len(raw) {
		if k := raw[i+1:]; k != "" {
			return k
		}
	}
	return strings.NewReplacer(" ", "", "'", "", ".", "").Replace(display)
}

func champName(champKeyVal string, d *live.AllGameData) string {
	for _, p := range d.AllPlayers {
		if champKey(p.RawChampionName, p.ChampionName) == champKeyVal {
			return fixChampName(p.ChampionName)
		}
	}
	return fixChampName(champKeyVal)
}

// fixChampName gives the proper display name where the game's internal name
// differs (ddragon images use "FiddleSticks" but the display is "Fiddlesticks").
func fixChampName(s string) string {
	return strings.ReplaceAll(s, "FiddleSticks", "Fiddlesticks")
}

func splitRiotID(riotID, gameName, summoner string) (name, tag string) {
	if riotID != "" {
		if i := strings.Index(riotID, "#"); i >= 0 {
			return riotID[:i], riotID[i+1:]
		}
		return riotID, ""
	}
	if gameName != "" {
		return gameName, ""
	}
	return summoner, ""
}

func mapLabel(g live.GameData) string {
	switch g.MapNumber {
	case 11:
		return "Summoner's Rift"
	case 12:
		return "Howling Abyss"
	case 21, 22:
		return "Nexus Blitz"
	case 30:
		return "Arena"
	}
	if g.GameMode != "" {
		return g.GameMode
	}
	return "Summoner's Rift"
}

func fmtClock(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	total := int(sec)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
