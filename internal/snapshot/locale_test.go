package snapshot

import (
	"reflect"
	"testing"

	"github.com/glow-moe/glow-collector/internal/ddragon"
	"github.com/glow-moe/glow-collector/internal/live"
)

// spell is a summoner-spell slot as a non-English client sends it: the
// display name translated, the raw name the same in every language.
func spell(display, key string) live.SpellSlot {
	return live.SpellSlot{DisplayName: display, RawDisplayName: "GeneratedTip_SummonerSpell_" + key + "_DisplayName"}
}

func trPlayer(champ, s1, k1, s2, k2, position string, items ...int) live.Player {
	p := live.Player{ChampionName: champ, RawChampionName: "game_character_displayname_" + champ, Position: position, Team: "ORDER"}
	p.SummonerSpells.SummonerSpellOne = spell(s1, k1)
	p.SummonerSpells.SummonerSpellTwo = spell(s2, k2)
	for i, id := range items {
		p.Items = append(p.Items, live.Item{ItemID: id, Slot: i})
	}
	return p
}

// A Turkish client ("Sıçra" is Flash, "Çarp" Smite...) used to come out as
// Flash + Flash for all ten players; the raw name is read now.
func TestTurkishClientSpells(t *testing.T) {
	ddragon.SetChampionsForTest(map[int]string{99: "Lux", 64: "LeeSin", 22: "Ashe"})
	cases := []struct {
		p    live.Player
		want []string
		role string
	}{
		{trPlayer("Lux", "Sıçra", "SummonerFlash", "Tutuştur", "SummonerDot", ""), []string{"SummonerFlash", "SummonerDot"}, ""},
		{trPlayer("LeeSin", "Çarp", "SummonerSmite", "Sıçra", "SummonerFlash", ""), []string{"SummonerSmite", "SummonerFlash"}, "JUNGLE"},
		{trPlayer("Ashe", "Şifa", "SummonerHeal", "Işınlan", "S12_SummonerTeleportUpgrade", ""), []string{"SummonerHeal", "SummonerTeleport"}, ""},
		{trPlayer("Lux", "Bariyer", "SummonerBarrier", "Bitkinlik", "SummonerExhaust", "", 3865), []string{"SummonerBarrier", "SummonerExhaust"}, "UTILITY"},
	}
	for _, c := range cases {
		row := mapPlayer(c.p, false)
		if !reflect.DeepEqual(row.SpellKeys, c.want) || row.Role != c.role {
			t.Errorf("%s: spells %v role %q, want %v %q", c.p.ChampionName, row.SpellKeys, row.Role, c.want, c.role)
		}
	}
}

func TestSpellKeyFromFallsBackToTheDisplayName(t *testing.T) {
	// No raw name (an older client): the English display name still works.
	if k := ddragon.SpellKeyFrom("", "Ignite"); k != "SummonerDot" {
		t.Errorf("got %s", k)
	}
	if k := ddragon.SpellKeyFrom("GeneratedTip_SummonerSpell_SummonerSmiteAvatarOffensive_DisplayName", "Çarp"); k != "SummonerSmite" {
		t.Errorf("upgraded smite: %s", k)
	}
	if k := ddragon.SpellKeyFrom("GeneratedTip_SummonerSpell_SummonerCherryHold_DisplayName", "x"); k != "SummonerCherryHold" {
		t.Errorf("arena: %s", k)
	}
}

func TestLanesSortOnlyWhenAllKnown(t *testing.T) {
	team := []Player{{Champ: "Jhin", Role: "BOTTOM"}, {Champ: "Zoe", Role: "UTILITY"}, {Champ: "Fizz", Role: "MIDDLE"}, {Champ: "Darius", Role: "TOP"}, {Champ: "Kayn", Role: "JUNGLE"}}
	var got []string
	for _, p := range byLane(team) {
		got = append(got, p.Champ)
	}
	if want := []string{"Darius", "Kayn", "Fizz", "Jhin", "Zoe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sorted %v", got)
	}
	// One unknown (or two on the same lane): the game's order stays.
	team[1].Role = ""
	if out := byLane(team); out[0].Champ != "Jhin" {
		t.Errorf("partial lanes reordered: %v", out)
	}
	team[1].Role = "TOP"
	if out := byLane(team); out[0].Champ != "Jhin" {
		t.Errorf("duplicate lanes reordered: %v", out)
	}
	// The game's own lane wins over the build guess.
	if r := laneOf("middle", []string{"SummonerSmite"}, nil); r != "MIDDLE" {
		t.Errorf("position: %s", r)
	}
}

func TestDrakeNames(t *testing.T) {
	for code, want := range map[string]string{"Air": "a Cloud Drake", "Fire": "an Infernal Drake", "Elder": "the Elder Dragon", "": "a Drake", "Newtech": "a Newtech Drake"} {
		if got := dragon(code); got != want {
			t.Errorf("%q: %q", code, got)
		}
	}
}
