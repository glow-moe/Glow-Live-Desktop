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
	"Flash":        "SummonerFlash",
	"Ignite":       "SummonerDot",
	"Teleport":     "SummonerTeleport",
	"Smite":        "SummonerSmite",
	"Heal":         "SummonerHeal",
	"Exhaust":      "SummonerExhaust",
	"Barrier":      "SummonerBarrier",
	"Cleanse":      "SummonerBoost",
	"Ghost":        "SummonerHaste",
	"Clarity":      "SummonerMana",
	"Mark":         "SummonerSnowball",
	"To the King!": "SummonerPoroRecall",
	"Poro Toss":    "SummonerPoroThrow",
}

// summonerSpellByID maps numeric summoner-spell ids (from the client) to ddragon
// keys.
var summonerSpellByID = map[int]string{
	1:  "SummonerBoost",    // Cleanse
	3:  "SummonerExhaust",  // Exhaust
	4:  "SummonerFlash",    // Flash
	6:  "SummonerHaste",    // Ghost
	7:  "SummonerHeal",     // Heal
	11: "SummonerSmite",    // Smite
	12: "SummonerTeleport", // Teleport
	13: "SummonerMana",     // Clarity
	14: "SummonerDot",      // Ignite
	21: "SummonerBarrier",  // Barrier
	32: "SummonerSnowball", // Mark (ARAM)
	39: "SummonerSnowball", // Mark upgrade
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
// too ("Unleashed Teleport" at 14min, "Unleashed Smite", "Chilling Smite", and so on).
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

// spellFiles are the summoner-spell keys ddragon has art for (the values above,
// plus Arena's own two).
var spellFiles = func() map[string]bool {
	m := map[string]bool{"SummonerCherryHold": true, "SummonerCherryFlash": true}
	for _, k := range SummonerSpellKeys {
		m[k] = true
	}
	return m
}()

// SpellKeyFrom picks the ddragon spell key from the live client's spell slot.
// rawDisplayName ("GeneratedTip_SummonerSpell_SummonerFlash_DisplayName") is the
// same in every client language, so it wins; displayName ("Sıçra" on a Turkish
// client) is only the fallback for a raw name of an unexpected shape.
// Upgraded variants ("SummonerSmiteAvatarOffensive", "S12_SummonerTeleportUpgrade")
// map to their base art.
func SpellKeyFrom(raw, display string) string {
	key := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "GeneratedTip_SummonerSpell_"), "_DisplayName")
	if key != "" && key != strings.TrimSpace(raw) {
		if spellFiles[key] {
			return key
		}
		switch k := strings.ToLower(key); {
		case strings.Contains(k, "smite"):
			return "SummonerSmite"
		case strings.Contains(k, "teleport"):
			return "SummonerTeleport"
		case strings.Contains(k, "snowball"), strings.Contains(k, "mark"):
			return "SummonerSnowball"
		case strings.Contains(k, "flash"):
			return "SummonerFlash"
		case strings.Contains(k, "dot"), strings.Contains(k, "ignite"):
			return "SummonerDot"
		case strings.Contains(k, "heal"):
			return "SummonerHeal"
		case strings.Contains(k, "exhaust"):
			return "SummonerExhaust"
		case strings.Contains(k, "barrier"):
			return "SummonerBarrier"
		case strings.Contains(k, "boost"):
			return "SummonerBoost"
		case strings.Contains(k, "haste"):
			return "SummonerHaste"
		case strings.Contains(k, "mana"):
			return "SummonerMana"
		}
	}
	return SpellKey(display)
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
	champMu      sync.Mutex
	champByID    map[int]string    // numeric champion id -> ddragon key
	champNames   map[string]string // ddragon key -> English name ("MonkeyKing" -> "Wukong")
	champsFailAt time.Time         // last load that came back empty
)

// ensureChampionsLocked loads champion.json (champMu held). A load that comes
// back empty (no network yet when the app starts with Windows) is retried a
// couple of minutes later instead of being kept for the whole session, which
// left chroma skins unmapped and their Discord art 404ing.
func ensureChampionsLocked() {
	if len(champByID) > 0 || time.Since(champsFailAt) < 2*time.Minute {
		return
	}
	champByID, champNames = map[int]string{}, map[string]string{}
	loadChampions()
	if len(champByID) == 0 {
		champsFailAt = time.Now()
	}
}

// ChampKeyByID maps a numeric champion id (from the client / champ select) to
// its ddragon key, loading champion.json once. Empty on miss.
func ChampKeyByID(id int) string {
	champMu.Lock()
	defer champMu.Unlock()
	ensureChampionsLocked()
	return champByID[id]
}

// ChampName is a champion's English name by ddragon key ("MonkeyKing" ->
// "Wukong"), so the page reads the same whatever language the game client is
// in. Empty until champion.json has loaded, and for an unknown key.
func ChampName(key string) string {
	champMu.Lock()
	defer champMu.Unlock()
	ensureChampionsLocked()
	return champNames[key]
}

// IsChampKey reports whether key is a champion ddragon knows ("Seraphine",
// "MonkeyKing"). False until champion.json has loaded, and for anything else.
func IsChampKey(key string) bool {
	if key == "" {
		return false
	}
	champMu.Lock()
	defer champMu.Unlock()
	ensureChampionsLocked()
	for _, k := range champByID {
		if k == key {
			return true
		}
	}
	return false
}

// SetChampionsForTest seeds the champion table so tests stay off the network.
func SetChampionsForTest(byID map[int]string) {
	champMu.Lock()
	defer champMu.Unlock()
	champByID = byID
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
			Key  string `json:"key"` // numeric id, as a string
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return
	}
	for key, v := range doc.Data {
		if n, err := strconv.Atoi(v.Key); err == nil {
			champByID[n] = key
		}
		if v.Name != "" {
			champNames[key] = v.Name
		}
	}
}

var (
	skinVidMu    sync.Mutex
	skinVideos   map[int]map[int]string // championId -> skinNum -> animated video URL
	champIDByKey map[string]int
)

// Per-champion skin data from Community Dragon, loaded once per champion:
// chroma → parent skin, the real skin numbers, and animated-splash videos.
var (
	skinParents = map[int]map[int]int{}    // champ id -> chroma num -> parent skin num
	skinNames   = map[int]map[int]string{} // champ id -> skin or chroma num -> English name
	skinNums    = map[int][]int{}          // champ id -> known skin nums
	skinFailAt  = map[int]time.Time{}      // last failed load, to avoid hammering offline
)

// champIDLocked resolves a ddragon key to the numeric champion id (skinVidMu held).
func champIDLocked(champKey string) int {
	// Rebuilt until the champion table has loaded (an empty one is retried).
	if len(champIDByKey) == 0 {
		champMu.Lock()
		ensureChampionsLocked()
		champIDByKey = make(map[string]int, len(champByID))
		for id, k := range champByID {
			champIDByKey[k] = id
		}
		champMu.Unlock()
	}
	return champIDByKey[champKey]
}

// ensureSkinDataLocked loads a champion's skin data once (skinVidMu held). A
// failed load is retried after a couple of minutes, not every tick.
func ensureSkinDataLocked(id int) {
	if skinVideos == nil {
		skinVideos = map[int]map[int]string{}
	}
	if _, ok := skinVideos[id]; ok {
		return
	}
	if time.Since(skinFailAt[id]) < 2*time.Minute {
		return
	}
	videos, parents, nums, names, ok := loadSkinData(id)
	if !ok {
		skinFailAt[id] = time.Now()
		return
	}
	skinVideos[id], skinParents[id], skinNums[id], skinNames[id] = videos, parents, nums, names
}

// ParentSkin maps a chroma's skin number to the skin it belongs to. Ddragon only
// has tiles/splashes for real skins, so the chroma id the live client reports
// would 404 everywhere the art is used (Discord, the overlay, the site). Unknown
// ids fall back to the closest lower known skin; offline, the id passes through.
func ParentSkin(champKey string, skin int) int {
	if skin <= 0 || champKey == "" {
		return skin
	}
	skinVidMu.Lock()
	defer skinVidMu.Unlock()
	id := champIDLocked(champKey)
	if id == 0 {
		return skin
	}
	ensureSkinDataLocked(id)
	if p, ok := skinParents[id][skin]; ok {
		return p
	}
	nums := skinNums[id]
	if len(nums) == 0 {
		return skin
	}
	best := 0
	for _, n := range nums {
		if n == skin {
			return skin
		}
		if n < skin && n > best {
			best = n
		}
	}
	return best
}

// SkinVideoURL returns the animated-splash video (webm) for a skin, or "" when
// the skin isn't animated (only Ultimate/legendary skins have one). Sourced from
// Community Dragon's per-champion data; cached per champion.
func SkinVideoURL(champKey string, skin int) string {
	if champKey == "" {
		return ""
	}
	skinVidMu.Lock()
	defer skinVidMu.Unlock()
	id := champIDLocked(champKey)
	if id == 0 {
		return ""
	}
	ensureSkinDataLocked(id)
	return skinVideos[id][skin]
}

// SkinName is a skin's (or chroma's) English name by skin number, so the page
// reads the same whatever language the game client is in. Empty when unknown
// (offline, or the base skin, which the client calls "default").
func SkinName(champKey string, skin int) string {
	if champKey == "" || skin <= 0 {
		return ""
	}
	skinVidMu.Lock()
	defer skinVidMu.Unlock()
	id := champIDLocked(champKey)
	if id == 0 {
		return ""
	}
	ensureSkinDataLocked(id)
	return skinNames[id][skin]
}

// SkinID returns the numeric skin id (championId*1000 + skin), or 0 on miss.
func SkinID(champKey string, skin int) int {
	if champKey == "" {
		return 0
	}
	skinVidMu.Lock()
	defer skinVidMu.Unlock()
	id := champIDLocked(champKey)
	if id == 0 {
		return 0
	}
	return id*1000 + skin
}

func loadSkinData(champID int) (videos map[int]string, parents map[int]int, nums []int, names map[int]string, ok bool) {
	videos, parents, names = map[int]string{}, map[int]int{}, map[int]string{}
	const base = "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default"
	resp, err := httpClient.Get(fmt.Sprintf("%s/v1/champions/%d.json", base, champID))
	if err != nil {
		return videos, parents, nil, names, false
	}
	defer resp.Body.Close()
	var doc struct {
		Skins []struct {
			ID              int    `json:"id"`
			Name            string `json:"name"`
			SplashVideoPath string `json:"splashVideoPath"`
			Chromas         []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"chromas"`
		} `json:"skins"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&doc) != nil {
		return videos, parents, nil, names, false
	}
	for _, sk := range doc.Skins {
		num := sk.ID % 1000
		nums = append(nums, num)
		names[num] = sk.Name
		for _, ch := range sk.Chromas {
			parents[ch.ID%1000] = num
			if ch.Name != "" {
				names[ch.ID%1000] = ch.Name
			}
		}
		if sk.SplashVideoPath == "" {
			continue
		}
		// "/lol-game-data/assets/ASSETS/..." → the lowercased CDragon asset URL.
		rel := strings.ToLower(strings.TrimPrefix(sk.SplashVideoPath, "/lol-game-data/assets/"))
		videos[num] = base + "/" + rel
	}
	return videos, parents, nums, names, true
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
	runeMu    sync.Mutex
	runeMap   map[int]string // rune/tree id -> icon path
	runeNames map[int]string // rune/tree id -> English name
)

// RuneIcon returns the icon URL for a rune or tree id (keystone, tree, any
// perk), loading runesReforged.json once. Empty on miss.
func RuneIcon(id int) string {
	runeMu.Lock()
	defer runeMu.Unlock()
	ensureRunesLocked()
	if p, ok := runeMap[id]; ok {
		return runeImgBase + p
	}
	return ""
}

// RuneName is a rune's or tree's English name by id ("Dark Harvest",
// "Domination"), so the page reads the same whatever language the game client
// is in. Empty on miss.
func RuneName(id int) string {
	runeMu.Lock()
	defer runeMu.Unlock()
	ensureRunesLocked()
	return runeNames[id]
}

func ensureRunesLocked() {
	if runeMap == nil {
		runeMap, runeNames = map[int]string{}, map[int]string{}
		loadRunes()
	}
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
		Name  string `json:"name"`
		Slots []struct {
			Runes []struct {
				ID   int    `json:"id"`
				Icon string `json:"icon"`
				Name string `json:"name"`
			} `json:"runes"`
		} `json:"slots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&trees); err != nil {
		return
	}
	for _, t := range trees {
		runeMap[t.ID], runeNames[t.ID] = t.Icon, t.Name
		for _, sl := range t.Slots {
			for _, r := range sl.Runes {
				runeMap[r.ID], runeNames[r.ID] = r.Icon, r.Name
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
			Image struct {
				Full string `json:"full"`
			} `json:"image"`
		} `json:"passive"`
		Spells []struct {
			Image struct {
				Full string `json:"full"`
			} `json:"image"`
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
