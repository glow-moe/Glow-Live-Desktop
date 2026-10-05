package lcu

import "testing"

func TestTitleCase(t *testing.T) {
	for in, want := range map[string]string{"CLASSIC": "Classic", "ONE FOR ALL": "One For All", "ultbook": "Ultbook", "GOLD": "Gold", "  diamond ": "Diamond", "": ""} {
		if got := titleCase(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
