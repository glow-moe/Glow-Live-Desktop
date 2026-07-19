// Package valorant reads VALORANT's local presence to report the player's
// current state (in menus / agent select / in a match), the map, mode, rank and
// live score - enough to drive Discord Rich Presence. It reuses the Riot Client
// lockfile (name:pid:port:password:protocol) written under LOCALAPPDATA and the
// local chat API (GET /chat/v4/presences), whose per-player `private` field is a
// base64 JSON blob with VALORANT's `sessionLoopState`, map, scores and tier.
//
// Everything here is best-effort and read-only; when VALORANT isn't running the
// lockfile is absent and Fetch returns ErrNoClient.
package valorant

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoClient means the Riot Client / VALORANT isn't running (no lockfile).
var ErrNoClient = errors.New("valorant not running")

// State is the resolved, display-ready VALORANT presence.
type State struct {
	Game       string `json:"game"` // always "valorant" (routes the site ingest)
	Running    bool   `json:"running"`
	GameName   string `json:"gameName"`
	TagLine    string `json:"tagLine"`
	Level      int    `json:"level"`
	Phase      string `json:"phase"` // MENUS | PREGAME | INGAME
	MapCode    string `json:"mapCode"`
	MapName    string `json:"mapName"`
	MapUUID    string `json:"mapUuid"`
	Queue      string `json:"queue"`
	ModeName   string `json:"modeName"`
	AllyScore  int    `json:"allyScore"`
	EnemyScore int    `json:"enemyScore"`
	/** True for a real scored match (hide the score for the range / menus). */
	Scored     bool   `json:"scored"`
	Tier       int    `json:"tier"`
	RankName   string `json:"rankName"`
	PartySize  int    `json:"partySize"`
	/** Selected agent (from the GLZ pregame/core-game API; presence has none). */
	Agent     string `json:"agent"`
	AgentUUID string `json:"agentUuid"`
	/** Everyone in agent select / the match, with their agent (GLZ). */
	Roster []RosterPlayer `json:"roster"`
}

// lockfileDir is where the Riot *Client* (not League) writes its lockfile.
func lockfilePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Riot Games", "Riot Client", "Config", "lockfile")
}

// lockCreds reads the Riot Client lockfile → local API port + password.
func lockCreds() (port, password string, err error) {
	lf := lockfilePath()
	if lf == "" {
		return "", "", ErrNoClient
	}
	b, e := os.ReadFile(lf)
	if e != nil {
		return "", "", ErrNoClient
	}
	parts := strings.Split(strings.TrimSpace(string(b)), ":")
	if len(parts) < 5 {
		return "", "", errors.New("lockfile malformed")
	}
	return parts[2], parts[3], nil
}

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 - localhost Riot cert
	},
}

// sessionResp is GET /chat/v1/session - our own identity + puuid, so we read
// OUR presence out of the list and not a friend's (friends broadcast a reduced
// `private` blob with level/rank hidden, which is what made those show as 0).
type sessionResp struct {
	PUUID    string `json:"puuid"`
	GameName string `json:"game_name"`
	GameTag  string `json:"game_tag"`
}

// presenceResp is the shape of GET /chat/v4/presences we care about.
type presenceResp struct {
	Presences []struct {
		PUUID    string `json:"puuid"`
		Product  string `json:"product"`
		GameName string `json:"game_name"`
		GameTag  string `json:"game_tag"`
		Private  string `json:"private"` // base64 JSON
	} `json:"presences"`
}

// privateBlob is VALORANT's decoded per-player `private` field. Current clients
// nest it under matchPresenceData / partyPresenceData / playerPresenceData; a
// few fields are also mirrored at the top level, kept here as fallbacks.
type privateBlob struct {
	Match struct {
		MatchMap         string `json:"matchMap"`
		QueueID          string `json:"queueId"`
		SessionLoopState string `json:"sessionLoopState"`
		ProvisioningFlow string `json:"provisioningFlow"`
	} `json:"matchPresenceData"`
	Party struct {
		PartyOwnerMatchMap string `json:"partyOwnerMatchMap"`
		ScoreAlly          int    `json:"partyOwnerMatchScoreAllyTeam"`
		ScoreFoe           int    `json:"partyOwnerMatchScoreEnemyTeam"`
		PartySize          int    `json:"partySize"`
	} `json:"partyPresenceData"`
	Player struct {
		AccountLevel    int `json:"accountLevel"`
		CompetitiveTier int `json:"competitiveTier"`
	} `json:"playerPresenceData"`

	// Top-level mirrors / older flat format.
	SessionLoopState string `json:"sessionLoopState"`
	ProvisioningFlow string `json:"provisioningFlow"`
	QueueID          string `json:"queueId"`
	ScoreAlly        int    `json:"partyOwnerMatchScoreAllyTeam"`
	ScoreFoe         int    `json:"partyOwnerMatchScoreEnemyTeam"`
	AccountLevel     int    `json:"accountLevel"`
	CompetitiveTier  int    `json:"competitiveTier"`
	PartySize        int    `json:"partySize"`
}

func firstNon0(v ...int) int {
	for _, x := range v {
		if x != 0 {
			return x
		}
	}
	return 0
}

// get does an authenticated local GET against the Riot Client API.
func get(port, password, path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+port+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization",
		"Basic "+base64.StdEncoding.EncodeToString([]byte("riot:"+password)))
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// Fetch reads OUR local presence and returns the current VALORANT state, or
// ErrNoClient when nothing is running.
func Fetch() (*State, error) {
	lf := lockfilePath()
	if lf == "" {
		return nil, ErrNoClient
	}
	raw, err := os.ReadFile(lf)
	if err != nil {
		// No readable lockfile ⇒ client not running (mirrors lcu's behaviour).
		return nil, ErrNoClient
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), ":")
	if len(parts) < 5 {
		return nil, errors.New("lockfile malformed")
	}
	port, password := parts[2], parts[3]

	var sess sessionResp
	if err := get(port, password, "/chat/v1/session", &sess); err != nil {
		return nil, err
	}
	if sess.PUUID == "" {
		return nil, ErrNoClient // chat not up yet
	}

	var pr presenceResp
	if err := get(port, password, "/chat/v4/presences", &pr); err != nil {
		return nil, err
	}
	for _, p := range pr.Presences {
		if p.PUUID != sess.PUUID || p.Product != "valorant" || p.Private == "" {
			continue
		}
		dec, derr := base64.StdEncoding.DecodeString(p.Private)
		if derr != nil {
			continue
		}
		var pb privateBlob
		if json.Unmarshal(dec, &pb) != nil {
			continue
		}
		st := build(sess.GameName, sess.GameTag, pb)
		// The agent isn't in presence - pull it from the GLZ API during agent
		// select / a live match (cheap: tokens are cached, 2 GLZ calls).
		if st.Phase == "PREGAME" || st.Phase == "INGAME" {
			if t := cachedTokens(port, password); t != nil {
				st.Roster = t.roster()
				for _, p := range st.Roster {
					if p.Self {
						st.Agent = p.Agent
						st.AgentUUID = p.AgentUUID
						break
					}
				}
			}
		}
		return st, nil
	}
	// Riot Client up but no VALORANT presence for us ⇒ VALORANT isn't running.
	return nil, ErrNoClient
}

// Dump returns a read-only diagnostic of the local Riot session + every
// presence, with each VALORANT `private` blob base64-decoded and pretty-printed
// so we can see the real field names/values. Used by `-valdump`.
func Dump() string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }

	lf := lockfilePath()
	w("lockfile: " + lf)
	raw, err := os.ReadFile(lf)
	if err != nil {
		w("  read error (Riot Client not running?): " + err.Error())
		return b.String()
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), ":")
	if len(parts) < 5 {
		w("  lockfile malformed")
		return b.String()
	}
	port, password := parts[2], parts[3]
	w("port: " + port + "\n")

	var sess sessionResp
	if e := get(port, password, "/chat/v1/session", &sess); e != nil {
		w("SESSION error: " + e.Error())
	} else {
		w(fmt.Sprintf("SELF puuid=%s  name=%s#%s\n", sess.PUUID, sess.GameName, sess.GameTag))
	}

	var pr presenceResp
	if e := get(port, password, "/chat/v4/presences", &pr); e != nil {
		w("PRESENCES error: " + e.Error())
		return b.String()
	}
	w(fmt.Sprintf("presences: %d total", len(pr.Presences)))
	for i, p := range pr.Presences {
		w(fmt.Sprintf("[%d] product=%s self=%v name=%s#%s privateLen=%d",
			i, p.Product, p.PUUID == sess.PUUID, p.GameName, p.GameTag, len(p.Private)))
		if p.Product != "valorant" || p.Private == "" {
			continue
		}
		dec, derr := base64.StdEncoding.DecodeString(p.Private)
		if derr != nil {
			w("   private decode error: " + derr.Error())
			continue
		}
		var pretty bytes.Buffer
		if json.Indent(&pretty, dec, "   ", "  ") == nil {
			w("   private:\n   " + pretty.String())
		} else {
			w("   private(raw): " + string(dec))
		}
	}
	return b.String()
}

func build(name, tag string, pb privateBlob) *State {
	code := mapCode(orElse(pb.Party.PartyOwnerMatchMap, pb.Match.MatchMap))
	m := maps[code]
	phase := orElse(pb.Match.SessionLoopState, pb.SessionLoopState)
	queue := orElse(pb.Match.QueueID, pb.QueueID)
	flow := orElse(pb.Match.ProvisioningFlow, pb.ProvisioningFlow)
	tier := firstNon0(pb.Player.CompetitiveTier, pb.CompetitiveTier)
	mode := modeName(queue)
	if flow == "ShootingRange" {
		mode = "The Range"
	}
	// Only the shooting range has no ally-enemy score; everything else in-game
	// (matchmade or custom) does.
	scored := phase == "INGAME" && flow != "ShootingRange"
	return &State{
		Game:       "valorant",
		Running:    true,
		GameName:   name,
		TagLine:    tag,
		Level:      firstNon0(pb.Player.AccountLevel, pb.AccountLevel),
		Phase:      phase,
		MapCode:    code,
		MapName:    orElse(m.Name, code),
		MapUUID:    m.UUID,
		Queue:      queue,
		ModeName:   mode,
		AllyScore:  firstNon0(pb.Party.ScoreAlly, pb.ScoreAlly),
		EnemyScore: firstNon0(pb.Party.ScoreFoe, pb.ScoreFoe),
		Scored:     scored,
		Tier:       tier,
		RankName:   rankName(tier),
		PartySize:  firstNon0(pb.Party.PartySize, pb.PartySize),
	}
}

// SplashURL is the valorant-api map splash for the RPC large image (empty when
// the map is unknown or the player is in menus).
func (s *State) SplashURL() string {
	if s.MapUUID == "" {
		return ""
	}
	return "https://media.valorant-api.com/maps/" + s.MapUUID + "/splash.png"
}

// ── Lookup tables ─────────────────────────────────────────────────

type mapInfo struct{ Name, UUID string }

// Keyed by the map *code name* (the last path segment of partyOwnerMatchMap,
// e.g. "/Game/Maps/Duality/Duality" → "Duality" → Bind).
var maps = map[string]mapInfo{
	"Ascent":  {"Ascent", "7eaecc1b-4337-bbf6-6ab9-04b8f06b3319"},
	"Duality": {"Bind", "2c9d57ec-4431-9c5e-2939-8f9ef6dd5cba"},
	"Triad":   {"Haven", "2bee0dc9-4ffe-519b-1cbd-7fbe763a6047"},
	"Bonsai":  {"Split", "d960549e-485c-e861-8d71-aa9d1aed12a2"},
	"Port":    {"Icebox", "e2ad5c54-4114-a870-9641-8ea21279579a"},
	"Foxtrot": {"Breeze", "2fb9a4fd-47b8-4e7d-a969-74b4046ebd53"},
	"Canyon":  {"Fracture", "b529448b-4d60-346e-e89e-00a4c527a405"},
	"Pitt":    {"Pearl", "fd267378-4d1d-484f-ff52-77821ed10dc2"},
	"Jam":     {"Lotus", "2fe4ed3a-450a-948b-6d6b-e89a78e680a9"},
	"Juliett": {"Sunset", "92584fbe-486a-b1b2-9faa-39b0f486b498"},
	"Infinity": {"Abyss", "224b0a95-48b9-f703-1bd8-67aca101a61f"},
	"Range":    {"The Range", "ee613ee9-28b7-4beb-9666-08db13bb2244"},
	"RangeV2":  {"The Range", "ee613ee9-28b7-4beb-9666-08db13bb2244"},
}

func mapCode(path string) string {
	if path == "" {
		return ""
	}
	seg := strings.Split(strings.Trim(path, "/"), "/")
	return seg[len(seg)-1]
}

var modes = map[string]string{
	"competitive": "Competitive",
	"unrated":     "Unrated",
	"swiftplay":   "Swiftplay",
	"spikerush":   "Spike Rush",
	"deathmatch":  "Deathmatch",
	"ggteam":      "Escalation",
	"hurm":        "Team Deathmatch",
	"onefa":       "Replication",
	"snowball":    "Snowball Fight",
	"newmap":      "New Map",
	"":            "Custom",
}

func modeName(q string) string {
	if m, ok := modes[strings.ToLower(q)]; ok {
		return m
	}
	if q == "" {
		return "Custom"
	}
	return strings.ToUpper(q[:1]) + q[1:]
}

var tiers = []string{"Iron", "Bronze", "Silver", "Gold", "Platinum", "Diamond", "Ascendant", "Immortal"}

func rankName(n int) string {
	if n <= 2 {
		return "Unranked"
	}
	if n >= 27 {
		return "Radiant"
	}
	idx := (n - 3) / 3
	div := (n-3)%3 + 1
	if idx < 0 || idx >= len(tiers) {
		return "Unranked"
	}
	return tiers[idx] + " " + string(rune('0'+div))
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
