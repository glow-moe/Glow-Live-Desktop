package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"hold":["v26.12"]}`))
	}))
	defer srv.Close()
	holdURL = srv.URL
	if !Held("v26.12") || Held("v26.11") {
		t.Fatal("hold list misread")
	}
	holdURL = "http://127.0.0.1:1/nothing"
	if Held("v26.12") {
		t.Fatal("a failed check counted as a hold")
	}
}

func TestNotesAreTheReleaseBullets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"body":"# v26.11\n\nIntro line\n- **Windows**: a launcher\n- Pairing shows a ` + "`code`" + `\n"}`))
	}))
	defer srv.Close()
	notesURL = srv.URL + "/"
	got := Notes("v26.11")
	if len(got) != 2 || got[0] != "Windows: a launcher" || got[1] != "Pairing shows a code" {
		t.Fatalf("notes %q", got)
	}
}
