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
