// Package ddragon resolves the few things the Live Client Data API doesn't hand
// over directly: the current patch string, summoner-spell image keys, and a
// champion's Q/W/E/R ability icon filenames. Results are cached in-memory.
package ddragon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const fallbackVersion = "14.13.1"

// SummonerSpellKeys maps live-client display names to ddragon /spell/ file keys.
var SummonerSpellKeys = map[string]string{
	"Flash":    "SummonerFlash",
	"Ignite":   "SummonerDot",
	"Teleport": "SummonerTeleport",
	"Smite":    "SummonerSmite",
	"Heal":     "SummonerHeal",
	"Exhaust":  "SummonerExhaust",
	"Barrier":  "SummonerBarrier",
	"Cleanse":  "SummonerBoost",
	"Ghost":    "SummonerHaste",
	"Clarity":  "SummonerMana",
	"Mark":     "SummonerSnowball",
	"To the King!": "SummonerPoroRecall",
	"Poro Toss":    "SummonerPoroThrow",
}

// summonerSpellByID maps numeric summoner-spell ids (from the client) to ddragon
// keys.
var summonerSpellByID = map[int]string{
	1:  "SummonerBoost",     // Cleanse
	3:  "SummonerExhaust",   // Exhaust
	4:  "SummonerFlash",     // Flash
	6:  "SummonerHaste",     // Ghost
	7:  "SummonerHeal",      // Heal
	11: "SummonerSmite",     // Smite
	12: "SummonerTeleport",  // Teleport
	13: "SummonerMana",      // Clarity
	14: "SummonerDot",       // Ignite
	21: "SummonerBarrier",   // Barrier
	32: "SummonerSnowball",  // Mark (ARAM)
	39: "SummonerSnowball",  // Mark upgrade
}

// SpellKeyByID returns the ddragon key for a numeric summoner-spell id (falls
// back to Flash so the tile always has art).
func SpellKeyByID(id int) string {
	if k, ok := summonerSpellByID[id]; ok {
		return k
	}
	return "SummonerFlash"
}

// SpellKey returns the ddragon spell key for a summoner-spell display name.
// Exact match first, then keyword match so upgraded/prefixed variants resolve
// too ("Unleashed Teleport" at 14min, "Unleashed Smite", "Chilling Smite", …).
// Falls back to "SummonerFlash" so the tile always has art.
func SpellKey(displayName string) string {
	d := strings.TrimSpace(displayName)
	if k, ok := SummonerSpellKeys[d]; ok {
		return k
	}
	switch dl := strings.ToLower(d); {
	case strings.Contains(dl, "teleport"):
		return "SummonerTeleport"
	case strings.Contains(dl, "smite"):
		return "SummonerSmite"
	case strings.Contains(dl, "ignite"):
		return "SummonerDot"
	case strings.Contains(dl, "heal"):
		return "SummonerHeal"
	case strings.Contains(dl, "exhaust"):
		return "SummonerExhaust"
	case strings.Contains(dl, "barrier"):
		return "SummonerBarrier"
	case strings.Contains(dl, "cleanse"):
		return "SummonerBoost"
	case strings.Contains(dl, "ghost"):
		return "SummonerHaste"
	case strings.Contains(dl, "clarity"):
		return "SummonerMana"
	case strings.Contains(dl, "mark"), strings.Contains(dl, "dash"):
		return "SummonerSnowball"
	case strings.Contains(dl, "flash"):
		return "SummonerFlash"
	}
	return "SummonerFlash"
}

// imgKeyFix maps ddragon champion.json keys that differ from their image
// filenames ("Fiddlesticks" data key -> "FiddleSticks" image files).
var imgKeyFix = map[string]string{"Fiddlesticks": "FiddleSticks"}

func imgKey(k string) string {
	if v, ok := imgKeyFix[k]; ok {
		return v
	}
	return k
}

// SplashURL is the (version-less) full skin splash for a champion + skin id.
func SplashURL(champKey string, skin int) string {
	return fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/img/champion/splash/%s_%d.jpg", imgKey(champKey), skin)
}

// TileURL is the skin's near-square loading tile (portrait, champion-centered) -
// crops far better than the wide splash for Discord's large image slot.
func TileURL(champKey string, skin int) string {
	return fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/img/champion/tiles/%s_%d.jpg", imgKey(champKey), skin)
}

// ProfileIconURL is a summoner profile icon by id. Served from Community Dragon,
// which has every icon - ddragon 404s on newer/event icons, and a missing icon
// shows as a broken avatar (and 404s a Discord large_image).
func ProfileIconURL(id int) string {
	return fmt.Sprintf("https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/profile-icons/%d.jpg", id)
}

var (
	champMu   sync.Mutex
	champByID map[int]string // numeric champion id -> ddragon key
)

// ChampKeyByID maps a numeric champion id (from the client / champ select) to
// its ddragon key, loading champion.json once. Empty on miss.
func ChampKeyByID(id int) string {
	champMu.Lock()
	defer champMu.Unlock()
	if champByID == nil {
		champByID = map[int]string{}
		loadChampions()
	}
	return champByID[id]
}

func loadChampions() {
	url := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/data/en_US/champion.json", Version())
	resp, err := httpClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var doc struct {
		Data map[string]struct {
			Key string `json:"key"` // numeric id, as a string
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return
	}
	for key, v := range doc.Data {
		if n, err := strconv.Atoi(v.Key); err == nil {
			champByID[n] = key
		}
	}
}

var (
	skinVidMu    sync.Mutex
	skinVideos   map[int]map[int]string // championId -> skinNum -> animated video URL
	champIDByKey map[string]int
)

// SkinVideoURL returns the animated-splash video (webm) for a skin, or "" when
// the skin isn't animated (only Ultimate/legendary skins have one). Sourced from
// Community Dragon's per-champion data; cached per champion.
func SkinVideoURL(champKey string, skin int) string {
	if champKey == "" {
		return ""
	}
	skinVidMu.Lock()
	defer skinVidMu.Unlock()
	if champIDByKey == nil {
		champMu.Lock()
		if champByID == nil {
			champByID = map[int]string{}
			loadChampions()
		}
		champMu.Unlock()
		champIDByKey = map[string]int{}
		for id, k := range champByID {
			champIDByKey[k] = id
		}
	}
	id := champIDByKey[champKey]
	if id == 0 {
		return ""
	}
	if skinVideos == nil {
		skinVideos = map[int]map[int]string{}
	}
	m, ok := skinVideos[id]
	if !ok {
		m = loadSkinVideos(id)
		skinVideos[id] = m
	}
	return m[skin]
}

// SkinID returns the numeric skin id (championId*1000 + skin), or 0 on miss.
func SkinID(champKey string, skin int) int {
	if champKey == "" {
		return 0
	}
	skinVidMu.Lock()
	defer skinVidMu.Unlock()
	if champIDByKey == nil {
		champMu.Lock()
		if champByID == nil {
			champByID = map[int]string{}
			loadChampions()
		}
		champMu.Unlock()
		champIDByKey = map[string]int{}
		for id, k := range champByID {
			champIDByKey[k] = id
		}
	}
	id := champIDByKey[champKey]
	if id == 0 {
		return 0
	}
	return id*1000 + skin
}

func loadSkinVideos(champID int) map[int]string {
	out := map[int]string{}
	const base = "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default"
	resp, err := httpClient.Get(fmt.Sprintf("%s/v1/champions/%d.json", base, champID))
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	var doc struct {
		Skins []struct {
			ID              int    `json:"id"`
			SplashVideoPath string `json:"splashVideoPath"`
		} `json:"skins"`
	}
	if json.NewDecoder(resp.Body).Decode(&doc) != nil {
		return out
	}
	for _, sk := range doc.Skins {
		if sk.SplashVideoPath == "" {
			continue
		}
		// "/lol-game-data/assets/ASSETS/..." → the lowercased CDragon asset URL.
		rel := strings.ToLower(strings.TrimPrefix(sk.SplashVideoPath, "/lol-game-data/assets/"))
		out[sk.ID%1000] = base + "/" + rel
	}
	return out
}

var httpClient = &http.Client{Timeout: 6 * time.Second}

var (
	verMu   sync.Mutex
	verVal  string
	verWhen time.Time
)

// Version returns the latest ddragon patch, cached for an hour. Falls back to a
// pinned version when offline so art still resolves.
func Version() string {
	verMu.Lock()
	defer verMu.Unlock()
	if verVal != "" && time.Since(verWhen) < time.Hour {
		return verVal
	}
	resp, err := httpClient.Get("https://ddragon.leagueoflegends.com/api/versions.json")
	if err != nil {
		if verVal != "" {
			return verVal
		}
		return fallbackVersion
	}
	defer resp.Body.Close()
	var vs []string
	if err := json.NewDecoder(resp.Body).Decode(&vs); err != nil || len(vs) == 0 {
		if verVal != "" {
			return verVal
		}
		return fallbackVersion
	}
	verVal, verWhen = vs[0], time.Now()
	return verVal
}

const runeImgBase = "https://ddragon.leagueoflegends.com/cdn/img/"

// Stat-shard id -> ddragon StatMods icon filename (version-less path).
var shardIcons = map[int]string{
	5008: "StatModsAdaptiveForceIcon.png",
	5005: "StatModsAttackSpeedIcon.png",
	5007: "StatModsCDRScalingIcon.png",
	5010: "StatModsMovementSpeedIcon.png",
	5001: "StatModsHealthScalingIcon.png",
	5011: "StatModsHealthPlusIcon.png",
	5013: "StatModsTenacityIcon.png",
	5002: "StatModsArmorIcon.png",
	5003: "StatModsMagicResIcon.png",
}

// ShardIcon returns the icon URL for a stat-shard rune id (empty if unknown).
func ShardIcon(id int) string {
	if f, ok := shardIcons[id]; ok {
		return runeImgBase + "perk-images/StatMods/" + f
	}
	return ""
}

var (
	runeMu  sync.Mutex
	runeMap map[int]string // rune/tree id -> icon path
)

// RuneIcon returns the icon URL for a rune or tree id (keystone, tree, any
// perk), loading runesReforged.json once. Empty on miss.
func RuneIcon(id int) string {
	runeMu.Lock()
	defer runeMu.Unlock()
	if runeMap == nil {
		runeMap = map[int]string{}
		loadRunes()
	}
	if p, ok := runeMap[id]; ok {
		return runeImgBase + p
	}
	return ""
}

func loadRunes() {
	url := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/data/en_US/runesReforged.json", Version())
	resp, err := httpClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var trees []struct {
		ID    int    `json:"id"`
		Icon  string `json:"icon"`
		Slots []struct {
			Runes []struct {
				ID   int    `json:"id"`
				Icon string `json:"icon"`
			} `json:"runes"`
		} `json:"slots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&trees); err != nil {
		return
	}
	for _, t := range trees {
		runeMap[t.ID] = t.Icon
		for _, sl := range t.Slots {
			for _, r := range sl.Runes {
				runeMap[r.ID] = r.Icon
			}
		}
	}
}

// Abilities holds a champion's ability icon keys (ddragon /spell/ filenames,
// minus the .png), keyed Q/W/E/R plus the Passive.
type Abilities struct {
	Q, W, E, R, Passive string
}

var (
	abMu    sync.Mutex
	abCache = map[string]Abilities{} // champKey -> abilities
)

type champData struct {
	Data map[string]struct {
		Passive struct {
			Image struct{ Full string `json:"full"` } `json:"image"`
		} `json:"passive"`
		Spells []struct {
			Image struct{ Full string `json:"full"` } `json:"image"`
		} `json:"spells"`
	} `json:"data"`
}

// ChampionAbilities fetches (and caches) a champion's ability icon keys. On any
// error it returns zero values; callers should treat empty keys as "no icon".
func ChampionAbilities(champKey string) Abilities {
	abMu.Lock()
	if a, ok := abCache[champKey]; ok {
		abMu.Unlock()
		return a
	}
	abMu.Unlock()

	url := fmt.Sprintf("https://ddragon.leagueoflegends.com/cdn/%s/data/en_US/champion/%s.json", Version(), champKey)
	resp, err := httpClient.Get(url)
	if err != nil {
		return Abilities{}
	}
	defer resp.Body.Close()
	var cd champData
	if err := json.NewDecoder(resp.Body).Decode(&cd); err != nil {
		return Abilities{}
	}
	entry, ok := cd.Data[champKey]
	if !ok || len(entry.Spells) < 4 {
		return Abilities{}
	}
	strip := func(s string) string { return strings.TrimSuffix(s, ".png") }
	a := Abilities{
		Q:       strip(entry.Spells[0].Image.Full),
		W:       strip(entry.Spells[1].Image.Full),
		E:       strip(entry.Spells[2].Image.Full),
		R:       strip(entry.Spells[3].Image.Full),
		Passive: strip(entry.Passive.Image.Full),
	}
	abMu.Lock()
	abCache[champKey] = a
	abMu.Unlock()
	return a
}
