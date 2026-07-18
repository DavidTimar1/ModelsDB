package catalog

import "testing"

func TestTitleize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"black-forest_labs", "Black Forest Labs"},
		{"flux.2-pro", "Flux.2 Pro"}, // only '-' and '_' become spaces; '.' is kept
		{"openai", "Openai"},
		{"", ""},
	}
	for _, c := range cases {
		if got := titleize(c.in); got != c.want {
			t.Errorf("titleize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDeriveName(t *testing.T) {
	if got := deriveName("black-forest-labs/flux.2-pro"); got != "Black Forest Labs: Flux.2 Pro" {
		t.Errorf("deriveName author/model = %q", got)
	}
	if got := deriveName("soloname"); got != "Soloname" {
		t.Errorf("deriveName without slash = %q", got)
	}
}
