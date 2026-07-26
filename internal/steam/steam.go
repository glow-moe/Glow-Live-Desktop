// Package steam reads what the signed-in Steam user is playing, including the
// game's own Rich Presence line ("Playing singleplayer: California 2").
//
// Why this exists next to the Discord presence we already read: Discord only
// carries a detail line when the game implements DISCORD Rich Presence, and
// plenty of games (Unturned, Contagion) implement the STEAM one instead. Where
// Discord shows a bare "Unturned", Steam knows the map.
//
// Two things are deliberately avoided:
//   - No Steam Web API key. The endpoint below is the same one the community
//     site's hover card uses, and it is keyless.
//   - No per-game work. rich_presence arrives already rendered, so every game
//     that publishes one is supported the moment it ships. Artwork is derived
//     from the appid, so nothing has to be uploaded anywhere.
//
// Limits worth knowing: this only sees games launched THROUGH Steam (a
// Battle.net or Epic copy is invisible here), and nothing at all while the
// account is set to Invisible or the client is in Offline Mode, because Steam
// then publishes no status to anyone.
package steam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// steamID64 base: subtracting it from a 64-bit id yields the 32-bit account id
// the community endpoints are keyed by.
const idBase = 76561197960265728

// Snap is what the signed-in user is playing right now, plus the bits of the
// player's own Steam profile the card shows alongside it.
type Snap struct {
	Game   string // "Unturned"
	Detail string // "Playing singleplayer: California 2"; empty for most games
	AppID  int    // 304930; 0 when it could not be read
	// NonSteam marks a shortcut the user added to their library themselves.
	NonSteam bool

	// Player-side extras. All optional: an account may have no badge, and only
	// some own an animated profile background.
	Level      int    // Steam level, e.g. 79
	Avatar     string // Steam profile picture
	Persona    string // Steam display name, which need not match the glow one
	BadgeName  string // "Unbeatable"
	BadgeDesc  string // "Forza Horizon 5 Foil Badge"
	BadgeIcon  string
	Background string // animated profile background loop (mp4)
}

var client = &http.Client{Timeout: 8 * time.Second}

// One account can span several machines. When the machine's own answer and
// Steam's public one disagree, the FIRST minutes belong to the machine: its
// state is instant while the endpoint is cached for two. But a disagreement
// that OUTLIVES that cache is telling a different story: the game is running
// on some other PC (or a dead launcher left its arguments behind here), and
// then the endpoint is the one that knows. The pair is tracked so the clock
// restarts whenever either side changes its answer.
const remoteTrustAfter = 150 * time.Second

var (
	arbMu    sync.Mutex
	arbPair  uint64
	arbSince time.Time
)

// disagreedFor reports how long this exact (local, remote) split has held.
func disagreedFor(localID, remoteID int) time.Duration {
	key := uint64(uint32(localID))<<32 | uint64(uint32(remoteID))
	arbMu.Lock()
	defer arbMu.Unlock()
	if arbPair != key {
		arbPair, arbSince = key, time.Now()
	}
	return time.Since(arbSince)
}

// agreed clears the tracker; the two sources are telling the same story.
func agreed() {
	arbMu.Lock()
	arbPair, arbSince = 0, time.Time{}
	arbMu.Unlock()
}

// HeaderImage is the game's store banner. Derived from the appid rather than
// the URL Steam hands back, because that one is a small capsule and its path
// shape varies between titles. Discord media-proxies external URLs, so this can
// be handed straight to a Rich Presence payload.
func HeaderImage(appID int) string {
	if appID <= 0 {
		return ""
	}
	return fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/header.jpg", appID)
}

// CoverImage is the game's tall box art, which Discord's square crop treats
// far better than the wide store banner.
//
// Two art generations exist. Classic titles keep hashless files under
// /steam/apps/<id>/, so the guessed URL just works. Newer releases moved to
// hash-prefixed paths that cannot be guessed, so for those the store's GetItems
// endpoint (keyless, asked from this machine) supplies the real URL. The answer
// is cached per appid; a lookup that fails entirely falls back to the guessed
// banner, which for a new title may 404 and cost the artwork, never the
// presence itself.
func CoverImage(appID int) string {
	if appID <= 0 {
		return ""
	}
	coverMu.Lock()
	cached, hit := coverURL[appID]
	coverMu.Unlock()
	if hit {
		return cached
	}

	url := resolveCover(appID)
	if url == "" {
		// Undecided (network trouble): fall back now, ask again next time.
		return HeaderImage(appID)
	}
	coverMu.Lock()
	coverURL[appID] = url
	coverMu.Unlock()
	return url
}

func resolveCover(appID int) string {
	legacy := fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/library_600x900.jpg", appID)
	if resp, err := client.Head(legacy); err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return legacy
		}
	} else {
		return ""
	}

	// New-style title: ask the store where the art actually lives.
	input := fmt.Sprintf(
		`{"ids":[{"appid":%d}],"context":{"country_code":"US","language":"english"},"data_request":{"include_assets":true}}`,
		appID,
	)
	req, err := http.NewRequest(http.MethodGet,
		"https://api.steampowered.com/IStoreBrowseService/GetItems/v1/?input_json="+url.QueryEscape(input), nil)
	if err != nil {
		return HeaderImage(appID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var body struct {
		Response struct {
			StoreItems []struct {
				Assets *struct {
					Format     string `json:"asset_url_format"`
					Header     string `json:"header"`
					LibCapsule string `json:"library_capsule"`
					LibCap2x   string `json:"library_capsule_2x"`
				} `json:"assets"`
			} `json:"store_items"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil ||
		len(body.Response.StoreItems) == 0 || body.Response.StoreItems[0].Assets == nil {
		return HeaderImage(appID)
	}
	a := body.Response.StoreItems[0].Assets
	file := a.LibCap2x
	if file == "" {
		file = a.LibCapsule
	}
	if file == "" {
		file = a.Header
	}
	if file == "" || a.Format == "" {
		return HeaderImage(appID)
	}
	path := strings.ReplaceAll(a.Format, "${FILENAME}", file)
	if !strings.HasPrefix(path, "store_item_assets/") {
		path = "store_item_assets/" + path
	}
	// The canonical host, straight: the cloudflare/akamai aliases 301 here, and
	// Discord's media proxy is happier without the hop.
	return "https://shared.steamstatic.com/" + path
}

var (
	coverMu  sync.Mutex
	coverURL = map[int]string{}
)

// StoreURL is the game's store page.
func StoreURL(appID int) string {
	if appID <= 0 {
		return ""
	}
	return fmt.Sprintf("https://store.steampowered.com/app/%d", appID)
}

// AccountID resolves the 32-bit account id of the Steam user this machine is
// signed in as, by reading the client's own loginusers.vdf. Deliberately local:
// it always reflects the account actually in use, which is not necessarily the
// one linked on the website.
func AccountID() (uint32, bool) {
	for _, p := range loginUsersPaths() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if id, ok := mostRecentID(string(b)); ok {
			return uint32(id - idBase), true
		}
	}
	return 0, false
}

// loginUsersPaths lists where the Steam client keeps loginusers.vdf, best guess
// first. A missing path is simply skipped.
func loginUsersPaths() []string {
	var dirs []string
	switch runtime.GOOS {
	case "windows":
		for _, root := range []string{
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("ProgramFiles"),
			`C:\Program Files (x86)`,
			`C:\Program Files`,
			`D:`, `E:`,
		} {
			if root != "" {
				dirs = append(dirs, filepath.Join(root, "Steam", "config"))
			}
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(h, "Library", "Application Support", "Steam", "config"))
		}
	default:
		if h, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs,
				filepath.Join(h, ".steam", "steam", "config"),
				filepath.Join(h, ".local", "share", "Steam", "config"),
				filepath.Join(h, ".steam", "root", "config"),
			)
		}
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, filepath.Join(d, "loginusers.vdf"))
	}
	return out
}

// mostRecentID picks the account to report from a loginusers.vdf. A machine can
// hold several; prefer the one Steam flagged MostRecent, else the newest
// Timestamp. A line-scan is enough here and avoids pulling in a VDF parser.
func mostRecentID(vdf string) (uint64, bool) {
	var cur, best uint64
	var bestStamp int64 = -1
	for _, raw := range strings.Split(vdf, "\n") {
		line := strings.TrimSpace(raw)
		fields := splitQuoted(line)

		// A block header is a lone quoted 17-digit id.
		if len(fields) == 1 && len(fields[0]) == 17 {
			if id, err := strconv.ParseUint(fields[0], 10, 64); err == nil && id > idBase {
				cur = id
			}
			continue
		}
		if cur == 0 || len(fields) < 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "mostrecent":
			if fields[1] == "1" {
				return cur, true
			}
		case "timestamp":
			if ts, err := strconv.ParseInt(fields[1], 10, 64); err == nil && ts > bestStamp {
				bestStamp, best = ts, cur
			}
		}
	}
	return best, best != 0
}

// splitQuoted pulls the "quoted" tokens out of a VDF line.
func splitQuoted(line string) []string {
	var out []string
	for {
		i := strings.IndexByte(line, '"')
		if i < 0 {
			return out
		}
		line = line[i+1:]
		j := strings.IndexByte(line, '"')
		if j < 0 {
			return out
		}
		out = append(out, line[:j])
		line = line[j+1:]
	}
}

// miniProfile is the slice of the hover-card payload we care about.
type miniProfile struct {
	Level     int    `json:"level"`
	AvatarURL string `json:"avatar_url"`
	Persona   string `json:"persona_name"`
	InGame *struct {
		Name         string `json:"name"`
		IsNonSteam   bool   `json:"is_non_steam"`
		Logo         string `json:"logo"`
		RichPresence string `json:"rich_presence"`
	} `json:"in_game"`
	FavoriteBadge *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"favorite_badge"`
	ProfileBackground *struct {
		MP4  string `json:"video/mp4"`
		WebM string `json:"video/webm"`
	} `json:"profile_background"`
}

// Fetch reports what the account is playing. ok is false when it is playing
// nothing, or when Steam is publishing no status at all (Invisible, Offline
// Mode, or a game started outside Steam).
//
// English is requested explicitly: the response is localised to the caller, and
// this string ends up on a public profile where the reader's language is not
// the player's.
func Fetch(accountID uint32) (Snap, bool) {
	// The machine knows what it launched, and it knows now. Steam's own answer is
	// cached for two minutes and hidden entirely while the account is Invisible,
	// so the local read decides WHICH game and the endpoint only decorates it.
	localID, localName, localOK := RunningApp()

	if accountID == 0 {
		// No linked account to decorate with, so the local read is all there is.
		return Snap{Game: localName, AppID: localID}, localOK && localName != ""
	}
	url := fmt.Sprintf("https://steamcommunity.com/miniprofile/%d/json?appid=undefined", accountID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Snap{}, false
	}
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	// Whatever the endpoint does, a local hit is still a running game.
	fallback := func() (Snap, bool) {
		if localOK && localName != "" {
			return Snap{Game: localName, AppID: localID}, true
		}
		return Snap{}, false
	}

	resp, err := client.Do(req)
	if err != nil {
		return fallback()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallback()
	}
	var mp miniProfile
	if err := json.NewDecoder(resp.Body).Decode(&mp); err != nil {
		return fallback()
	}
	g := mp.InGame
	if g != nil && g.Name == "" {
		g = nil
	}

	// Nothing running here while the endpoint says something is. Fresh, that is
	// the cache describing a game that was just closed; held past the cache
	// window, it is a game running on ANOTHER machine, and that machine cannot
	// speak for itself here.
	if localOK && localID == 0 {
		if g == nil {
			agreed()
			return Snap{}, false
		}
		if disagreedFor(0, appIDFromLogo(g.Logo)) < remoteTrustAfter {
			return Snap{}, false
		}
	}
	if g == nil {
		// The account may be Invisible, or the endpoint may not have caught up
		// with a game that just started. The player extras are still usable.
		s, ok := fallback()
		if ok {
			s.Level, s.Avatar, s.Persona = mp.Level, mp.AvatarURL, mp.Persona
			if b := mp.FavoriteBadge; b != nil {
				s.BadgeName, s.BadgeDesc, s.BadgeIcon = b.Name, b.Description, b.Icon
			}
		}
		return s, ok
	}
	remoteID := appIDFromLogo(g.Logo)
	name, appID, detail := g.Name, remoteID, strings.TrimSpace(g.RichPresence)

	// Both sides name a game but not the same one. Early on the machine wins
	// (its switch is instant, the endpoint's answer is the previous game, and
	// that game's status line goes with it). A split that outlives the cache
	// window flips: the endpoint is watching a game on another PC, while the
	// local id is a leftover launcher that never got cleaned up.
	if localOK && localID != 0 && remoteID != 0 && remoteID != localID {
		if disagreedFor(localID, remoteID) < remoteTrustAfter {
			appID, detail = localID, ""
			if localName != "" {
				name = localName
			}
		}
	} else {
		agreed()
	}

	s := Snap{
		Game:     name,
		Detail:   detail,
		AppID:    appID,
		NonSteam: g.IsNonSteam,
		Level:    mp.Level,
		Avatar:   mp.AvatarURL,
		Persona:  mp.Persona,
	}
	if b := mp.FavoriteBadge; b != nil {
		s.BadgeName, s.BadgeDesc, s.BadgeIcon = b.Name, b.Description, b.Icon
	}
	if bg := mp.ProfileBackground; bg != nil {
		// mp4 plays everywhere the profile card runs; webm is the same loop.
		s.Background = bg.MP4
		if s.Background == "" {
			s.Background = bg.WebM
		}
	}
	return s, true
}

// appIDFromLogo digs the appid out of the artwork URL. The path shape differs
// between titles (some carry an extra hash segment), so this looks for the
// segment right after "/apps/" rather than assuming a fixed depth.
func appIDFromLogo(logo string) int {
	const marker = "/apps/"
	i := strings.Index(logo, marker)
	if i < 0 {
		return 0
	}
	rest := logo[i+len(marker):]
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		rest = rest[:j]
	}
	id, err := strconv.Atoi(rest)
	if err != nil {
		return 0
	}
	return id
}
