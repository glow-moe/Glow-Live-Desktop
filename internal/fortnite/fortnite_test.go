package fortnite

import (
	"strings"
	"testing"
	"time"
)

// Lines as FortniteGame.log writes them (server address and tokens replaced).
const match = `[2026.10.30-19.13.26:839][515]MatchmakingLog: [2b0a] Link id changing from [Mnemonic=[] Version=[latest]] to [Mnemonic=[experience_og] Version=[latest]]
[2026.10.30-19.15.20:000][100]LogEOSSDK: LogEOSPresence: Updating Presence to [Online]. LocalUserId=[5ce...5f2] Template=[] RichText=[Fortnite OG]
[2026.10.30-19.15.36:617][509]LogLoad: LoadMap: 10.0.0.9:9034/Figment_S10_Map/Athena_Terrain_S10?EncryptionToken=x:y?VerseURI=/Fortnite.com/GameFeatures/FigmentRoot
[2026.10.30-19.15.49:070][616]LogBattleRoyaleGamePhaseLogic: HandleGamePhaseChanged. OldPhase = EAthenaGamePhase::Setup. NewPhase = EAthenaGamePhase::Warmup
[2026.10.30-19.15.56:162][201]LogFort: PLAYLIST: Preparing to apply Data Tables and Curves in AFortGameModeAthena::ApplyPlaylistDataTables(), PlaylistName is Playlist_FigmentNoBuildSolo (Server Only)
[2026.10.30-19.16.03:269][626]LogEOSSDK: LogEOSPresence: Updating Presence to [Online]. LocalUserId=[5ce...5f2] Template=[] RichText=[Fortnite OG Zero  Solo - 98 Left]
[2026.10.30-19.16.40:011][574]LogBattleRoyaleGamePhaseLogic: HandleGamePhaseChanged. OldPhase = EAthenaGamePhase::Warmup. NewPhase = EAthenaGamePhase::Aircraft
[2026.10.30-19.17.40:000][574]LogEOSSDK: LogEOSPresence: Updating Presence to [Online]. LocalUserId=[5ce...5f2] Template=[] RichText=[Fortnite OG Zero  Solo - 73 Left]
`

func TestMatchThenLobby(t *testing.T) {
	var s State
	s.feed(strings.NewReader(match))
	if !s.InMatch || s.Map != "Athena_Terrain_S10" || s.Playlist != "Playlist_FigmentNoBuildSolo" || s.Phase != "Aircraft" || s.Island != "experience_og" {
		t.Fatalf("in match: %+v", s)
	}
	if s.Status != "Fortnite OG Zero Solo - 73 Left" {
		t.Fatalf("status %q", s.Status)
	}
	if want := time.Date(2026, 10, 30, 19, 15, 36, 617e6, time.UTC); !s.JoinedAt.Equal(want) {
		t.Fatalf("joined %v", s.JoinedAt)
	}
	s.feed(strings.NewReader("[2026.10.30-19.18.55:799][267]LogLoad: LoadMap: /Game/Maps/Frontend?closed\n"))
	if s.InMatch || s.Phase != "" || s.Last == nil {
		t.Fatalf("back in lobby: %+v", s)
	}
	if s.Last.Left != 73 || s.Last.Playlist != "Playlist_FigmentNoBuildSolo" || s.Last.EndedAt.Sub(s.Last.JoinedAt) != 3*time.Minute+19*time.Second+182*time.Millisecond {
		t.Fatalf("last: %+v", s.Last)
	}
}

func TestStartupLobbyIsNotAMatch(t *testing.T) {
	var s State
	s.feed(strings.NewReader("[2026.10.30-19.13.06:490][216]LogLoad: LoadMap: /Game/Maps/Frontend?Name=Player\n[2026.10.30-19.16.50:630][792]MatchmakingLog: [3ed0] Melocet is now Eligible for matchmaking\n"))
	if s.InMatch || s.Last != nil || s.Name != "Melocet" {
		t.Fatalf("%+v", s)
	}
}

const loadout = `[2026.10.30-19.13.34:242][665]LogFortPlayerLoadout: [FORT-1129229] Local Player PlayerPawn_Athena_C_2147482646 loadout changed (bHasChanged=1, bCosmeticLoadoutSet=1)
--- [LoadoutSlot.Contrails]=AthenaSkyDiveContrail:trails_id_009_hearts

--- [LoadoutSlot.Glider]=AthenaGlider:glider_id_067_psburnout

--- [LoadoutSlot.Pickaxe]=AthenaPickaxe:halloweenscythe

--- [LoadoutSlot.Character]=AthenaCharacter:character_hammervice
		{Channel: Outfit - Variant: Stage1}

--- [LoadoutSlot.Backpack]=AthenaBackpack:bid_847_punkkoi
--- [LoadoutSlot.ExtraPart]=empty/null (desired item = none)

--- [LoadoutSlot.Mimosa.Main]=CosmeticMimosa:companion_carriershade
		{Channel: CustomName - Variant: Text - Data: Empress}
		{Channel: Outfit - Variant: Stage3}
--- [LoadoutSlot.LoadingScreen]=empty/null (desired item = none)
[2026.10.30-19.13.34:300][665]LogFort: something else
--- [LoadoutSlot.Character]=AthenaCharacter:not_mine
`

func TestLoadout(t *testing.T) {
	var s State
	s.feed(strings.NewReader(loadout))
	want := Loadout{Character: "character_hammervice", Backpack: "bid_847_punkkoi", Pickaxe: "halloweenscythe", Glider: "glider_id_067_psburnout", Contrail: "trails_id_009_hearts", Pet: "companion_carriershade", PetName: "Empress"}
	if s.Loadout != want {
		t.Fatalf("loadout %+v", s.Loadout)
	}
}

func TestParty(t *testing.T) {
	var s State
	s.feed(strings.NewReader(`[2026.10.30-19.13.15:020][233]LogParty: Verbose: Adding [Melocet] Id [MCP:5cea7bb9ccf2494a9633a45241e145f2] as a new member to [MCP:5cea7bb9ccf2494a9633a45241e145f2]'s party via an existing UITeamMember in [Melocet]'s game.
[2026.10.30-19.13.19:289][353]LogParty: Verbose: Removing [Melocet] Id [MCP:5cea7bb9ccf2494a9633a45241e145f2] from [Melocet]'s team.
[2026.10.30-19.13.19:289][353]LogParty: Verbose: Adding [Melocet] Id [MCP:5cea7bb9ccf2494a9633a45241e145f2] as a new member to [MCP:5cea7bb9ccf2494a9633a45241e145f2]'s party via an existing UITeamMember in [Melocet]'s game.
[2026.10.30-19.16.50:630][792]MatchmakingLog: [3ed0] Melocet is now Eligible for matchmaking
[2026.10.30-20.54.25:519][531]LogParty: Verbose: Adding [megecet42] Id [MCP:7dbbe...f57b3] as a new member to [MCP:5cea7bb9ccf2494a9633a45241e145f2]'s party via an existing UITeamMember in [Melocet]'s game.
[2026.10.30-20.54.25:600][531]LogParty: Verbose: Adding [friend two] Id [MCP:1a2b...9f9f9] as a new member to [MCP:5cea7bb9ccf2494a9633a45241e145f2]'s party via an existing UITeamMember in [Melocet]'s game.
[2026.10.30-20.59.00:000][531]LogParty: Verbose: Removing [friend two] Id [MCP:1a2b...9f9f9] from [Melocet]'s team.
`))
	if got := s.Others(); len(got) != 1 || got[0].Name != "megecet42" || got[0].ID != "" {
		t.Fatalf("others %v (party %v)", got, s.Party)
	}
	s.feed(strings.NewReader(`[2026.10.30-20.54.25:608][542]LogNativeEpicPartyEOS: [member_state_updated_broadcast] LocalAccountId=[5cea7bb9ccf2494a9633a45241e145f2], TargetAccountId=[7dbbe96e0ce245118e3315caf63f57b3], PartyId=[f097]` + "\n"))
	if got := s.Others(); len(got) != 1 || got[0].ID != "7dbbe96e0ce245118e3315caf63f57b3" || s.ID != "5cea7bb9ccf2494a9633a45241e145f2" {
		t.Fatalf("ids: others %v, me %q", got, s.ID)
	}
}
