package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/glow-moe/glow-collector/internal/snapshot"
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
	if !strings.HasSuffix(a.Buttons[0].URL, "/melocet/league") {
		t.Fatalf("buttons %+v", a.Buttons)
	}
	if d := tftDetail(s.Tft, s.Clock); d != "In a game · Ranked · 4:10" {
		t.Fatalf("detail %q", d)
	}
}

func TestTFTActivityOutOfAGame(t *testing.T) {
	a := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "menus"}}, time.Time{}, "")
	if a.Details != "Teamfight Tactics" || a.State != "In the menus" || a.Assets.LargeImage != glowIcon || a.Timestamps != nil || a.Buttons != nil {
		t.Fatalf("got %+v", a)
	}
	early := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "inGame", Set: 18}}, time.Time{}, "")
	if early.State != "Set 18" {
		t.Fatalf("no units yet: %q", early.State)
	}
	if over := tftActivity(snapshot.Snapshot{Tft: &snapshot.TFT{State: "postGame", Units: []snapshot.TFTUnit{{ChampKey: "Varus", Stars: 3}}}}, time.Time{}, ""); over.State != "Game over" || over.Assets.LargeImage != glowIcon {
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
