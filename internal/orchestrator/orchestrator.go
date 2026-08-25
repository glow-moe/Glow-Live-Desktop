// Package orchestrator unifies the League + Forza collectors behind one poll
// loop and a single Status the GUI can render. Forza wins while its Data Out
// telemetry is flowing (you're driving, not in a LoL game); otherwise it polls
// the League live API. Mirrors the console app's auto-detect, minus the CLI.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/ddragon"
	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/forza"
	"github.com/glow-moe/glow-collector/internal/lcu"
	"github.com/glow-moe/glow-collector/internal/live"
	"github.com/glow-moe/glow-collector/internal/pair"
	"github.com/glow-moe/glow-collector/internal/poster"
	"github.com/glow-moe/glow-collector/internal/snapshot"
	"github.com/glow-moe/glow-collector/internal/steam"
)

// liveSettings mirrors the L!VE preferences the user sets on the site (the
// dashboard's L!VE tab). The collector reads them from /api/live/settings so the
// GUI never needs its own copy - the site is the single source of truth.
type liveSettings struct {
	HideEnemyNames bool `json:"hideEnemyNames"`
	HideMyName     bool `json:"hideMyName"`
	DelaySec       int  `json:"delaySec"`
	// The OBS overlay's look (accent + which blocks show + render scale). The
	// self-contained /overlay page reads these from /live.json so changing them in
	// the dashboard themes the overlay on the next refresh - no URL to re-copy.
	OverlayAccent string      `json:"overlayAccent"`
	OverlayScale  float64     `json:"overlayScale"`
	OverlayShow   overlayShow `json:"overlayShow"`
}

// overlayShow mirrors OverlayShow on the site: which blocks the in-game card
// shows and whether it's the full card or the compact rank + KDA strip.
type overlayShow struct {
	Stats  bool   `json:"stats"`
	Spells bool   `json:"spells"`
	Runes  bool   `json:"runes"`
	Mode   string `json:"mode"`
}

func fetchSettings(endpoint, token string) (liveSettings, bool) {
	var s liveSettings
	req, err := http.NewRequest(http.MethodGet, pair.BaseFrom(endpoint)+"/api/live/settings", nil)
	if err != nil {
		return s, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return s, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s, false
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return s, false
	}
	return s, true
}

// animeSnap is the "now watching" the browser extension pushes to glow.moe. The
// extension owns detection; we only read it back and mirror it to Discord.
type animeSnap struct {
	Title   string `json:"title"`
	Episode int    `json:"episode"`
	Poster  string `json:"poster"`
}

// fetchAnime reads the profile's current anime "now watching" from glow.moe
// (public read, keyed by the user id). ok is false when nothing is playing.
func fetchAnime(endpoint, userID string) (animeSnap, bool) {
	var out struct {
		Live     bool       `json:"live"`
		Snapshot *animeSnap `json:"snapshot"`
	}
	// userID is a cuid ([a-z0-9]), safe to place in the query without escaping.
	u := pair.BaseFrom(endpoint) + "/api/live/read?u=" + userID + "&game=anime"
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Get(u)
	if err != nil {
		return animeSnap{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return animeSnap{}, false
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || !out.Live || out.Snapshot == nil {
		return animeSnap{}, false
	}
	return *out.Snapshot, out.Snapshot.Title != ""
}

// animeDetail is the one-line status the GUI shows for anime.
func animeDetail(a animeSnap) string {
	if a.Episode > 0 {
		return fmt.Sprintf("%s · Ep %d", a.Title, a.Episode)
	}
	return a.Title
}

// animeActivity builds the Discord Rich Presence for anime. It runs under the
// shared glow app, so Discord's headline reads "glow.moe" (the local IPC can't
// set the Watching type, and the site lacks the activities.write scope); the
// title + episode land in the details/state lines.
func animeActivity(a animeSnap, username string) discord.Activity {
	state := ""
	if a.Episode > 0 {
		state = fmt.Sprintf("Episode %d", a.Episode)
	}
	large := glowIcon
	if a.Poster != "" {
		large = a.Poster
	}
	act := discord.Activity{
		Type:    3, // Watching (so Discord reads "Watching glow.moe", not "Playing")
		Details: a.Title,
		State:   state,
		Assets: &discord.Assets{
			LargeImage: large,
			LargeText:  a.Title,
			SmallImage: glowIcon,
			SmallText:  "glow.moe",
		},
	}
	if username != "" {
		act.Buttons = []discord.Button{
			{Label: "Anime profile", URL: "https://glow.moe/" + username + "/anime"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return act
}

// Per-game Discord apps. Each is named after the game, so the "Playing X" line
// (which Discord ties to the app name, not the activity) shows the real game.
// The ids are injected at build time (see build-*.sh + the .appids file) so they
// stay out of source control; unset means that game just skips Rich Presence.
var (
	appGlow    = ""
	appLoL     = ""
	appForzaH6 = ""
	appForzaH5 = ""
)

func orGlow(id string) string {
	if id == "" {
		return appGlow
	}
	return id
}

func forzaAppID(gameID string) string {
	if gameID == "fh5" {
		return orGlow(appForzaH5)
	}
	return orGlow(appForzaH6)
}

// glowIcon is the glow badge (used as the small corner image on game presences).
const glowIcon = "https://glow.moe/icon-512.png"

// forzaImage is the large Rich Presence image while driving Forza.
const forzaImage = "https://glow.moe/games/forza.png"

// Status is what the GUI renders each poll.
type Status struct {
	Game     string `json:"game"`     // "" | "league" | "forza"
	InGame   bool   `json:"inGame"`   // a game is being read
	Detail   string `json:"detail"`   // e.g. "Ahri · 12:34" or "Forza · 240 mph"
	Pushing  bool   `json:"pushing"`  // last tick pushed to glow.moe
	Pushes   int    `json:"pushes"`   // total pushes this session
	Err      string `json:"err"`      // last error ("" when fine)
	Delay    int    `json:"delay"`    // applied stream delay (from the site), seconds
	AppID    int    `json:"appId"`    // Steam appid of the current game (0 if none)
	GameName string `json:"gameName"` // Steam game name, for the settings list
	Hidden   bool   `json:"hidden"`   // the user turned this game off in the app
}

type forzaState struct {
	mu   sync.Mutex
	snap *forza.Snapshot
	at   time.Time
}

func (f *forzaState) set(s *forza.Snapshot) {
	f.mu.Lock()
	f.snap, f.at = s, time.Now()
	f.mu.Unlock()
}

func (f *forzaState) get() (*forza.Snapshot, time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap, f.at
}

// Orchestrator runs the detect/push loop. Safe for the GUI to Start/Stop.
type Orchestrator struct {
	mu          sync.Mutex
	cfg         config.Config
	forzaGame   string
	forzaPort   int
	cancel      context.CancelFunc
	running     bool
	status      Status
	pushes      int
	overlayJSON []byte // latest masked League snapshot ({live,snapshot}) for the localhost overlay
	forza       *forzaState
	onStatus    func(Status)
	onSeenGame  func(appID int, name string) // records a Steam game for the settings list

	// Discord Rich Presence (best-effort; nil when Discord isn't running).
	dc           *discord.Client
	dcApp        string // app id the current client is connected with
	username     string
	userID       string // profile cuid, for reading own anime "now watching"
	steamID      uint32 // Steam account id signed in on this machine (0 = none)
	steamIDAt    time.Time
	steamSnap    steam.Snap // last good Steam answer
	steamSnapAt  time.Time  // when that answer arrived
	steamPollAt  time.Time  // when the endpoint was last asked
	steamForced  int        // appid the poll interval was last skipped for
	steamLocalID int        // what the machine reported last tick, for quit detection
	steamGameID  int        // appid the current session is for
	steamGameAt  time.Time  // when that game first appeared
	startMs      int64
	lastPresence time.Time
	presenceOn   bool

	// L!VE preferences read from the site (refreshed periodically).
	settings   liveSettings
	settingsAt time.Time
}

// New builds an orchestrator from the saved config.
func New(cfg config.Config) *Orchestrator {
	return &Orchestrator{cfg: cfg, forzaGame: "fh6", forzaPort: 5300, forza: &forzaState{}}
}

// OnStatus registers a callback fired on every status change (nil is fine).
func (o *Orchestrator) OnStatus(fn func(Status)) { o.onStatus = fn }

// OnSeenGame registers the callback that records a Steam game into the local
// settings list (so the user can toggle it later). Fired once per new appid.
func (o *Orchestrator) OnSeenGame(fn func(appID int, name string)) { o.onSeenGame = fn }

// gameHidden reports whether the user turned this appid off in the app. Reads
// the live config, kept fresh by SetConfig on every save.
func (o *Orchestrator) gameHidden(appID int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, id := range o.cfg.HiddenGames {
		if id == appID {
			return true
		}
	}
	return false
}

// SetUsername sets the profile name used for the Rich Presence button.
func (o *Orchestrator) SetUsername(name string) {
	o.mu.Lock()
	o.username = name
	o.mu.Unlock()
}

// SetUserID sets the profile id used to read the anime "now watching" back.
func (o *Orchestrator) SetUserID(id string) {
	o.mu.Lock()
	o.userID = id
	o.mu.Unlock()
}

// SetConfig swaps the config the loop reads (token/delay changes take effect
// next tick; poll interval applies on the next Start).
func (o *Orchestrator) SetConfig(cfg config.Config) {
	o.mu.Lock()
	o.cfg = cfg
	o.mu.Unlock()
}

// Status returns the latest status snapshot.
func (o *Orchestrator) Status() Status {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.status
}

// Running reports whether the loop is active.
func (o *Orchestrator) Running() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.running
}

// Start begins the Forza listener + poll loop (no-op if already running).
func (o *Orchestrator) Start() {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = cancel
	o.running = true
	o.startMs = time.Now().UnixMilli()
	o.pushes = 0
	o.mu.Unlock()

	// Discord connects lazily per game (see presence → useApp), so the app id
	// matches whatever you're playing.

	go o.forzaListen(ctx)
	go o.loop(ctx)
}

// Stop cancels the loop and clears the status.
func (o *Orchestrator) Stop() {
	o.mu.Lock()
	if o.cancel != nil {
		o.cancel()
	}
	o.running = false
	o.status = Status{}
	dc := o.dc
	o.dc = nil
	o.dcApp = ""
	o.presenceOn = false
	o.mu.Unlock()
	if dc != nil {
		_ = dc.Clear()
		_ = dc.Close()
	}
	o.emit(Status{})
}

// forzaListen binds Data Out (UDP) and feeds parsed packets to the shared state.
// Best-effort: silently skips Forza if the port is taken.
func (o *Orchestrator) forzaListen(ctx context.Context) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: o.forzaPort})
	if err != nil {
		return
	}
	defer conn.Close()
	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		if n, _, rerr := conn.ReadFromUDP(buf); rerr == nil {
			if s, ok := forza.Parse(buf[:n], o.forzaGame); ok {
				o.forza.set(s)
			}
		}
	}
}

func (o *Orchestrator) loop(ctx context.Context) {
	o.mu.Lock()
	interval := time.Duration(o.cfg.PollMs) * time.Millisecond
	o.mu.Unlock()
	if interval < 500*time.Millisecond {
		interval = 1500 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	o.tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.tick()
		}
	}
}

func (o *Orchestrator) tick() {
	o.mu.Lock()
	cfg := o.cfg
	uname := o.username
	start := o.startMs
	uid := o.userID
	settings := o.settings
	stale := time.Since(o.settingsAt) > 60*time.Second
	o.mu.Unlock()

	// Pull the site's L!VE settings (delay + name masking) periodically, so what
	// the user sets on glow.moe → L!VE is exactly what the collector applies.
	if cfg.Token != "" && stale {
		if s, ok := fetchSettings(cfg.Endpoint, cfg.Token); ok {
			o.mu.Lock()
			o.settings = s
			o.settingsAt = time.Now()
			o.mu.Unlock()
			settings = s
		}
	}
	effDelay := settings.DelaySec
	if effDelay == 0 {
		effDelay = cfg.DelaySec // local fallback when the site delay is 0
	}

	// Reset the local overlay each tick; a League branch below re-populates it
	// (the OBS overlay is League-only, so other titles read as not-live).
	o.clearOverlay()

	// Forza wins while telemetry is fresh (you're driving).
	if fs, at := o.forza.get(); fs != nil && time.Since(at) < 10*time.Second {
		fs.UpdatedAt = time.Now().UnixMilli()
		st := Status{Game: "forza", InGame: true, Detail: forzaDetail(fs), Pushes: o.pushes, Delay: effDelay}
		o.push(cfg, effDelay, fs, &st)
		o.presence(forzaAppID(o.forzaGame), forzaActivity(fs, o.forzaGame, uname, start))
		o.set(st)
		return
	}

	// League: in a live game.
	if data, err := live.Fetch(); err == nil {
		snap := snapshot.Build(data, ddragon.Version(), time.Now().UnixMilli())
		// Names are pushed raw; glow.moe masks them at read time per the user's
		// current L!VE privacy settings, so toggling takes effect immediately.
		st := Status{Game: "league", InGame: true, Detail: leagueDetail(snap.Me.ChampName, snap.Clock), Pushes: o.pushes, Delay: effDelay}
		o.push(cfg, effDelay, snap, &st)
		o.cacheOverlay(snap)
		o.presence(orGlow(appLoL), leagueActivity(snap, data.GameData.GameTime, uname))
		o.set(st)
		return
	}

	// League out-of-game: the client is up but you're not in a match - lobby,
	// queue, champion select, the loading screen, post-game, or just sitting in
	// the client (which carries your rank / mastery / last-5 match results). We
	// push it exactly like the old console did, so the profile shows the same
	// out-of-game card AND mirror the state to Discord (under the LoL app, so it
	// still reads "Playing League of Legends"); the in-game HUD takes over once a
	// match loads.
	if lob, err := lcu.Fetch(cfg.LeaguePath); err == nil && lob != nil {
		snap := snapshot.FromLobby(lob, ddragon.Version(), time.Now().UnixMilli())
		st := Status{Game: "league", InGame: false, Detail: lob.Label, Pushes: o.pushes, Delay: effDelay}
		o.push(cfg, effDelay, snap, &st)
		o.cacheOverlay(snap)
		o.presence(orGlow(appLoL), leagueLobbyActivity(snap.Lobby, uname))
		o.set(st)
		return
	}

	// Steam: any other game launched through Steam. Sits below League and Forza
	// because those carry real telemetry, and above anime because a running game
	// beats a passive watch status. Steam's own Rich Presence often knows the map
	// or mode when Discord's does not, which is the whole point of reading it.
	if cfg.SteamPresence {
		if id, ok := o.steamAccount(); ok {
			if s, live := o.steamSnapshot(id); live {
				// Remember every game so the settings list can offer a toggle for
				// it later, even when it is not running. The callback dedupes.
				if o.onSeenGame != nil && s.AppID > 0 {
					o.onSeenGame(s.AppID, s.Game)
				}
				// The user can turn a game off entirely: nothing to the site, and
				// nothing to Discord. Their machine, their call - a privacy switch
				// that stays local.
				if o.gameHidden(s.AppID) {
					o.clearPresence()
					o.set(Status{Game: "steam", AppID: s.AppID, GameName: s.Game, Hidden: true, Delay: effDelay})
					return
				}
				st := Status{Game: "steam", InGame: true, Detail: steamDetail(s), AppID: s.AppID, GameName: s.Game, Pushes: o.pushes, Delay: effDelay}
				o.push(cfg, effDelay, o.steamSnapshotPayload(s), &st)
				app, named := steamApp(s.AppID)
				hints := steamGameHints(cfg.Endpoint, s.AppID)
				if hints.SelfRP {
					// The game drives its own, richer Discord presence. Step off
					// that slot so we do not fight it; the site still gets the
					// status above.
					o.clearPresence()
				} else if err := o.presence(app, steamActivity(s, uname, hints.Icon, named)); err != nil {
					st.Err = "Discord: " + err.Error()
				}
				o.set(st)
				return
			}
		}
	}

	// Anime: no game running, so mirror what you're watching (detected + pushed
	// by the browser extension, read back from glow.moe) to Discord. We only set
	// the local Rich Presence here; the extension owns detection and the push.
	if cfg.AnimePresence && cfg.Token != "" && uid != "" {
		if a, ok := fetchAnime(cfg.Endpoint, uid); ok {
			st := Status{Game: "anime", InGame: true, Detail: animeDetail(a), Pushes: o.pushes, Delay: effDelay}
			if err := o.presence(orGlow(""), animeActivity(a, uname)); err != nil {
				st.Err = "Discord: " + err.Error()
			}
			o.set(st)
			return
		}
	}

	// Nothing running.
	o.clearPresence()
	o.set(Status{Detail: "Waiting for a game…", Pushes: o.pushes, Delay: effDelay})
}

func (o *Orchestrator) push(cfg config.Config, delaySec int, snap any, st *Status) {
	if cfg.Token == "" {
		st.Err = "not linked"
		return
	}
	if err := poster.Post(cfg.Endpoint, cfg.Token, delaySec, snap); err != nil {
		st.Err = err.Error()
		return
	}
	o.pushes++
	st.Pushes = o.pushes
	st.Pushing = true
}

func (o *Orchestrator) set(s Status) {
	o.mu.Lock()
	o.status = s
	o.mu.Unlock()
	o.emit(s)
}

var overlayOffline = []byte(`{"live":false,"snapshot":null}`)

// cacheOverlay stores the current League snapshot (masked per the owner's L!VE
// privacy settings) for the local overlay server. Masks a COPY so the raw
// snapshot still goes to glow.moe (which masks at read time; keeping the raw
// push lets un-hiding work). No stream-snipe delay buffer: parity with the server today.
func (o *Orchestrator) cacheOverlay(snap snapshot.Snapshot) {
	snap.Blue = append([]snapshot.Player(nil), snap.Blue...)
	snap.Red = append([]snapshot.Player(nil), snap.Red...)
	o.mu.Lock()
	hideMy, hideEnemy := o.settings.HideMyName, o.settings.HideEnemyNames
	look := normalizeLook(o.settings)
	o.mu.Unlock()
	snap.Mask(hideMy, hideEnemy)
	b, err := json.Marshal(map[string]any{"live": snap.Live, "snapshot": snap, "look": look})
	if err != nil {
		return
	}
	o.mu.Lock()
	o.overlayJSON = b
	o.mu.Unlock()
}

// normalizeLook builds the overlay's visual config (accent + scale + which blocks
// show) for /live.json, mirroring normalizeLiveSettings on the site. Before the
// first successful settings fetch the struct is zero-valued, which would hide
// everything - so an unset (non-hex) accent falls back to the full default card.
func normalizeLook(s liveSettings) map[string]any {
	if !hexColor(s.OverlayAccent) {
		return map[string]any{
			"accent": "#f5c211",
			"scale":  1.0,
			"show":   map[string]any{"stats": true, "spells": true, "runes": true, "mode": "full"},
		}
	}
	scale := s.OverlayScale
	if scale < 0.5 || scale > 2 {
		scale = 1
	}
	mode := "full"
	if s.OverlayShow.Mode == "minimal" {
		mode = "minimal"
	}
	return map[string]any{
		"accent": s.OverlayAccent,
		"scale":  scale,
		"show": map[string]any{
			"stats":  s.OverlayShow.Stats,
			"spells": s.OverlayShow.Spells,
			"runes":  s.OverlayShow.Runes,
			"mode":   mode,
		},
	}
}

// hexColor reports whether s is a #rrggbb colour (the shape the site always
// stores overlayAccent in), used to tell a fetched settings struct from a
// zero-valued one.
func hexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// clearOverlay marks the local overlay as not-live (between games / other titles).
func (o *Orchestrator) clearOverlay() {
	o.mu.Lock()
	o.overlayJSON = overlayOffline
	o.mu.Unlock()
}

// OverlayJSON returns the latest masked League snapshot for the localhost overlay
// server ({live,snapshot} shape, matching /api/live/read).
func (o *Orchestrator) OverlayJSON() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.overlayJSON == nil {
		return overlayOffline
	}
	return o.overlayJSON
}

func (o *Orchestrator) emit(s Status) {
	if o.onStatus != nil {
		o.onStatus(s)
	}
}

// useApp returns a Discord client connected with appID, reconnecting if the app
// changed (each game has its own app, so "Playing X" reads the game name).
func (o *Orchestrator) useApp(appID string) (*discord.Client, error) {
	if appID == "" {
		return nil, fmt.Errorf("no Discord app id for this game")
	}
	o.mu.Lock()
	if o.dc != nil && o.dcApp == appID {
		dc := o.dc
		o.mu.Unlock()
		return dc, nil
	}
	old := o.dc
	o.dc = nil
	o.dcApp = ""
	o.mu.Unlock()
	if old != nil {
		_ = old.Clear()
		_ = old.Close()
	}
	dc, err := discord.Connect(appID)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	o.dc = dc
	o.dcApp = appID
	o.mu.Unlock()
	return dc, nil
}

// presence publishes a pre-built activity via the game's own app (so the header
// says "Playing <game>"), throttled to every 5s. The activity already carries
// its images, timestamps and buttons.
func (o *Orchestrator) presence(appID string, a discord.Activity) error {
	o.mu.Lock()
	throttled := time.Since(o.lastPresence) < 5*time.Second && o.dcApp == appID
	o.mu.Unlock()
	if throttled {
		return nil
	}
	dc, err := o.useApp(appID)
	if dc == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("Discord not running")
	}
	go func() { _ = dc.SetActivity(a) }()
	o.mu.Lock()
	o.lastPresence = time.Now()
	o.presenceOn = true
	o.mu.Unlock()
	return nil
}

func (o *Orchestrator) clearPresence() {
	o.mu.Lock()
	dc := o.dc
	on := o.presenceOn
	o.presenceOn = false
	o.mu.Unlock()
	if dc != nil && on {
		go func() { _ = dc.Clear() }()
	}
}

func forzaTitle(gameID string) string {
	switch gameID {
	case "fh6":
		return "Forza Horizon 6"
	case "fh5":
		return "Forza Horizon 5"
	case "fm":
		return "Forza Motorsport"
	}
	return "Forza"
}

func forzaActivity(s *forza.Snapshot, gameID, username string, startMs int64) discord.Activity {
	title := forzaTitle(gameID)
	details := title
	if s.Car.Name != "" {
		details = title + "  ·  " + s.Car.Name
	}
	state := fmt.Sprintf("%d mph  ·  Gear %s", s.Speed, s.Gear)
	if !s.Racing {
		state = "In the menus"
	}
	a := discord.Activity{
		Details:    details,
		State:      state,
		Timestamps: &discord.Timestamps{Start: startMs},
		Assets: &discord.Assets{
			LargeImage: forzaImage, // big: Forza art
			LargeText:  title,
			SmallImage: glowIcon, // corner: glow badge
			SmallText:  "glow.moe",
		},
	}
	if username != "" {
		a.Buttons = []discord.Button{{Label: "View live on Glow", URL: "https://glow.moe/" + username + "/forza"}}
	}
	return a
}

// leagueActivity is the rich in-game LoL presence: the champion skin tile (or an
// animated-skin GIF) as the big image, KDA/CS/gold, glow badge + two buttons.
func leagueActivity(s snapshot.Snapshot, gameSeconds float64, username string) discord.Activity {
	me := s.Me
	kda, cs := "", 0
	for _, p := range append(append([]snapshot.Player{}, s.Blue...), s.Red...) {
		if p.IsMe {
			kda, cs = p.Kda, p.Cs
			break
		}
	}
	details := me.ChampName
	if kda != "" {
		details += "   " + kda
	}
	large := me.SkinName
	if large == "" {
		large = me.ChampName
	}
	largeImg := ddragon.TileURL(me.ChampKey, me.Skin)
	if me.SkinVideoUrl != "" {
		if id := ddragon.SkinID(me.ChampKey, me.Skin); id > 0 {
			largeImg = fmt.Sprintf("https://glow.moe/skins/%d.gif", id)
		}
	}
	a := discord.Activity{
		Details:    details,
		State:      fmt.Sprintf("Lv %d  |  %d CS  |  %d gold", me.Level, cs, me.Gold),
		Timestamps: &discord.Timestamps{Start: (time.Now().Unix() - int64(gameSeconds)) * 1000},
		Assets: &discord.Assets{
			LargeImage: largeImg,
			LargeText:  large,
			SmallImage: glowIcon,
			SmallText:  "glow.moe",
		},
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "🔴 Live game", URL: "https://glow.moe/" + username + "/league"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

// leagueLobbyActivity is the out-of-game LoL presence (lobby / queue / champ
// select / in the client). It runs under the LoL app, so Discord still reads
// "Playing League of Legends"; the phase + queue/champ land in details/state.
func leagueLobbyActivity(lob *snapshot.Lobby, username string) discord.Activity {
	details := lob.Label // "In a lobby" / "In champion select" / "In the client" …
	state := lob.QueueName
	if state == "" {
		state = lob.ModeLabel
	}
	// In champ select the picked champion is the interesting bit.
	if lob.ChampName != "" {
		state = "Locked in " + lob.ChampName
	}
	if state == "" && lob.Level > 0 {
		state = fmt.Sprintf("Level %d", lob.Level)
	}
	// Big image: the picked champ tile if any, else the summoner icon.
	large, largeText := glowIcon, "League of Legends"
	if lob.ChampKey != "" {
		large, largeText = ddragon.TileURL(lob.ChampKey, 0), lob.ChampName
	} else if lob.IconURL != "" {
		large, largeText = lob.IconURL, lob.Summoner
	}
	a := discord.Activity{
		Details: details,
		State:   state,
		Assets: &discord.Assets{
			LargeImage: large,
			LargeText:  largeText,
			SmallImage: glowIcon,
			SmallText:  "glow.moe",
		},
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "League profile", URL: "https://glow.moe/" + username + "/league"},
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

// steamAccount resolves (and remembers) the Steam account signed in on this
// machine. Cached because it comes from a file on disk and the poll loop runs
// every couple of seconds; re-read periodically so switching accounts is picked
// up without a restart.
func (o *Orchestrator) steamAccount() (uint32, bool) {
	o.mu.Lock()
	id, at := o.steamID, o.steamIDAt
	o.mu.Unlock()
	if id != 0 && time.Since(at) < 5*time.Minute {
		return id, true
	}
	if id == 0 && time.Since(at) < 30*time.Second {
		return 0, false // no Steam here; do not stat the disk every tick
	}
	found, ok := steam.AccountID()
	o.mu.Lock()
	o.steamID, o.steamIDAt = found, time.Now()
	o.mu.Unlock()
	return found, ok
}

// How often the Steam community endpoint may be asked, and how long a good
// answer stays usable. The poll loop runs every ~1.5s, which is far too fast for
// this endpoint: hammering it earns throttled empty replies, and each of those
// used to drop the presence and bring it straight back, so the card flickered.
// The grace window keeps the last good answer through a failed poll or two.
//
// Neither delay applies to starting, quitting or switching a game: those come
// from the machine itself and are acted on the moment they happen.
const (
	steamPollEvery = 25 * time.Second
	steamKeepFor   = 90 * time.Second
)

// steamSnapshot returns what Steam last reported, refreshing at most every
// steamPollEvery. A failed refresh does not immediately clear the presence: the
// previous answer stands until it goes stale.
func (o *Orchestrator) steamSnapshot(id uint32) (steam.Snap, bool) {
	o.mu.Lock()
	snap, good, tried, forced := o.steamSnap, o.steamSnapAt, o.steamPollAt, o.steamForced
	o.mu.Unlock()

	// Which game is running is a local question, so it is asked every tick. The
	// endpoint is only consulted for what it alone knows: the game's own status
	// line and the player's Steam profile.
	localID, _, localOK := steam.RunningApp()
	o.mu.Lock()
	prevLocal := o.steamLocalID
	if localOK {
		o.steamLocalID = localID
	}
	o.mu.Unlock()
	// Quitting THIS machine's game clears instantly: no reason to wait out a
	// grace window meant for a network that went quiet. Scoped to the game the
	// machine itself was running, because the card may be showing a game played
	// on another PC, and this machine cannot declare that one closed.
	if localOK && localID == 0 && prevLocal != 0 && snap.AppID == prevLocal {
		o.mu.Lock()
		o.steamSnap, o.steamSnapAt = steam.Snap{}, time.Time{}
		o.mu.Unlock()
		return steam.Snap{}, false
	}
	// Skipping the interval is a one-off per game, not a standing licence: a game
	// whose name the local library cannot supply, asked while the endpoint is
	// down, would otherwise look freshly switched on every tick forever.
	switched := localOK && localID != 0 && snap.AppID != 0 && localID != snap.AppID && localID != forced

	fresh := !good.IsZero() && time.Since(good) < steamKeepFor
	if !switched && time.Since(tried) < steamPollEvery {
		return snap, fresh && snap.Game != ""
	}

	s, live := steam.Fetch(id)

	o.mu.Lock()
	o.steamPollAt = time.Now()
	if switched {
		o.steamForced = localID
	}
	if live {
		o.steamSnap, o.steamSnapAt = s, time.Now()
	} else if !fresh {
		// Confirmed idle (not a blip): forget the game.
		o.steamSnap, o.steamSnapAt = steam.Snap{}, time.Time{}
	}
	snap, fresh = o.steamSnap, live || fresh
	o.mu.Unlock()
	return snap, fresh && snap.Game != ""
}

// steamAppID picks the Discord application to publish a Steam game under.
// Discord ties the "Playing X" headline to the application, not to the payload,
// so using the game's own registered app is what makes it read "Playing
// Unturned". Games Discord does not know fall back to ours, which still shows
// the title on the details line.
// steamPayload is what the profile card on glow.moe receives. The library side
// (playtime, achievements, genres) is already in the site's own database from
// the Steam sync, so only the live half travels: what is running, the game's own
// status line, and the player extras that only the live endpoint knows.
type steamPayload struct {
	Game      string `json:"game"`
	AppID     int    `json:"appId"`
	Title     string `json:"title"`
	Detail    string `json:"detail,omitempty"`
	NonSteam  bool   `json:"nonSteam,omitempty"`
	StartedAt int64  `json:"startedAt"` // ms; drives "42 min this session"
	UpdatedAt int64  `json:"updatedAt"`

	Level      int    `json:"level,omitempty"`
	Avatar     string `json:"avatar,omitempty"`
	Persona    string `json:"persona,omitempty"`
	BadgeName  string `json:"badgeName,omitempty"`
	BadgeDesc  string `json:"badgeDesc,omitempty"`
	BadgeIcon  string `json:"badgeIcon,omitempty"`
	Background string `json:"background,omitempty"`
}

// steamSnapshotPayload builds the push body, stamping the session start so the
// card can count from when this game actually appeared rather than from the
// moment a viewer loaded the page.
func (o *Orchestrator) steamSnapshotPayload(s steam.Snap) steamPayload {
	o.mu.Lock()
	if o.steamGameAt.IsZero() || o.steamGameID != s.AppID {
		o.steamGameID, o.steamGameAt = s.AppID, time.Now()
	}
	started := o.steamGameAt
	o.mu.Unlock()
	return steamPayload{
		Game:       "steam",
		AppID:      s.AppID,
		Title:      s.Game,
		Detail:     s.Detail,
		NonSteam:   s.NonSteam,
		StartedAt:  started.UnixMilli(),
		UpdatedAt:  time.Now().UnixMilli(),
		Level:      s.Level,
		Avatar:     s.Avatar,
		Persona:    s.Persona,
		BadgeName:  s.BadgeName,
		BadgeDesc:  s.BadgeDesc,
		BadgeIcon:  s.BadgeIcon,
		Background: s.Background,
	}
}

// named reports whether the returned application is the game's own, which is
// what decides if the headline already carries the title.
func steamApp(appID int) (app string, named bool) {
	if id, ok := steam.DiscordAppID(appID); ok {
		return strconv.FormatUint(id, 10), true
	}
	return orGlow(""), false
}

// steamDetail is the one-line status the app shows for a Steam game.
func steamDetail(s steam.Snap) string {
	if s.Detail != "" {
		return s.Game + " · " + s.Detail
	}
	return s.Game
}

// steamActivity builds the Rich Presence for a Steam game.
//
// Nothing here is per-game: the detail line arrives from Steam already
// rendered, and when Discord knows the game we publish under its own
// application, so a title works the day it ships without anything being
// registered or uploaded on our side.
// steamHints is what glow tells the app about a Steam game for Discord: the
// square icon to use, and whether the game reports its own Rich Presence.
type steamHints struct {
	Icon   string `json:"icon"`
	SelfRP bool   `json:"selfRP"`
}

// steamGameHints asks glow (which holds the SteamGridDB key) for a game's icon
// and self-RP flag. Cached per appid for the process lifetime: one request the
// first time a game appears, none after. A failure returns the zero value,
// which means "no icon, not self-RP" - the safe default that keeps our presence
// behaving as before.
var (
	hintMu    sync.Mutex
	hintCache = map[int]steamHints{}
)

func steamGameHints(endpoint string, appID int) steamHints {
	if appID <= 0 {
		return steamHints{}
	}
	hintMu.Lock()
	cached, hit := hintCache[appID]
	hintMu.Unlock()
	if hit {
		return cached
	}
	u := pair.BaseFrom(endpoint) + "/api/steam/icon?appId=" + strconv.Itoa(appID) + "&os=" + runtime.GOOS
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Get(u)
	if err != nil {
		return steamHints{} // do not cache a transient failure
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return steamHints{}
	}
	var out steamHints
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return steamHints{}
	}
	hintMu.Lock()
	hintCache[appID] = out
	hintMu.Unlock()
	return out
}

func steamActivity(s steam.Snap, username, large string, named bool) discord.Activity {
	// Published under the game's own Discord app, the headline already reads
	// "Playing Unturned", so repeating the title here would print it twice: say
	// where it is running instead. Without a match the headline is our app, and
	// the title has to appear somewhere.
	details := s.Game
	if named {
		details = "on Steam"
	}
	a := discord.Activity{
		Details: details,
		// The game's own line goes in state: it is the field the profile card
		// reads, so the map or mode survives the trip to glow.moe. Omitted by the
		// encoder for the many games that publish none.
		State: s.Detail,
	}
	// The large image is a square icon when glow could resolve one (Discord crops
	// this slot square, so a native icon beats a cropped cover), else the box
	// art, else our own badge. The glow badge always rides the corner, which is
	// why a large image of our own must always be set.
	if large == "" {
		large = steam.CoverImage(s.AppID)
	}
	if large == "" {
		large = glowIcon
	}
	a.Assets = &discord.Assets{
		LargeImage: large,
		LargeText:  s.Game,
		SmallImage: glowIcon,
		SmallText:  "glow.moe",
	}
	if username != "" {
		a.Buttons = []discord.Button{
			{Label: "View my Glow profile", URL: "https://glow.moe/" + username},
		}
	}
	return a
}

func forzaDetail(fs *forza.Snapshot) string {
	name := fs.Car.Name
	if name == "" {
		name = "Forza"
	}
	return fmt.Sprintf("%s · %d mph · gear %s", name, fs.Speed, fs.Gear)
}

func leagueDetail(champ, clock string) string {
	if champ == "" {
		return "In game · " + clock
	}
	return champ + " · " + clock
}
