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
	"sync"
	"time"

	"github.com/glow-moe/glow-collector/internal/config"
	"github.com/glow-moe/glow-collector/internal/ddragon"
	"github.com/glow-moe/glow-collector/internal/discord"
	"github.com/glow-moe/glow-collector/internal/forza"
	"github.com/glow-moe/glow-collector/internal/live"
	"github.com/glow-moe/glow-collector/internal/pair"
	"github.com/glow-moe/glow-collector/internal/poster"
	"github.com/glow-moe/glow-collector/internal/snapshot"
)

// liveSettings mirrors the L!VE preferences the user sets on the site (the
// dashboard's L!VE tab). The collector reads them from /api/live/settings so the
// GUI never needs its own copy - the site is the single source of truth.
type liveSettings struct {
	HideEnemyNames bool `json:"hideEnemyNames"`
	HideMyName     bool `json:"hideMyName"`
	DelaySec       int  `json:"delaySec"`
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
	Game    string `json:"game"`    // "" | "league" | "forza"
	InGame  bool   `json:"inGame"`  // a game is being read
	Detail  string `json:"detail"`  // e.g. "Ahri · 12:34" or "Forza · 240 mph"
	Pushing bool   `json:"pushing"` // last tick pushed to glow.moe
	Pushes  int    `json:"pushes"`  // total pushes this session
	Err     string `json:"err"`     // last error ("" when fine)
	Delay   int    `json:"delay"`   // applied stream delay (from the site), seconds
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
	mu        sync.Mutex
	cfg       config.Config
	forzaGame string
	forzaPort int
	cancel    context.CancelFunc
	running   bool
	status    Status
	pushes    int
	forza     *forzaState
	onStatus  func(Status)

	// Discord Rich Presence (best-effort; nil when Discord isn't running).
	dc           *discord.Client
	dcApp        string // app id the current client is connected with
	username     string
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

// SetUsername sets the profile name used for the Rich Presence button.
func (o *Orchestrator) SetUsername(name string) {
	o.mu.Lock()
	o.username = name
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
		o.presence(orGlow(appLoL), leagueActivity(snap, data.GameData.GameTime, uname))
		o.set(st)
		return
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

func (o *Orchestrator) emit(s Status) {
	if o.onStatus != nil {
		o.onStatus(s)
	}
}

// useApp returns a Discord client connected with appID, reconnecting if the app
// changed (each game has its own app, so "Playing X" reads the game name).
func (o *Orchestrator) useApp(appID string) *discord.Client {
	o.mu.Lock()
	if o.dc != nil && o.dcApp == appID {
		dc := o.dc
		o.mu.Unlock()
		return dc
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
		return nil
	}
	o.mu.Lock()
	o.dc = dc
	o.dcApp = appID
	o.mu.Unlock()
	return dc
}

// presence publishes a pre-built activity via the game's own app (so the header
// says "Playing <game>"), throttled to every 5s. The activity already carries
// its images, timestamps and buttons.
func (o *Orchestrator) presence(appID string, a discord.Activity) {
	o.mu.Lock()
	throttled := time.Since(o.lastPresence) < 5*time.Second && o.dcApp == appID
	o.mu.Unlock()
	if throttled {
		return
	}
	dc := o.useApp(appID)
	if dc == nil {
		return
	}
	go func() { _ = dc.SetActivity(a) }()
	o.mu.Lock()
	o.lastPresence = time.Now()
	o.presenceOn = true
	o.mu.Unlock()
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
