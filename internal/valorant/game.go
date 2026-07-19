package valorant

// Agent resolution via Riot's authenticated GLZ API. The local presence blob
// carries no agent, so we mint the local access + entitlement tokens, discover
// the player's affinity (region/shard), then query the pregame (agent select)
// and core-game (in-match) endpoints for our own CharacterID → agent name.
//
// All read-only, localhost auth + Riot's own edge. Best-effort: any failure
// leaves the agent blank and the presence still works.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Token cache - the auth chain (entitlements + PAS affinity + version) is stable
// for a session, so we mint it at most every few minutes and reuse it.
var (
	tokMu    sync.Mutex
	tokCache *tokens
	tokAt    time.Time
)

func cachedTokens(port, password string) *tokens {
	tokMu.Lock()
	defer tokMu.Unlock()
	if tokCache != nil && time.Since(tokAt) < 3*time.Minute {
		return tokCache
	}
	t, err := auth(port, password)
	if err != nil {
		return tokCache // reuse the last good one on a transient failure (may be nil)
	}
	tokCache = t
	tokAt = time.Now()
	return t
}

// clientPlatform is the standard base64 platform blob GLZ expects.
const clientPlatform = "ew0KCSJwbGF0Zm9ybVR5cGUiOiAiUEMiLA0KCSJwbGF0Zm9ybU9TIjogIldpbmRvd3MiLA0KCSJwbGF0Zm9ybU9TVmVyc2lvbiI6ICIxMC4wLjE5MDQyLjEuMjU2LjY0Yml0IiwNCgkicGxhdGZvcm1DaGlwc2V0IjogIlVua25vd24iDQp9"

type tokens struct {
	access      string
	entitlement string
	puuid       string
	region      string
	shard       string
	version     string
}

// entitlementsResp is GET /entitlements/v1/token.
type entitlementsResp struct {
	AccessToken string `json:"accessToken"`
	Token       string `json:"token"`
	Subject     string `json:"subject"`
}

// auth mints the tokens + resolves region/shard/version. Reuses the Riot Client
// lockfile (port/password) already read by Fetch.
func auth(port, password string) (*tokens, error) {
	var et entitlementsResp
	if err := get(port, password, "/entitlements/v1/token", &et); err != nil {
		return nil, err
	}
	if et.AccessToken == "" || et.Token == "" || et.Subject == "" {
		return nil, errors.New("no entitlements yet")
	}
	region, shard, err := affinity(et.AccessToken)
	if err != nil {
		return nil, err
	}
	return &tokens{
		access:      et.AccessToken,
		entitlement: et.Token,
		puuid:       et.Subject,
		region:      region,
		shard:       shard,
		version:     clientVersion(),
	}, nil
}

// affinity asks Riot's PAS service which region/shard this player is on.
func affinity(access string) (region, shard string, err error) {
	req, _ := http.NewRequest(http.MethodGet,
		"https://riot-geo.pas.si.riotgames.com/pas/v1/service/chat", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// The body is a JWT: header.payload.signature - decode the payload.
	parts := strings.Split(strings.TrimSpace(string(body)), ".")
	if len(parts) < 2 {
		return "", "", errors.New("bad pas token")
	}
	pl, derr := base64.RawURLEncoding.DecodeString(parts[1])
	if derr != nil {
		return "", "", derr
	}
	var claims struct {
		Affinity string `json:"affinity"`
	}
	if json.Unmarshal(pl, &claims) != nil || claims.Affinity == "" {
		return "", "", errors.New("no affinity")
	}
	return claims.Affinity, shardOf(claims.Affinity), nil
}

// shardOf maps a play region to its data shard.
func shardOf(region string) string {
	switch region {
	case "latam", "br":
		return "na"
	default:
		return region // na, eu, ap, kr
	}
}

// clientVersion pulls the current riotClientVersion (no auth) so GLZ accepts us.
func clientVersion() string {
	resp, err := httpClient.Get("https://valorant-api.com/v1/version")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var v struct {
		Data struct {
			RiotClientVersion string `json:"riotClientVersion"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil {
		return ""
	}
	return v.Data.RiotClientVersion
}

// glzGet does an authenticated GLZ GET.
func (t *tokens) glzGet(path string, out any) (int, error) {
	base := "https://glz-" + t.region + "-1." + t.shard + ".a.pvp.net"
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+t.access)
	req.Header.Set("X-Riot-Entitlements-JWT", t.entitlement)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	if t.version != "" {
		req.Header.Set("X-Riot-ClientVersion", t.version)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, errors.New(resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, json.Unmarshal(body, out)
}

// agentID resolves our currently-selected agent's CharacterID (agent select or
// in-match). Returns "" when neither is active (e.g. in the menus).
func (t *tokens) agentID() string {
	// Agent select.
	var pg struct {
		MatchID string `json:"MatchID"`
	}
	if code, _ := t.glzGet("/pregame/v1/players/"+t.puuid, &pg); code == http.StatusOK && pg.MatchID != "" {
		var m struct {
			AllyTeam struct {
				Players []struct {
					Subject     string `json:"Subject"`
					CharacterID string `json:"CharacterID"`
				} `json:"Players"`
			} `json:"AllyTeam"`
		}
		if code, _ := t.glzGet("/pregame/v1/matches/"+pg.MatchID, &m); code == http.StatusOK {
			for _, p := range m.AllyTeam.Players {
				if strings.EqualFold(p.Subject, t.puuid) {
					return p.CharacterID
				}
			}
		}
	}
	// In-match.
	var cg struct {
		MatchID string `json:"MatchID"`
	}
	if code, _ := t.glzGet("/core-game/v1/players/"+t.puuid, &cg); code == http.StatusOK && cg.MatchID != "" {
		var m struct {
			Players []struct {
				Subject     string `json:"Subject"`
				CharacterID string `json:"CharacterID"`
			} `json:"Players"`
		}
		if code, _ := t.glzGet("/core-game/v1/matches/"+cg.MatchID, &m); code == http.StatusOK {
			for _, p := range m.Players {
				if strings.EqualFold(p.Subject, t.puuid) {
					return p.CharacterID
				}
			}
		}
	}
	return ""
}

// rawEdge does an authenticated GET against a PD/GLZ edge, returning the raw body.
func (t *tokens) rawEdge(base, path string) ([]byte, int) {
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("Authorization", "Bearer "+t.access)
	req.Header.Set("X-Riot-Entitlements-JWT", t.entitlement)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	if t.version != "" {
		req.Header.Set("X-Riot-ClientVersion", t.version)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

// rawLocal does a basic-auth GET against the local Riot Client API.
func rawLocal(port, password, path string) ([]byte, int) {
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:"+port+path, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("Authorization",
		"Basic "+base64.StdEncoding.EncodeToString([]byte("riot:"+password)))
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode
}

func firstMatchID(body []byte) string {
	var h struct {
		History []struct {
			MatchID string `json:"MatchID"`
		} `json:"History"`
	}
	if json.Unmarshal(body, &h) == nil && len(h.History) > 0 {
		return h.History[0].MatchID
	}
	return ""
}

func matchIDField(body []byte) string {
	var m struct {
		MatchID string `json:"MatchID"`
	}
	if json.Unmarshal(body, &m) == nil {
		return m.MatchID
	}
	return ""
}

// DumpFull pulls EVERY authenticated endpoint we can and dumps the raw JSON, so
// we can see exactly what data is available for the showcase. Run in a match for
// the richest output. Sections are capped so the file stays manageable.
func DumpFull() string {
	port, password, err := lockCreds()
	if err != nil {
		return "lockfile: " + err.Error()
	}
	t, aerr := auth(port, password)
	if aerr != nil {
		return "auth error: " + aerr.Error()
	}
	pd := "https://pd." + t.shard + ".a.pvp.net"
	glz := "https://glz-" + t.region + "-1." + t.shard + ".a.pvp.net"

	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }
	const maxLen = 30000
	dump := func(title string, body []byte, code int) {
		w("\n===== " + title + fmt.Sprintf("  (HTTP %d, %d bytes) =====", code, len(body)))
		if len(body) == 0 {
			return
		}
		var pretty bytes.Buffer
		out := body
		if json.Indent(&pretty, body, "", "  ") == nil {
			out = pretty.Bytes()
		}
		if len(out) > maxLen {
			w(string(out[:maxLen]) + "\n…[truncated]")
		} else {
			w(string(out))
		}
	}
	edge := func(title, base, path string) []byte {
		body, code := t.rawEdge(base, path)
		dump(title, body, code)
		return body
	}

	w(fmt.Sprintf("puuid=%s  region=%s  shard=%s  version=%s", t.puuid, t.region, t.shard, t.version))

	edge("MMR / RANK", pd, "/mmr/v1/players/"+t.puuid)
	edge("COMPETITIVE UPDATES", pd, "/mmr/v1/players/"+t.puuid+"/competitiveupdates?startIndex=0&endIndex=15&queue=competitive")
	hist := edge("MATCH HISTORY", pd, "/match-history/v1/history/"+t.puuid+"?startIndex=0&endIndex=10")
	edge("LOADOUT", pd, "/personalization/v2/players/"+t.puuid+"/playerloadout")

	// Local party.
	pbody, pcode := rawLocal(port, password, "/parties/v1/players/"+t.puuid)
	dump("PARTY player (local)", pbody, pcode)

	// Pregame roster (agent select).
	pg, pgCode := t.rawEdge(glz, "/pregame/v1/players/"+t.puuid)
	dump("PREGAME player", pg, pgCode)
	if id := matchIDField(pg); id != "" {
		edge("PREGAME MATCH (rosters + agents)", glz, "/pregame/v1/matches/"+id)
	}

	// Coregame roster (in-match).
	cg, cgCode := t.rawEdge(glz, "/core-game/v1/players/"+t.puuid)
	dump("COREGAME player", cg, cgCode)
	if id := matchIDField(cg); id != "" {
		edge("COREGAME MATCH (rosters + agents)", glz, "/core-game/v1/matches/"+id)
	}

	// One full match detail (heavy - capped) for KDA/ACS/HS structure.
	if id := firstMatchID(hist); id != "" {
		edge("MATCH DETAILS (last game)", pd, "/match-details/v1/matches/"+id)
	}

	return b.String()
}

func agentName(id string) string {
	if n, ok := agents[strings.ToLower(id)]; ok {
		return n
	}
	return ""
}

// RosterPlayer is one player in the agent-select / in-match lobby.
type RosterPlayer struct {
	Name      string `json:"name"`
	Agent     string `json:"agent"`
	AgentUUID string `json:"agentUuid"`
	Ally      bool   `json:"ally"`
	Level     int    `json:"level"`
	Self      bool   `json:"self"`
}

// namesOf resolves puuids → "Name#Tag" via the name service (best-effort).
func (t *tokens) namesOf(puuids []string) map[string]string {
	out := map[string]string{}
	if len(puuids) == 0 {
		return out
	}
	body, _ := json.Marshal(puuids)
	req, err := http.NewRequest(http.MethodPut,
		"https://pd."+t.shard+".a.pvp.net/name-service/v2/players", bytes.NewReader(body))
	if err != nil {
		return out
	}
	req.Header.Set("Authorization", "Bearer "+t.access)
	req.Header.Set("X-Riot-Entitlements-JWT", t.entitlement)
	req.Header.Set("X-Riot-ClientPlatform", clientPlatform)
	if t.version != "" {
		req.Header.Set("X-Riot-ClientVersion", t.version)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	var arr []struct {
		Subject  string
		GameName string
		TagLine  string
	}
	if json.NewDecoder(resp.Body).Decode(&arr) == nil {
		for _, n := range arr {
			nm := n.GameName
			if n.TagLine != "" {
				nm += "#" + n.TagLine
			}
			out[strings.ToLower(n.Subject)] = nm
		}
	}
	return out
}

type glzPlayer struct {
	Subject        string
	TeamID         string
	CharacterID    string
	PlayerIdentity struct {
		AccountLevel int
		Incognito    bool
	}
}

func (t *tokens) finishRoster(players []glzPlayer) []RosterPlayer {
	var selfTeam string
	puuids := make([]string, 0, len(players))
	for _, p := range players {
		puuids = append(puuids, p.Subject)
		if strings.EqualFold(p.Subject, t.puuid) {
			selfTeam = p.TeamID
		}
	}
	names := t.namesOf(puuids)
	out := make([]RosterPlayer, 0, len(players))
	for _, p := range players {
		self := strings.EqualFold(p.Subject, t.puuid)
		nm := names[strings.ToLower(p.Subject)]
		if p.PlayerIdentity.Incognito && !self {
			nm = "" // respect hidden names
		}
		out = append(out, RosterPlayer{
			Name:      nm,
			Agent:     agentName(p.CharacterID),
			AgentUUID: p.CharacterID,
			Ally:      selfTeam == "" || p.TeamID == selfTeam,
			Level:     p.PlayerIdentity.AccountLevel,
			Self:      self,
		})
	}
	return out
}

// roster returns everyone in the current agent select / match with their agent.
func (t *tokens) roster() []RosterPlayer {
	// In-match (both teams, all agents).
	var cp struct{ MatchID string }
	if code, _ := t.glzGet("/core-game/v1/players/"+t.puuid, &cp); code == http.StatusOK && cp.MatchID != "" {
		var m struct{ Players []glzPlayer }
		if code, _ := t.glzGet("/core-game/v1/matches/"+cp.MatchID, &m); code == http.StatusOK {
			return t.finishRoster(m.Players)
		}
	}
	// Agent select (own team only; enemies are hidden until the match).
	var pp struct{ MatchID string }
	if code, _ := t.glzGet("/pregame/v1/players/"+t.puuid, &pp); code == http.StatusOK && pp.MatchID != "" {
		var m struct {
			AllyTeam struct{ Players []glzPlayer }
		}
		if code, _ := t.glzGet("/pregame/v1/matches/"+pp.MatchID, &m); code == http.StatusOK {
			return t.finishRoster(m.AllyTeam.Players)
		}
	}
	return nil
}

// DumpAgent runs the auth + GLZ flow and prints what it finds, for debugging
// `-valagent`. Run it during agent select or in a match.
func DumpAgent() string {
	port, password, err := lockCreds()
	if err != nil {
		return "lockfile: " + err.Error()
	}
	t, aerr := auth(port, password)
	if aerr != nil {
		return "auth error: " + aerr.Error()
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("puuid=%s\nregion=%s shard=%s version=%s\n",
		t.puuid, t.region, t.shard, t.version))

	var pg map[string]any
	c1, e1 := t.glzGet("/pregame/v1/players/"+t.puuid, &pg)
	b.WriteString(fmt.Sprintf("pregame players: HTTP %d err=%v -> %v\n", c1, e1, pg["MatchID"]))
	var cg map[string]any
	c2, e2 := t.glzGet("/core-game/v1/players/"+t.puuid, &cg)
	b.WriteString(fmt.Sprintf("coregame players: HTTP %d err=%v -> %v\n", c2, e2, cg["MatchID"]))
	id := t.agentID()
	b.WriteString("agentNow=" + agentName(id) + "  (" + id + ")\n")
	return b.String()
}

// agents maps CharacterID (lowercased) → display name.
var agents = map[string]string{
	"41fb69c1-4189-7b37-f117-bcaf1e96f1bf": "Astra",
	"5f8d3a7f-467b-97f3-062c-13acf203c006": "Breach",
	"9f0d8ba9-4140-b941-57d3-a7ad57c6b417": "Brimstone",
	"22697a3d-45bf-8dd7-4fec-84a9e28c69d7": "Chamber",
	"1dbf2edd-4729-0984-3115-daa5eed44993": "Clove",
	"117ed9e3-49f3-6512-3ccf-0cada7e3823b": "Cypher",
	"cc8b64c8-4b25-4ff9-6e7f-37b4da43d235": "Deadlock",
	"dade69b4-4f5a-8528-247b-219e5a1facd6": "Fade",
	"e370fa57-4757-3604-3648-499e1f642d3f": "Gekko",
	"95b78ed7-4637-86d9-7e41-71ba8c293152": "Harbor",
	"0e38b510-41a8-5780-5e8f-568b2a4f2d6c": "Iso",
	"add6443a-41bd-e414-f6ad-e58d267f4e95": "Jett",
	"601dbbe7-43ce-be57-2a40-4abd24953621": "KAY/O",
	"1e58de9c-4950-5125-93e9-a0aee9f98746": "Killjoy",
	"bb2a4828-46eb-8cd1-e765-15848195d751": "Neon",
	"8e253930-4c05-31dd-1b6c-968525494517": "Omen",
	"eb93336a-449b-9c1b-0a54-a891f7921d69": "Phoenix",
	"f94c3b30-42be-e959-889c-5aa313dba261": "Raze",
	"a3bfb853-43b2-7238-a4f1-ad90e9e46bcc": "Reyna",
	"569fdd95-4d10-43ab-ca70-79becc718b46": "Sage",
	"6f2a04ca-43e0-be17-7f36-b3908627744d": "Skye",
	"320b2a48-4d9b-a075-30f1-1f93a9b638fa": "Sova",
	"b444168c-4e35-8076-db47-ef9bf368f384": "Tejo",
	"707eab51-4836-f488-046a-cda6bf494859": "Viper",
	"efba5359-4016-a1e5-7626-b1ae76895940": "Vyse",
	"df1cb487-4902-002e-5c17-d28e83e78588": "Waylay",
	"7f94d92c-4234-0a36-9646-3a87eb8b5c89": "Yoru",
}
