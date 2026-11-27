package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/snapshot"
	"github.com/glow-moe/glow-collector/internal/tft"
)

func TestTFTActivityInAGame(t *testing.T) {
	started := time.UnixMilli(1794139717000)
	s := snapshot.Snapshot{Clock: "4:10", Tft: &snapshot.TFT{State: "inGame", Queue: "Ranked", Set: 18,
		Units: []snapshot.TFTUnit{{ChampKey: "Varus", Stars: 2}, {ChampKey: "Akali", Stars: 2}, {ChampKey: "Xayah", Stars: 1}, {ChampKey: "Kayle", Stars: 1}}}}
	a := tftActivity(s, started, "melocet")
	if a.Details != "Teamfight Tactics · Ranked" || a.State != "2★ Varus · 2★ Akali · Xayah" {
		t.Fatalf("got %q / %q", a.Details, a.State)
	}
	if !strings.Contains(a.Assets.LargeImage, "/tiles/Varus_0.jpg") || a.Assets.LargeText != "Varus" || a.Timestamps.Start != started.UnixMilli() {
		t.Fatalf("got %+v %+v", a.Assets, a.Timestamps)
	}
	if !strings.HasSuffix(a.Buttons[0].URL, "/melocet/tft") {
		t.Fatalf("buttons %+v", a.Buttons)
	}
	if d := tftDetail(s.Tft, s.Clock); d != "In a game · Ranked · 4:10" {
		t.Fatalf("detail %q", d)
	}
}

func TestTFTActivityOutOfAGame(t *testing.T) {
	a := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "menus"}}, time.Time{}, "")
	if a.Details != "Teamfight Tactics" || a.State != "In the menus" || a.Assets.LargeImage != tftLogo || a.Timestamps != nil || a.Buttons != nil {
		t.Fatalf("got %+v", a)
	}
	early := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "inGame", Set: 18}}, time.Time{}, "")
	if early.State != "Set 18" {
		t.Fatalf("no units yet: %q", early.State)
	}
	if over := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "postGame", Units: []snapshot.TFTUnit{{ChampKey: "Varus", Stars: 3}}}}, time.Time{}, ""); over.State != "Game over" || over.Assets.LargeImage != tftLogo {
		t.Fatalf("end screen: %+v", over)
	}
}

func TestTFTActivityShowsTheLittleLegend(t *testing.T) {
	icon := "https://raw.communitydragon.org/x/tooltip_styletwoahri_base_classic_tier1.png"
	for _, state := range []string{"menus", "inGame"} {
		a := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: state, CompanionIcon: icon, Units: []snapshot.TFTUnit{{ChampKey: "Varus", Stars: 2}}}}, time.Time{}, "")
		if a.Assets.LargeImage != icon || a.Assets.LargeText != "Teamfight Tactics" {
			t.Fatalf("%s: %+v", state, a.Assets)
		}
	}
}

func TestTFTMenusLogoAndAClock(t *testing.T) {
	opened := time.UnixMilli(1795770000000)
	a := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "menus"}}, tftSince(tft.State{Phase: "menus", OpenedAt: opened}), "")
	if a.Assets.LargeImage != tftLogo || a.Assets.SmallImage != glowIcon {
		t.Fatalf("TFT logo with the glow badge: %+v", a.Assets)
	}
	if a.Timestamps == nil || a.Timestamps.Start != opened.UnixMilli() {
		t.Fatalf("menus clock: %+v", a.Timestamps)
	}
	game := time.UnixMilli(1795771000000)
	if got := tftSince(tft.State{StartedAt: game, OpenedAt: opened}); !got.Equal(game) {
		t.Fatalf("in a game the clock is the game's: %v", got)
	}
	icon := snapshot.TFTCompanionIcon("StyleTwoAhri_Base_Classic_Tier1")
	if b := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "menus", CompanionIcon: icon}}, time.Time{}, ""); b.Assets.SmallImage != glowIcon {
		t.Fatalf("badge with a Little Legend: %+v", b.Assets)
	}
}

func TestTFTEndScreenSaysThePlace(t *testing.T) {
	a := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "postGame", Place: 2}}, time.Time{}, "")
	if a.State != "Finished 2nd" {
		t.Fatalf("got %q", a.State)
	}
	for n, want := range map[int]string{1: "1st", 3: "3rd", 4: "4th", 8: "8th", 11: "11th"} {
		if ordinal(n) != want {
			t.Errorf("%d: %s", n, ordinal(n))
		}
	}
}
