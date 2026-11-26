package update

import "testing"

func TestCompareTestBuilds(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v26.9.1", "v26.10", -1},
		{"v26.12", "v26.12", 0},
		{"v26.12.DEV6", "v26.12", -1}, // a test build takes the real release
		{"v26.12.DEV6", "v26.11", 1},
		{"v26.12.DEV4", "v26.12.DEV3", 1},
		{"v26.12.DEV10", "v26.12.DEV9", 1},
		{"v26.12.1", "v26.12.DEV6", 1},
		{"v26.12.1", "v26.12", 1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
