package shared

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.0.8", "0.1.0", -1},
		{"0.1.0", "0.0.8", 1},
		{"1.2.3", "1.2.10", -1},   // numeric, not lexical
		{"v0.1.0", "0.1.0", 0},    // leading v tolerated
		{"0.1.0-rc1", "0.1.0", 0}, // pre-release suffix ignored
		{"1.0", "1.0.0", 0},       // missing patch == 0
		{"2.0.0", "1.9.9", 1},
	}
	for _, c := range cases {
		if got := CompareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestIsDevVersion pins which builds skip the update nag and the self-updater:
// the bare fallback, an unset version, and anything the dev wrappers stamp with
// a -dev suffix. A release version never does.
func TestIsDevVersion(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	cases := []struct {
		version string
		want    bool
	}{
		{"dev", true},
		{"", true},
		{"1.26.1-dev", true},
		{"1.26.1", false},
		{"v1.26.1", false},
		{"1.26.1-rc1", false},
	}
	for _, c := range cases {
		Version = c.version
		if got := IsDevVersion(); got != c.want {
			t.Errorf("IsDevVersion() with Version = %q is %v, want %v", c.version, got, c.want)
		}
	}
}
