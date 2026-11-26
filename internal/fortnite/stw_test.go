package fortnite

import (
	"strings"
	"testing"
)

const stwLog = `[2026.11.25-18.15.03:056][ 25]LogLoad: LoadMap: 18.157.35.184:9023/STW_Zones/Maps/Zones/Zone_Temperate_Suburban?EncryptionToken=x?game=/Game/Blueprints/FortGameModePvE_BP.FortGameModePvE_BP_C
[2026.11.25-18.22.10:074][772]LogFort: [CollectionMultiProgressBar_C][CollectionMultiProgressBar] | FORT-888269 | CollectionBar_C_2147482646 Created.
    > BarBounds = (0, 25,000)
    > AchievedCount = 25,892
    > BarColor = (R=0.780000,G=0.336482,B=0.156000,A=1.000000)
    > BageItemDef = CombatScore_MissionBadge_Bronze
[2026.11.25-18.22.10:074][772]LogFort: [CollectionMultiProgressBar_C][CollectionMultiProgressBar] | FORT-888269 | CollectionBar_C_2147482645 Created.
    > BarBounds = (25,000, 77,500)
    > AchievedCount = 25,892
    > BageItemDef = CombatScore_MissionBadge_Silver
[2026.11.25-18.27.30:050][674]LogFort: Warning: AFortPlayerControllerZone::ClientOnZoneEndScoreReports_Implementation - Start Server Authoritative Reports
[2026.11.25-18.27.30:050][674]LogFort: Warning: AFortPlayerControllerZone::ClientOnZoneEndScoreReports_Implementation - Start Report for Player [Melocet] [MCP:5cea7bb9ccf2494a9633a45241e145f2]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:CombatScore [267789]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:BuildingScore [41184]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:UtilityScore [43376]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:BadgesScore [2660000]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:TotalScore [352349]
[2026.11.25-18.27.30:050][674]LogFort: Warning: AFortPlayerControllerZone::ClientOnZoneEndScoreReports_Implementation - Start Report for Player [ioi-telpa] [MCP:d3e3e...14961]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:TotalScore [134073]
[2026.11.25-18.27.30:050][674]LogFort: Warning: AFortPlayerControllerZone::ClientOnZoneEndScoreReports_Implementation - End Server Authoritative Reports
[2026.11.25-18.27.30:050][674]LogFort: Warning: AFortPlayerControllerZone::ClientOnZoneEndScoreReports_Implementation - Start Client Cached Reports
[2026.11.25-18.27.30:050][674]LogFort: Warning: AFortPlayerControllerZone::ClientOnZoneEndScoreReports_Implementation - Start Report for Player [Melocet] [MCP:5cea7bb9ccf2494a9633a45241e145f2]
[2026.11.25-18.27.30:050][674]LogFort: Warning: ZoneScore:TotalScore [1]
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Start LogXPData
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Player Name: Melocet
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Initial Level: 310
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Initial Display XP: 4229143
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta Level: 311
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta XP: 4402820
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta Base XP: 86839
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta Bonus XP: 0
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta Boost Missed XP: 26051
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta Rest XP: 86838
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Player Name: ioi-telpa
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Initial Level: 42
[2026.11.25-18.27.33:273][390]LogInGameRewardsSTW: Delta Base XP: 317361
[2026.11.25-18.27.51:455][909]LogLoad: LoadMap: 18.156.203.65:9104/STW_HestiaBeauty/World/Zone/HestiaBeauty_Zone?EncryptionToken=y
`

func TestStwMissionMedalsAndResult(t *testing.T) {
	s := State{} // the app started mid-log: who I am is learned later
	lines := strings.Split(stwLog, "\n")
	// Up to the end of the medals: in the mission.
	s.feed(strings.NewReader(strings.Join(lines[:10], "\n")))
	if s.Stw.Map != "Suburban" || s.Stw.Mission != "" {
		t.Fatalf("map: %+v", s.Stw)
	}
	if m := s.Stw.Medals[0]; m.Score != 25892 || m.Tier != 1 || m.Next != 77500 {
		t.Fatalf("combat medal: %+v", m)
	}
	s.feed(strings.NewReader(strings.Join(lines[10:], "\n")))
	if s.StwLast == nil {
		t.Fatal("no result")
	}
	l, xp := s.StwLast.Mine("5cea7bb9ccf2494a9633a45241e145f2", "")
	if len(l.Players) != 2 || l.Map != "Suburban" {
		t.Fatalf("result: %+v", l)
	}
	if p := l.Players[0]; !p.Me || p.Combat != 267789 || p.Building != 41184 || p.Utility != 43376 || p.Total != 352349 {
		t.Fatalf("my report: %+v", p)
	}
	if p := l.Players[1]; p.Me || p.Name != "ioi-telpa" || p.Total != 134073 {
		t.Fatalf("their report (the cached copy must not count): %+v", p)
	}
	if xp.XP != 86839+86838 || xp.Level != 311 || xp.LevelUp != 1 {
		t.Fatalf("xp: %+v", xp)
	}
	if _, x := s.StwLast.Mine("", "ioi-telpa"); x.XP != 317361 {
		t.Fatalf("their xp by name: %+v", x)
	}
	// Back at Homebase: a fresh run, the result stays.
	if s.Stw.Map != "" || s.Stw.Medals[0].Score != 0 || s.StwLast == nil {
		t.Fatalf("homebase: %+v", s.Stw)
	}
}

func TestStwMapAndWaves(t *testing.T) {
	if terr, mission := stwMap("/STW_Zones/Maps/Zones/Outpost/Zone_Outpost_Plankerton_AD"); terr != "" || mission != "Storm Shield Defense" {
		t.Fatalf("outpost: %q %q", terr, mission)
	}
	if terr, _ := stwMap("/STW_HestiaBeauty/World/Zone/HestiaBeauty_Zone"); terr != "" {
		t.Fatalf("homebase: %q", terr)
	}
	if terr, _ := stwMap("/Game/Athena/Maps/Athena_Terrain"); terr != "" {
		t.Fatalf("battle royale: %q", terr)
	}
	s := State{}
	s.feed(strings.NewReader("[2026.11.05-10.00.00:000][  1]LogLoad: LoadMap: 1.2.3.4:9000/STW_Zones/Maps/Zones/Outpost/Zone_Outpost_Plankerton_AD?x\n" +
		"[2026.11.05-10.05.00:000][  1]LogHealthSnapshot: ======= Snapshot: Start of Wave 2 (FortGameStatePvE, Difficulty 22.00) ======="))
	if s.Stw.Wave != 2 || s.Stw.Mission != "Storm Shield Defense" {
		t.Fatalf("wave: %+v", s.Stw)
	}
}
