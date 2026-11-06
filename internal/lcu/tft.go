package lcu

import "strings"

const (
	tftMode        = "TFT"
	queueHyperRoll = 1130
	queueDoubleUp  = 1160
	assetsBase     = "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/assets/"
)

// TFTInfo is what the League client knows about a Teamfight Tactics game that
// the Live Client API doesn't: the queue, your Little Legend and your rank in
// that queue. Every field is best-effort.
type TFTInfo struct {
	Queue         string
	Companion     string
	CompanionIcon string
	Tier          string
	Division      string
	LP            int
}

// FetchTFT reads the client during a TFT game.
func FetchTFT(leaguePath string) (*TFTInfo, error) {
	c, err := dial(leaguePath)
	if err != nil {
		return nil, err
	}
	info := &TFTInfo{}

	var session struct {
		GameData struct {
			Queue struct {
				ID int `json:"id"`
			} `json:"queue"`
		} `json:"gameData"`
	}
	queue := 0
	if c.get("/lol-gameflow/v1/session", &session) == nil {
		queue = session.GameData.Queue.ID
		info.Queue = c.queueLabel(queue)
	}

	var companions struct {
		Selected struct {
			Name string `json:"name"`
			Icon string `json:"loadoutsIcon"`
		} `json:"selectedLoadoutItem"`
	}
	if c.get("/lol-cosmetics/v1/inventories/tft/companions", &companions) == nil {
		info.Companion = companions.Selected.Name
		info.CompanionIcon = companionIconURL(companions.Selected.Icon)
	}

	var ranked struct {
		QueueMap map[string]struct {
			Tier     string `json:"tier"`
			Division string `json:"division"`
			LP       int    `json:"leaguePoints"`
		} `json:"queueMap"`
	}
	if key := tftRankKey(queue); key != "" && c.get("/lol-ranked/v1/current-ranked-stats", &ranked) == nil {
		e := ranked.QueueMap[key]
		info.Tier, info.Division, info.LP = cleanTier(e.Tier), cleanDiv(e.Division), e.LP
	}
	return info, nil
}

// companionIconURL turns the client's icon path
// ("/lol-game-data/assets/ASSETS/Loadouts/Companions/X.png") into its Community
// Dragon address, where every path is lower case.
func companionIconURL(path string) string {
	low := strings.ToLower(path)
	i := strings.LastIndex(low, "assets/")
	if i < 0 {
		return ""
	}
	return assetsBase + low[i+len("assets/"):]
}

// tftRankKey is the ranked entry a TFT queue counts toward. Double Up has its
// own ladder; Hyper Roll has rated tiers, not a rank, so it shows none.
func tftRankKey(queue int) string {
	switch queue {
	case queueHyperRoll:
		return ""
	case queueDoubleUp:
		return "RANKED_TFT_DOUBLE_UP"
	}
	return "RANKED_TFT"
}
