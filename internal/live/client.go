// Package live reads Riot's Live Client Data API, served over HTTPS with a
// self-signed Riot cert on 127.0.0.1:2999 only while a game is running.
package live

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

const endpoint = "https://127.0.0.1:2999/liveclientdata/allgamedata"

// ErrNoGame means the local API isn't answering (no game in progress).
var ErrNoGame = errors.New("no live game")

// client trusts the Riot self-signed cert (localhost only) and fails fast.
var client = &http.Client{
	Timeout: 3 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 - localhost Riot cert
	},
}

// AllGameData is the subset of /allgamedata the HUD needs.
type AllGameData struct {
	ActivePlayer ActivePlayer `json:"activePlayer"`
	AllPlayers   []Player     `json:"allPlayers"`
	Events       struct {
		Events []Event `json:"Events"`
	} `json:"events"`
	GameData GameData `json:"gameData"`
}

type ActivePlayer struct {
	Abilities     Abilities     `json:"abilities"`
	ChampionStats ChampionStats `json:"championStats"`
	CurrentGold   float64       `json:"currentGold"`
	FullRunes     FullRunes     `json:"fullRunes"`
	Level         int           `json:"level"`
	SummonerName  string        `json:"summonerName"`
	RiotID        string        `json:"riotId"`
	RiotIDName    string        `json:"riotIdGameName"`
}

type Ability struct {
	AbilityLevel int `json:"abilityLevel"`
}

type Abilities struct {
	Q Ability `json:"Q"`
	W Ability `json:"W"`
	E Ability `json:"E"`
	R Ability `json:"R"`
}

type ChampionStats struct {
	AbilityPower  float64 `json:"abilityPower"`
	Armor         float64 `json:"armor"`
	AttackDamage  float64 `json:"attackDamage"`
	AttackSpeed   float64 `json:"attackSpeed"`
	CritChance    float64 `json:"critChance"`
	AbilityHaste  float64 `json:"abilityHaste"`
	CurrentHealth float64 `json:"currentHealth"`
	MaxHealth     float64 `json:"maxHealth"`
	MagicResist   float64 `json:"magicResist"`
	MoveSpeed     float64 `json:"moveSpeed"`
	ResourceValue float64 `json:"resourceValue"`
	ResourceMax   float64 `json:"resourceMax"`
}

type FullRunes struct {
	Keystone          Rune   `json:"keystone"`
	PrimaryRuneTree   Rune   `json:"primaryRuneTree"`
	SecondaryRuneTree Rune   `json:"secondaryRuneTree"`
	StatRunes         []Rune `json:"statRunes"`
	// The full page in slot order: keystone, 3 primary minors, 2 secondary.
	GeneralRunes []Rune `json:"generalRunes"`
}

type Rune struct {
	DisplayName    string `json:"displayName"`
	RawDescription string `json:"rawDescription"`
	ID             int    `json:"id"`
}

type Player struct {
	ChampionName    string        `json:"championName"`
	RawChampionName string        `json:"rawChampionName"`
	IsBot           bool          `json:"isBot"`
	IsDead          bool          `json:"isDead"`
	RespawnTimer    float64       `json:"respawnTimer"`
	Items           []Item        `json:"items"`
	Level           int           `json:"level"`
	RiotID          string        `json:"riotId"`
	RiotIDName      string        `json:"riotIdGameName"`
	Scores          Scores        `json:"scores"`
	SkinID          int           `json:"skinID"`
	SkinName        string        `json:"skinName"`
	SummonerName    string        `json:"summonerName"`
	SummonerSpells  SummonerSpell `json:"summonerSpells"`
	Team            string        `json:"team"`
}

type Item struct {
	ItemID int `json:"itemID"`
	Slot   int `json:"slot"`
}

type Scores struct {
	Assists    int     `json:"assists"`
	CreepScore int     `json:"creepScore"`
	Deaths     int     `json:"deaths"`
	Kills      int     `json:"kills"`
	WardScore  float64 `json:"wardScore"`
}

type SummonerSpell struct {
	SummonerSpellOne struct {
		DisplayName string `json:"displayName"`
	} `json:"summonerSpellOne"`
	SummonerSpellTwo struct {
		DisplayName string `json:"displayName"`
	} `json:"summonerSpellTwo"`
}

type Event struct {
	EventID     int      `json:"EventID"`
	EventName   string   `json:"EventName"`
	EventTime   float64  `json:"EventTime"`
	KillerName  string   `json:"KillerName"`
	VictimName  string   `json:"VictimName"`
	Assisters   []string `json:"Assisters"`
	Recipient   string   `json:"Recipient"`
	DragonType  string   `json:"DragonType"`
	TurretKilled string  `json:"TurretKilled"`
	KillStreak  int      `json:"KillStreak"`
	Stolen      string   `json:"Stolen"`
}

type GameData struct {
	GameMode  string  `json:"gameMode"`
	GameTime  float64 `json:"gameTime"`
	MapName   string  `json:"mapName"`
	MapNumber int     `json:"mapNumber"`
}

// FetchRaw returns the raw /allgamedata bytes. Returns ErrNoGame when the client
// isn't in a game (connection refused / timeout).
func FetchRaw() ([]byte, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, ErrNoGame
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrNoGame
	}
	return io.ReadAll(resp.Body)
}

// Parse decodes raw /allgamedata bytes into the subset we use.
func Parse(b []byte) (*AllGameData, error) {
	var d AllGameData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Fetch reads and decodes /allgamedata once.
func Fetch() (*AllGameData, error) {
	b, err := FetchRaw()
	if err != nil {
		return nil, err
	}
	return Parse(b)
}
