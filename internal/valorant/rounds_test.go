package valorant

import (
	"encoding/json"
	"testing"
)

func TestRoundsClutchAceAndFeed(t *testing.T) {
	players := []FinalPlayer{{Team: "Red", Self: true}, {Team: "Red"}, {Team: "Blue"}, {Team: "Blue"}, {Team: "Blue"}}
	subjects := []string{"me", "mate", "e1", "e2", "e3"}
	vandal := "9c82e19d-4575-0200-1a81-3eacf00cf872"
	raw := `[
	 {"roundNum":0,"roundResult":"Eliminated","winningTeam":"Red","playerStats":[
	   {"subject":"e1","kills":[{"roundTime":1000,"killer":"e1","victim":"mate","finishingDamage":{"damageType":"Weapon","damageItem":"` + vandal + `"}}]},
	   {"subject":"me","kills":[
	     {"roundTime":4000,"killer":"me","victim":"e3","finishingDamage":{"damageType":"Weapon","damageItem":"` + vandal + `"}},
	     {"roundTime":2000,"killer":"me","victim":"e1","finishingDamage":{"damageType":"Weapon","damageItem":"` + vandal + `"}},
	     {"roundTime":3000,"killer":"me","victim":"e2","finishingDamage":{"damageType":"Ability","damageItem":"Ultimate"}}],
	    "damage":[{"receiver":"e1","headshots":2},{"receiver":"e2","headshots":1}]}]},
	 {"roundNum":1,"roundResult":"Bomb detonated","winningTeam":"Blue","bombPlanter":"e2","playerStats":[
	   {"subject":"e2","kills":[{"roundTime":500,"killer":"e2","victim":"me","finishingDamage":{"damageType":"Weapon","damageItem":"` + vandal + `"}}]}]}
	]`
	var in []roundResult
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	rs, weapons := rounds(in, players, subjects)
	if len(rs) != 2 || len(weapons) != 1 || weapons[0] != vandal {
		t.Fatalf("rounds %d weapons %v", len(rs), weapons)
	}
	r1 := rs[0]
	if r1.N != 1 || !r1.Won || r1.Kills != 3 || r1.HS != 3 || r1.Clutch != 3 || r1.Died {
		t.Fatalf("round 1: %+v", r1)
	}
	// In order of time, the ability kill with no weapon.
	if r1.Feed[0] != [4]int{1000, 2, 1, 0} || r1.Feed[2] != [4]int{3000, 0, 3, -1} {
		t.Fatalf("feed: %v", r1.Feed)
	}
	r2 := rs[1]
	if r2.Won || !r2.Died || r2.Clutch != 0 || r2.How != "Bomb detonated" || r2.Planted {
		t.Fatalf("round 2: %+v", r2)
	}
	// Deathmatch: no teams, no rounds.
	if rs, _ := rounds(in, []FinalPlayer{{Self: true}, {}, {}, {}, {}}, subjects); rs != nil {
		t.Fatal("deathmatch has no rounds")
	}
}
