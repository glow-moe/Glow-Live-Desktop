package tft

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Lines as TFT.log writes them (Set 18 tutorial game, 2026-11-08).
const game = `[2026.11.08-12.08.25:693][149]TFT: [Standalone] TFTLobbySubsystem_2147482177: Setting ActiveQueueId=[2210]; current=[0]
[2026.11.08-12.08.37:446][400]TFT: [Client] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::InGame]
[2026.11.08-12.09.01:000][604]TFT: [Client] TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.Purchase] Actor[TFTCharacter_2147450114 {DA_18_Varus}] Plays Event[A_VO_Varus18_Purchase_Cue] with Interrupt Channel [AC_VO_Round]
[2026.11.08-12.09.02:000][605]TFT: [Client] TFTAudioEventSubsystem: Actor[TFTCharacter_2147450073 {DA_18_Varus}] and Event[Audio.Event.VO.Unit.Purchase] was blocked by InterruptChannel[AC_VO_Round]
[2026.11.08-12.09.03:000][606]TFT: [Client] TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.StarUp.2Star] Actor[TFTCharacter_2147451513 {DA_18_Varus}] Plays Event[A_VO_Varus18_StarUp_Cue] with Interrupt Channel [AC_VO_Action]
[2026.11.08-12.09.04:000][607]TFT: [Client] TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.Purchase] Actor[TFTCharacter_2147448496 {DA_18_Xayah}] Plays Event[A_VO_Xayah18_Purchase_Cue] with Interrupt Channel [AC_VO_Round]
[2026.11.08-12.09.05:000][608]TFT: [Client] TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.Spell] Actor[TFTCharacter_2147444343 {DA_18_Rakan}] Plays Event[A_VO_Rakan18_Spell_Cue] with Interrupt Channel [AC_VO_Basic]
[2026.11.08-12.09.06:000][609]TFT: [Client] TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.Purchase] Actor[TFTCharacter_2147443831 {DA_18_Akali_AD}] Plays Event[A_VO_Akali18_Purchase_Cue] with Interrupt Channel [AC_VO_Round]
[2026.11.08-12.09.07:000][610]TFT: [Client] TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.StarUp.2Star] Actor[TFTCharacter_2147443831 {DA_18_Akali_AD}] Plays Event[A_VO_Akali18_StarUp_Cue] with Interrupt Channel [AC_VO_Action]
`

const ended = `[2026.11.08-12.12.27:454][110]TFT: [Standalone] TFTLobbySubsystem_2147482177: Exiting LobbyState[ETFTLobbyState::InGame]
[2026.11.08-12.12.27:454][110]TFT: [Standalone] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::EndOfGameInterstitial]
`

func TestGameFromTheLog(t *testing.T) {
	var s State
	s.feed(strings.NewReader(game))
	if s.Phase != PhaseInGame || s.QueueID != 2210 || s.Set != 18 {
		t.Fatalf("phase %q queue %d set %d", s.Phase, s.QueueID, s.Set)
	}
	if want := time.Date(2026, 11, 8, 12, 8, 37, 446e6, time.UTC); !s.StartedAt.Equal(want) {
		t.Fatalf("started %v", s.StartedAt)
	}
	// Rakan only cast a spell (an enemy can too): not bought.
	want := []Unit{{Champ: "Varus", Stars: 2}, {Champ: "Xayah", Stars: 1}, {Champ: "Akali", Stars: 2}}
	if len(s.Units) != len(want) {
		t.Fatalf("units %+v", s.Units)
	}
	for i := range want {
		if s.Units[i].Champ != want[i].Champ || s.Units[i].Stars != want[i].Stars {
			t.Fatalf("unit %d = %+v, want %+v", i, s.Units[i], want[i])
		}
	}
	// Akali was starred up last.
	if b := s.Best(2); b[0].Champ != "Akali" || b[1].Champ != "Varus" {
		t.Fatalf("best %+v", b)
	}
}

func TestEndKeepsTheBoardAndNextGameClearsIt(t *testing.T) {
	var s State
	s.feed(strings.NewReader(game + ended))
	if s.Phase != PhasePostGame || len(s.Units) != 3 || !s.StartedAt.IsZero() {
		t.Fatalf("after the game: %+v", s)
	}
	s.feed(strings.NewReader("[2026.11.08-12.20.00:000][1]TFT: [Client] TFTLobbySubsystem_1: Entering LobbyState[ETFTLobbyState::InGame]\n"))
	if s.Phase != PhaseInGame || len(s.Units) != 0 {
		t.Fatalf("next game: %+v", s)
	}
	s.feed(strings.NewReader("[2026.11.08-12.30.00:000][1]TFT: [Client] TFTLobbySubsystem_1: Entering LobbyState[ETFTLobbyState::Frontend]\n"))
	if s.Phase != PhaseMenus {
		t.Fatalf("menus: %q", s.Phase)
	}
}

func TestSoundsOutsideAGameDontCount(t *testing.T) {
	var s State
	s.feed(strings.NewReader(strings.SplitN(game, "\n", 3)[2]))
	if len(s.Units) != 0 {
		t.Fatalf("units without a game: %+v", s.Units)
	}
}

func TestPresenceFillsTheQueue(t *testing.T) {
	s := State{Phase: PhaseMenus}
	s.presence([]byte(`{"activity":{"location":"game_map_displayname_22","mode":"game_display_name_queueId_1100","progress":{"type":"CUSTOM","value":"InGame"}}}`))
	if s.QueueID != 1100 || s.Phase != PhaseInGame {
		t.Fatalf("%+v", s)
	}
	s = State{Phase: PhasePostGame, QueueID: 2210}
	s.presence([]byte(`{"activity":{"mode":"game_display_name_queueId_1100","progress":{"value":"InGame"}}}`))
	if s.QueueID != 2210 || s.Phase != PhasePostGame {
		t.Fatalf("the log wins: %+v", s)
	}
	s.presence(nil)
}

func TestUnitCap(t *testing.T) {
	s := State{Phase: PhaseInGame}
	for i := 0; i < MaxUnits+3; i++ {
		s.sound("TFTAudioEventSubsystem: Trigger[Audio.Event.VO.Unit.Purchase] Actor[X {DA_18_Champ" + string(rune('A'+i)) + "}]")
	}
	if len(s.Units) != MaxUnits {
		t.Fatalf("%d units", len(s.Units))
	}
}

func TestLittleLegendFromTheLobbyOnly(t *testing.T) {
	var s State
	s.feed(strings.NewReader(`[2026.11.08-12.08.26:225][150]TFT: Error: [Standalone] TFTNiagaraSubsystem: Failed to find ComponentTag matching Request[Id[4], ComponentTag[IdleParticles], NiagaraState[Hidden], Actor[BP_PetStyleTwoAhri_Skin1_C_2147462317 {DA_StyleTwoAhri_Base_Classic_Tier1}]
[2026.11.08-12.08.37:446][400]TFT: [Client] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::InGame]
[2026.11.08-12.08.40:102][223]TFT: Error: [Client] TFTNiagaraSubsystem: Failed to find ComponentTag matching Request[Id[7], ComponentTag[IdleParticles], NiagaraState[Hidden], Actor[BP_PetFoxy_Skin3_C_2147461225 {DA_Foxy_Space_Tier2}]]
`))
	if s.Companion != "StyleTwoAhri_Base_Classic_Tier1" {
		t.Fatalf("companion %q", s.Companion)
	}
}

func TestArenaFromTheLoadingWorldOnly(t *testing.T) {
	var s State
	s.feed(strings.NewReader(`[2026.11.08-12.08.35:668][1]LogStreaming: Display: UWorld::AddToWorld: computing time limit for /Arena_Backstage/Day/SubLevels/1_0_Backstage_Day_DefaultLighting took (less than) 122.17 ms
[2026.11.08-12.08.37:446][400]TFT: [Client] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::InGame]
[2026.11.08-12.10.00:000][1]LogStreaming: Display: UWorld::AddToWorld: computing time limit for /Arena_ZaunCity/Night/SubLevels/1_0_ZaunCity_Night_DefaultLighting took (less than) 99 ms
[2026.11.08-12.12.27:454][110]TFT: [Standalone] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::EndOfGameInterstitial]
[2026.11.08-12.15.50:408][152]TFT: [Client] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::InGame]
`))
	if s.Arena != "Backstage_Day" {
		t.Fatalf("arena %q", s.Arena)
	}
}

func TestFromHeadFindsTheLegendAndArena(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/TFT.log"
	head := `[2026.11.08-12.08.26:225][150]TFT: Error: [Standalone] TFTNiagaraSubsystem: Failed to find ComponentTag matching Request[Id[4], ComponentTag[IdleParticles], NiagaraState[Hidden], Actor[BP_PetStyleTwoAhri_Skin1_C_2147462317 {DA_StyleTwoAhri_Base_Classic_Tier1}]
[2026.11.08-12.08.35:668][1]LogStreaming: Display: UWorld::AddToWorld: computing time limit for /Arena_Backstage/Day/SubLevels/1_0_Backstage_Day_DefaultLighting took (less than) 122.17 ms
[2026.11.08-12.08.37:446][400]TFT: [Client] TFTLobbySubsystem_2147482177: Entering LobbyState[ETFTLobbyState::InGame]
`
	if err := os.WriteFile(path, []byte(head+strings.Repeat("LogTemp: filler line\n", 10)), 0o600); err != nil {
		t.Fatal(err)
	}
	s := fromHead(path)
	if s.Companion != "StyleTwoAhri_Base_Classic_Tier1" || s.Arena != "Backstage_Day" {
		t.Fatalf("got %q %q", s.Companion, s.Arena)
	}
	if got := fromHead(dir + "/missing.log"); got.Arena != "" {
		t.Fatal("a missing log gives nothing")
	}
}

func TestLittleLegendIsRemembered(t *testing.T) {
	dir := t.TempDir()
	old := rememberPath
	rememberPath = func() string { return dir + "/tft-companion.txt" }
	defer func() { rememberPath, remembered, rememberedRead = old, "", false }()
	remembered, rememberedRead = "", false

	if recall() != "" {
		t.Fatal("nothing seen yet")
	}
	remember("StyleTwoAhri_Base_Classic_Tier1")
	remembered, rememberedRead = "", false // a new run
	if got := recall(); got != "StyleTwoAhri_Base_Classic_Tier1" {
		t.Fatalf("got %q", got)
	}
	os.WriteFile(dir+"/tft-companion.txt", []byte("../../evil"), 0o644)
	remembered, rememberedRead = "", false
	if got := recall(); got != "" {
		t.Fatalf("junk on disk read as %q", got)
	}
}
