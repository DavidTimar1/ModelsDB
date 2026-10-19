package app

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"testing"
)

type searchCase struct {
	Text    string   `json:"text"`
	Members []string `json:"members"`
	Want    bool     `json:"want"`
}

func loadSearchCases(t *testing.T) []searchCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/search_cases.json")
	if err != nil {
		t.Fatalf("read the shared search cases: %v", err)
	}
	var cases []searchCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("the shared search cases are not valid: %v", err)
	}
	return cases
}

// TestSearchSyntaxSharedCases runs the table the JavaScript twin is held to.
func TestSearchSyntaxSharedCases(t *testing.T) {
	for i, c := range loadSearchCases(t) {
		if got := parseSearch(c.Text).matches(c.Members); got != c.Want {
			t.Errorf("case %d: text %q members %q: got %v, want %v", i, c.Text, c.Members, got, c.Want)
		}
	}
}

// TestSearchSyntaxJavaScriptAgrees runs the same table through the matcher in
// app.js when node is installed, so the panel in the browser and the server
// cannot drift apart.
func TestSearchSyntaxJavaScriptAgrees(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; run testdata/run_search_cases.js where it is")
	}
	out, err := exec.Command(node, "testdata/run_search_cases.js").CombinedOutput()
	if err != nil {
		t.Errorf("the JavaScript matcher disagrees with the shared cases:\n%s", out)
	}
}

// TestSearchSyntaxThroughTheAPI drives the three Go entry points.
func TestSearchSyntaxThroughTheAPI(t *testing.T) {
	aCatalogToQuery(t)
	esc := url.QueryEscape
	tests := []struct {
		about string
		query string
		want  []string
	}{
		{"every word, any order", "?name=" + esc("image flux"), []string{"Flux Image"}},
		{"a phrase out of order finds nothing", "?name=" + esc(`"image flux"`), []string{}},
		{"a phrase in order", "?name=" + esc(`"flux image"`), []string{"Flux Image"}},
		{"NOT", "?name=" + esc("gemini !flash"), []string{}},
		{"NOT keeps the others", "?name=" + esc("!flash"), []string{"Claude Opus", "Flux Image"}},
		{"OR", "?name=" + esc("claude OR flux"), []string{"Claude Opus", "Flux Image"}},
		{"half-typed text is not refused", "?name=" + esc(`"flux im`), []string{"Flux Image"}},
		{"contains with two words in any order", "?description=" + esc("contains:diffs long"), []string{"Claude Opus"}},
		{"contains NOT matches an empty cell", "?description=" + esc("contains:!diffs"), []string{"Flux Image", "Gemini Flash"}},
		{"contains with a positive term never matches an empty cell", "?description=" + esc("contains:assistant !zzz"), []string{"Claude Opus"}},
		{"list column, terms met by different members", "?input_modalities=" + esc("contains:image text"), []string{"Flux Image"}},
		{"list column, a phrase must sit in one member", "?input_modalities=" + esc(`contains:"text image"`), []string{}},
		{"list column, NOT", "?input_modalities=" + esc("contains:!image"), []string{"Claude Opus", "Gemini Flash"}},
		{"contains alone still matches everything", "?description=" + esc("contains:"), []string{"Claude Opus", "Flux Image", "Gemini Flash"}},
	}
	for _, tt := range tests {
		t.Run(tt.about, func(t *testing.T) {
			if got := queried(t, tt.query); !eqStr(got, tt.want) {
				t.Errorf("GET /api/models%s served %v, want %v", tt.query, got, tt.want)
			}
		})
	}

	values := servedValues(t, "?key=input_modalities&contains="+esc("text !image"))
	if values.DistinctTotal != 1 || values.Values[0].Value != "text" {
		t.Errorf("/api/columns/values contains=%q answered %v, want text alone", "text !image", values.Values)
	}
	if got := servedValues(t, "?key=input_modalities&contains="+esc(`"`)); got.DistinctTotal != 2 {
		t.Errorf("a lone quote narrowed the values to %d, want both", got.DistinctTotal)
	}
}
