package fortnite

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOutfitNameLooksUpInTheBackground(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/character_hammervice" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"status":200,"data":{"id":"Character_HammerVice","name":"Coastal Catwoman"}}`))
	}))
	defer srv.Close()
	nameBase = srv.URL + "/"
	defer func() { nameBase = "https://fortnite-api.com/v2/cosmetics/br/" }()

	if got := OutfitName("Character_HammerVice"); got != "" {
		t.Fatalf("first ask answers before the lookup: %q", got)
	}
	deadline := time.Now().Add(3 * time.Second)
	for OutfitName("character_hammervice") == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := OutfitName("character_hammervice"); got != "Coastal Catwoman" {
		t.Fatalf("got %q", got)
	}
	if calls != 1 {
		t.Fatalf("%d lookups", calls)
	}
	if OutfitName("../x") != "" || OutfitName("") != "" {
		t.Fatal("bad ids are never looked up")
	}
}
