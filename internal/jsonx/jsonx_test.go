package jsonx

import (
	"strings"
	"testing"
)

func TestKeepsTheRestOfAFrame(t *testing.T) {
	var v struct {
		Name  string `json:"name"`
		Items []int  `json:"items"`
		Level int    `json:"level"`
	}
	if err := Unmarshal([]byte(`{"name":"Akali","items":{"error":"Unable to find player"},"level":18}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "Akali" || v.Level != 18 || v.Items != nil {
		t.Fatalf("got %+v", v)
	}
	if err := Unmarshal([]byte(`{"name":`), &v); err == nil {
		t.Fatal("broken JSON read as fine")
	}
	if err := Decode(strings.NewReader(`{"level":"x","name":"B"}`), &v); err != nil || v.Name != "B" {
		t.Fatalf("decode: %v %+v", err, v)
	}
}
