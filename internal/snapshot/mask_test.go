package snapshot

import "testing"

func sample() Snapshot {
	return Snapshot{
		Me: Me{RiotName: "Guts", RiotTag: "aykms"},
		Blue: []Player{
			{Summoner: "Ashe Main#EUW"},
			{Summoner: "VermilioN#0001"},
		},
		Red: []Player{
			{Summoner: "Guts#aykms", IsMe: true},
			{Summoner: "DearWolf#TR1"},
		},
		Feed: []FeedEvent{
			{Who: "DearWolf", Text: "slew VermilioN"},
			{Who: "Guts", Text: "slew Ashe Main"},
			{Who: "Ashe Main", Text: "slayed Baron Nashor"},
		},
	}
}

func TestMaskOthersHidesBothSidesAndFeed(t *testing.T) {
	s := sample()
	s.Mask(false, true)
	if s.Blue[0].Summoner != hiddenName || s.Blue[1].Summoner != hiddenName || s.Red[1].Summoner != hiddenName {
		t.Fatalf("other players not hidden: %+v %+v", s.Blue, s.Red)
	}
	if s.Red[0].Summoner != "Guts#aykms" || s.Me.RiotName != "Guts" {
		t.Fatalf("owner should stay visible: %q %q", s.Red[0].Summoner, s.Me.RiotName)
	}
	if s.Feed[0].Who != hiddenName || s.Feed[0].Text != "slew "+hiddenName {
		t.Fatalf("feed not scrubbed: %+v", s.Feed[0])
	}
	if s.Feed[1].Who != "Guts" || s.Feed[1].Text != "slew "+hiddenName {
		t.Fatalf("owner line wrong: %+v", s.Feed[1])
	}
	if s.Feed[2].Text != "slayed Baron Nashor" {
		t.Fatalf("objective text changed: %+v", s.Feed[2])
	}
}

func TestMaskMyNameOnlyOwner(t *testing.T) {
	s := sample()
	s.Mask(true, false)
	if s.Red[0].Summoner != hiddenName || s.Me.RiotName != hiddenName || s.Me.RiotTag != "" {
		t.Fatalf("owner not hidden: %+v %+v", s.Red[0], s.Me)
	}
	if s.Red[1].Summoner != "DearWolf#TR1" || s.Feed[0].Who != "DearWolf" {
		t.Fatalf("others should stay: %+v %+v", s.Red[1], s.Feed[0])
	}
	if s.Feed[1].Who != hiddenName {
		t.Fatalf("owner feed line not hidden: %+v", s.Feed[1])
	}
}

func TestReplaceNameWholeWordsOnly(t *testing.T) {
	if got := replaceName("slew Ashe", "Ash"); got != "slew Ashe" {
		t.Fatalf("partial name replaced: %q", got)
	}
	if got := replaceName("slew Ash", "Ash"); got != "slew "+hiddenName {
		t.Fatalf("whole name not replaced: %q", got)
	}
}
