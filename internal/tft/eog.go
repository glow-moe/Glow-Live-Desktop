package tft

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/glow-moe/glow-collector/internal/jsonx"
)

// EndOfGame is TFT's own end of game file (TFTEoGStats.json, next to the
// log), written when a game ends: every player's place and final board with
// items. The log only has what you bought; this is what you ended with.
type EndOfGame struct {
	GameID  int64
	Seconds int
	Place   int     // yours, 1 to 8
	Board   []Piece // yours
	Players []Player
}

// Piece is a champion on a final board.
type Piece struct {
	Champ string   // "Tristana", "Lux"
	Stars int      //
	Items []string // "InfinityEdge", "Artifact_LichBane", "18_EmblemElderwood"
}

// Player is one of the eight, by place.
type Player struct {
	Name, Tag string
	Place     int
	Me        bool
	Board     []Piece
}

func eogPath() string {
	if p := logPath(); p != "" {
		return filepath.Join(filepath.Dir(p), "TFTEoGStats.json")
	}
	return ""
}

type eogFile struct {
	GameID     int64 `json:"gameId"`
	StatsBlock struct {
		GameLengthSeconds int `json:"gameLengthSeconds"`
		Players           []struct {
			PUUID          string `json:"PUUID"`
			IsInteractable bool   `json:"isInteractable"`
			FfaStanding    int    `json:"ffaStanding"`
			GameName       string `json:"riotIdGameName"`
			TagLine        string `json:"riotIdTagLine"`
			BoardPieces    []struct {
				ChampionName string   `json:"championName"`
				StarLevel    int      `json:"starLevel"`
				ItemNames    []string `json:"itemNames"`
			} `json:"boardPieces"`
		} `json:"players"`
	} `json:"statsBlock"`
}

// parseEndOfGame reads the file's JSON; you are the player with your Riot ID,
// or the one the file marks as yours (the only one with a PUUID).
func parseEndOfGame(b []byte, name, tag string) (EndOfGame, bool) {
	var f eogFile
	if jsonx.Unmarshal(b, &f) != nil || len(f.StatsBlock.Players) == 0 {
		return EndOfGame{}, false
	}
	out := EndOfGame{GameID: f.GameID, Seconds: f.StatsBlock.GameLengthSeconds}
	mine := -1
	for i, p := range f.StatsBlock.Players {
		if name != "" && strings.EqualFold(p.GameName, name) && strings.EqualFold(p.TagLine, tag) {
			mine = i
			break
		}
	}
	if mine < 0 {
		for i, p := range f.StatsBlock.Players {
			if p.IsInteractable || (p.PUUID != "" && strings.Trim(p.PUUID, "0-") != "") {
				mine = i
				break
			}
		}
	}
	for i, p := range f.StatsBlock.Players {
		pl := Player{Name: p.GameName, Tag: p.TagLine, Place: p.FfaStanding, Me: i == mine}
		for _, bp := range p.BoardPieces {
			champ := eogChamp(bp.ChampionName)
			if champ == "" {
				continue
			}
			pc := Piece{Champ: champ, Stars: max(bp.StarLevel, 1)}
			for _, it := range bp.ItemNames {
				if it = strings.TrimPrefix(it, "DA_"); it != "" && len(pc.Items) < 3 {
					pc.Items = append(pc.Items, it)
				}
			}
			pl.Board = append(pl.Board, pc)
		}
		if pl.Me {
			out.Place, out.Board = pl.Place, pl.Board
		}
		out.Players = append(out.Players, pl)
	}
	if mine < 0 {
		return EndOfGame{}, false
	}
	return out, true
}

var (
	eogSetFirst = regexp.MustCompile(`^DA_\d{1,2}_([A-Z][A-Za-z0-9]{1,30})`)     // DA_18_Tristana, DA_18_Akali_AD
	eogSetLast  = regexp.MustCompile(`^DA_([A-Z][A-Za-z]{1,30}?)\d{1,2}(?:_|$)`) // DA_Lux18_Base, DA_Amumu18
)

// eogChamp is the champion in a board piece's name, as the log names it.
func eogChamp(s string) string {
	if m := eogSetFirst.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := eogSetLast.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

var (
	eogMod  time.Time
	eogLast EndOfGame
	eogOK   bool
)

// EndKeep is how long after a game its end of game file still shows.
const EndKeep = 30 * time.Minute

// endOfGame is the file's game when it was written after `since` (the game's
// start, when known) and in the last EndKeep. Read again only when it
// changes. Callers hold mu.
func endOfGame(since time.Time, name, tag string) (EndOfGame, bool) {
	p := eogPath()
	if p == "" {
		return EndOfGame{}, false
	}
	fi, err := os.Stat(p)
	if err != nil {
		return EndOfGame{}, false
	}
	if recent := time.Now().Add(-EndKeep); since.Before(recent) {
		since = recent
	}
	if fi.ModTime().Before(since) {
		return EndOfGame{}, false
	}
	if !fi.ModTime().Equal(eogMod) {
		eogMod = fi.ModTime()
		eogOK = false
		if b, err := os.ReadFile(p); err == nil {
			eogLast, eogOK = parseEndOfGame(b, name, tag)
		}
	}
	return eogLast, eogOK
}
