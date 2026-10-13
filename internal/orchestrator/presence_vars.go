package orchestrator

import (
	"strconv"

	"github.com/glow-moe/glow-collector/internal/ddragon"
	"github.com/glow-moe/glow-collector/internal/forza"
	"github.com/glow-moe/glow-collector/internal/roblox"
	"github.com/glow-moe/glow-collector/internal/snapshot"
	"github.com/glow-moe/glow-collector/internal/steam"
)

// The {vars} each Discord status kind offers the site's templates
// (presence_tpl.go). Flags are "yes" or empty.

func itoa(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func forzaVars(s *forza.Snapshot, gameID string) map[string]string {
	return map[string]string{
		"title": forzaTitle(gameID), "game": gameID, "car": s.Car.Name,
		"speed": strconv.Itoa(s.Speed), "gear": s.Gear, "racing": yes(s.Racing),
	}
}

func leagueVars(s snapshot.Snapshot) map[string]string {
	me := s.Me
	v := map[string]string{
		"champ": me.ChampName, "champKey": me.ChampKey, "skin": me.SkinName,
		"level": itoa(me.Level), "gold": strconv.Itoa(me.Gold), "clock": s.Clock, "map": s.MapName,
	}
	if me.ChampKey != "" {
		v["tile"] = ddragon.TileURL(me.ChampKey, me.Skin)
	}
	for _, p := range append(append([]snapshot.Player{}, s.Blue...), s.Red...) {
		if p.IsMe {
			v["kda"], v["cs"] = p.Kda, strconv.Itoa(p.Cs)
			break
		}
	}
	return v
}

func lobbyVars(l *snapshot.Lobby) map[string]string {
	v := map[string]string{
		"phase": l.Phase, "label": l.Label, "queue": l.QueueName, "mode": l.ModeLabel, "map": l.MapName,
		"champ": l.ChampName, "champKey": l.ChampKey, "level": itoa(l.Level), "summoner": l.Summoner,
		"icon": l.IconURL, "ranked": yes(l.IsRanked), "phaseLabel": l.PhaseLabel,
		"party": itoa(len(l.Members)), "size": itoa(l.LobbySize),
	}
	if l.ChampKey != "" {
		v["tile"] = ddragon.TileURL(l.ChampKey, 0)
	}
	return v
}

func steamVars(s steam.Snap, icon string, named bool) map[string]string {
	return map[string]string{
		"game": s.Game, "detail": s.Detail, "appId": itoa(s.AppID), "icon": icon,
		"cover": steam.CoverImage(s.AppID), "named": yes(named),
	}
}

func robloxVars(g roblox.Game, named bool) map[string]string {
	v := map[string]string{"game": g.Name, "creator": g.Creator, "icon": g.Icon, "named": yes(named)}
	if g.PlaceID > 0 {
		v["placeId"] = strconv.FormatInt(g.PlaceID, 10)
	}
	return v
}

func splitVars(s splitSnap) map[string]string {
	return map[string]string{
		"server": s.Server.Name, "place": splitcraftState(s), "device": splitcraftDevice(s),
		"online": splitcraftOnline(s), "name": s.Name, "skin": splitcraftSkin(s), "logo": splitcraftLogo(s),
		"afk": yes(s.AFK), "dedicated": yes(appSplitcraft != ""),
	}
}

func animeVars(a animeSnap) map[string]string {
	return map[string]string{"title": a.Title, "episode": itoa(a.Episode), "poster": a.Poster, "paused": yes(a.Paused)}
}

func readingVars(r mangaSnap) map[string]string {
	ch := ""
	if r.Chapter > 0 {
		ch = strconv.FormatFloat(r.Chapter, 'f', -1, 64)
	}
	return map[string]string{"title": r.Title, "chapter": ch, "poster": r.Poster, "paused": yes(r.Paused)}
}

func musicVars(m musicSnap) map[string]string {
	return map[string]string{
		"title": m.Title, "artist": m.Artist, "album": m.Album, "artwork": m.Artwork,
		"service": m.Service, "url": m.URL, "paused": yes(m.Paused),
	}
}
