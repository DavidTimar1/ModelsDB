package app

import (
	"encoding/json"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFoldNameSplitsOnPunctuationAndLetterDigitBoundaries is C2's worked
// example, pinned exactly: "Qwen/Qwen3-32B-FP8" folds to
// ["qwen","qwen","3","32","b","fp","8"].
func TestFoldNameSplitsOnPunctuationAndLetterDigitBoundaries(t *testing.T) {
	got := foldName("Qwen/Qwen3-32B-FP8")
	want := []string{"qwen", "qwen", "3", "32", "b", "fp", "8"}
	if !eqStr(got, want) {
		t.Errorf("foldName(%q) = %v, want %v", "Qwen/Qwen3-32B-FP8", got, want)
	}
}

// TestFoldedNamesScoreAsTheSameName is C2's central claim: two names that are
// the same model spelled with different punctuation score 0 against each
// other, the same as if they were written identically.
func TestFoldedNamesScoreAsTheSameName(t *testing.T) {
	score, why := nameCloseness("Qwen3-32B", "Qwen-3-32B")
	if score != 0 {
		t.Errorf("nameCloseness(%q, %q) = %d, want 0", "Qwen3-32B", "Qwen-3-32B", score)
	}
	if why != "the same name written differently" {
		t.Errorf(`why = %q, want "the same name written differently"`, why)
	}
}

// TestNameClosenessIsTheBestOfRawAndFolded is C3: a pair the RAW comparison
// already handles (one name containing the other) still scores through this
// path, and folding never makes an otherwise-close pair look worse.
func TestNameClosenessIsTheBestOfRawAndFolded(t *testing.T) {
	// Raw containment: "Qwen3-32B" is a substring of "Qwen: Qwen3-32B" once
	// lowercased, so this must score 1 even though folding changes nothing
	// about that relationship.
	score, why := nameCloseness("Qwen3-32B", "Qwen: Qwen3-32B")
	if score != 1 {
		t.Errorf("nameCloseness(%q, %q) = %d, want 1", "Qwen3-32B", "Qwen: Qwen3-32B", score)
	}
	if why != "one name contains the other" {
		t.Errorf(`why = %q, want "one name contains the other"`, why)
	}

	// A pair with no relationship at all under either rule scores -1 (no match).
	if score, _ := nameCloseness("Qwen3-32B", "Llama 4 Scout"); score >= 0 {
		t.Errorf("nameCloseness(unrelated names) = %d, want a negative (no match) score", score)
	}
}

// TestEditDistanceCountsRunesNotBytes is C4: a multi-byte character counts as
// ONE change, not several. "café" to "cafe" is a single character
// substitution (é, two UTF-8 bytes, for e, one byte); a byte-wise comparison
// would have scored this higher.
func TestEditDistanceCountsRunesNotBytes(t *testing.T) {
	if got := editDistance("café", "cafe"); got != 1 {
		t.Errorf("editDistance(%q, %q) = %d, want 1 (one rune changed)", "café", "cafe", got)
	}
}

// TestEditDistanceOnASCIIIsUnchanged is C4's other half: an ASCII-only pair
// must score exactly as a byte-wise comparison would, since every ASCII byte
// is its own rune. TestAnUnknownParameterIsRefusedAndNamesTheNearestColumns
// (query_test.go) is the guard that proves this at the column-suggestion call
// site; this pins the same guarantee directly on editDistance.
func TestEditDistanceOnASCIIIsUnchanged(t *testing.T) {
	tests := []struct{ a, b string }{
		{"favorite", "favourite"},
		{"context_length", "contextlength"},
		{"", "x"},
		{"abc", "abc"},
	}
	for _, tt := range tests {
		got := editDistance(tt.a, tt.b)
		want := byteEditDistance(tt.a, tt.b)
		if got != want {
			t.Errorf("editDistance(%q, %q) = %d, want %d (byte-wise)", tt.a, tt.b, got, want)
		}
	}
}

// byteEditDistance is the ORIGINAL byte-indexed implementation editDistance
// carried before C4, kept here ONLY as this test's own independent oracle -
// never called by the app - so the rune change is checked against the old
// behaviour rather than against itself.
func byteEditDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			c := previous[j] + 1
			if current[j-1]+1 < c {
				c = current[j-1] + 1
			}
			if previous[j-1]+cost < c {
				c = previous[j-1] + cost
			}
			current[j] = c
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

// aCatalogOfNamesToFindSimilar stores four models: two variants of the same
// name spelled differently, one that plainly contains a misspelling of it, and
// one unrelated model - which is the spread C5-C7's tests need.
func aCatalogOfNamesToFindSimilar(t *testing.T) {
	t.Helper()
	migratedDB(t)
	for _, name := range []string{"Qwen: Qwen3-32B", "Llama 4 Scout"} {
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
	}
}

// similarGET drives GET /api/models/similar and decodes the answer, failing
// the test if the request was refused.
func similarGET(t *testing.T, query string) []similarModel {
	t.Helper()
	rec := httptest.NewRecorder()
	handleSimilarModels(rec, httptest.NewRequest(http.MethodGet, "/api/models/similar"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/models/similar%s status = %d, body %s", query, rec.Code, rec.Body.String())
	}
	var out []similarModel
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET /api/models/similar%s body is not a JSON array: %v (%s)", query, err, rec.Body.String())
	}
	return out
}

// similarRefused drives GET /api/models/similar and returns the refusal,
// failing the test when the request was answered instead.
func similarRefused(t *testing.T, query string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handleSimilarModels(rec, httptest.NewRequest(http.MethodGet, "/api/models/similar"+query, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/models/similar%s answered %d, want 400: %s", query, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestSimilarModelsRefusals is C5: name is required, limit has a ceiling, and
// an unknown parameter is refused rather than ignored.
func TestSimilarModelsRefusals(t *testing.T) {
	aCatalogOfNamesToFindSimilar(t)

	if body := similarRefused(t, ""); !strings.Contains(body, "name") {
		t.Errorf("a missing name was not refused clearly: %s", body)
	}
	if body := similarRefused(t, "?name="); !strings.Contains(body, "name") {
		t.Errorf("a blank name was not refused clearly: %s", body)
	}
	if body := similarRefused(t, "?name=Qwen&limit=51"); !strings.Contains(body, "50") {
		t.Errorf("a limit above 50 was not refused naming the ceiling: %s", body)
	}
	if body := similarRefused(t, "?name=Qwen&limit=nope"); !strings.Contains(body, "nope") {
		t.Errorf("a non-numeric limit was not refused: %s", body)
	}
	if body := similarRefused(t, "?name=Qwen&sort=asc"); !strings.Contains(body, `"sort"`) {
		t.Errorf("an unknown parameter was not refused naming it: %s", body)
	}
}

// TestSimilarModelsShapeAndOrdering is C6: best first, the documented shape,
// and rows with no match left out entirely.
func TestSimilarModelsShapeAndOrdering(t *testing.T) {
	aCatalogOfNamesToFindSimilar(t)

	out := similarGET(t, "?name=Qwen3-32B")
	if len(out) == 0 {
		t.Fatal("served no rows for a name that should find Qwen: Qwen3-32B")
	}
	if out[0].Name != "Qwen: Qwen3-32B" {
		t.Errorf("best match = %q, want %q", out[0].Name, "Qwen: Qwen3-32B")
	}
	if out[0].Why == "" {
		t.Error("the best match carries no \"why\"")
	}
	for i := 1; i < len(out); i++ {
		if out[i].Score < out[i-1].Score {
			t.Errorf("row %d (score %d) is closer than row %d (score %d): not best-first", i, out[i].Score, i-1, out[i-1].Score)
		}
	}
	// The unrelated model shares no word and is not within reach by edits, so it
	// must be left out entirely rather than answered with a low score.
	for _, m := range out {
		if m.Name == "Llama 4 Scout" {
			t.Errorf("an unrelated name was included: %+v", m)
		}
	}
}

// TestSimilarModelsHonoursLimitAndIncludeHidden is the rest of C5-C7: limit
// caps the answer, and a hidden row is left out unless include_hidden asks for
// it, matching GET /api/models.
func TestSimilarModelsHonoursLimitAndIncludeHidden(t *testing.T) {
	aCatalogOfNamesToFindSimilar(t)
	if err := store.SetHidden("Llama 4 Scout", true); err != nil {
		t.Fatalf("hide Llama 4 Scout: %v", err)
	}

	if out := similarGET(t, "?name=Llama&limit=1"); len(out) > 1 {
		t.Errorf("limit=1 served %d rows", len(out))
	}
	if out := similarGET(t, "?name=Llama%204%20Scoot"); len(out) != 0 {
		t.Errorf("a hidden row was served without include_hidden=1: %+v", out)
	}
	out := similarGET(t, "?name=Llama%204%20Scoot&include_hidden=1")
	found := false
	for _, m := range out {
		if m.Name == "Llama 4 Scout" {
			found = true
			if !m.Hidden {
				t.Error("Llama 4 Scout is hidden in the store but the row says hidden:false")
			}
		}
	}
	if !found {
		t.Errorf("a hidden row was not found even with include_hidden=1: %+v", out)
	}
}

// TestAddModelCarriesASimilarAdvisory is C8: FindExistingModel's exact-match
// refusal is unaffected, and a SUCCESSFUL add carries the "similar" advisory,
// always present, excluding the row it just created.
func TestAddModelCarriesASimilarAdvisory(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("Qwen: Qwen3-32B", map[string]string{}); err != nil {
		t.Fatalf("seed Qwen: Qwen3-32B: %v", err)
	}

	// A close-but-not-identical name is still CREATED - C8 says the 409 stays
	// exact, so this must succeed rather than refuse.
	rec := addModel(t, `{"name":"Qwen3-32B","cells":{}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (a near-duplicate name must still be created); body %s", rec.Code, rec.Body.String())
	}
	var answer struct {
		OK      bool           `json:"ok"`
		Name    string         `json:"name"`
		Similar []similarModel `json:"similar"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v (%s)", err, rec.Body.String())
	}
	if answer.Similar == nil {
		t.Error(`"similar" is absent from the answer, want an array (possibly empty)`)
	}
	found := false
	for _, m := range answer.Similar {
		if m.Name == "Qwen3-32B" {
			t.Error("the row just created appears in its own \"similar\" advisory")
		}
		if m.Name == "Qwen: Qwen3-32B" {
			found = true
		}
	}
	if !found {
		t.Errorf(`"similar" does not name the close row already here: %+v`, answer.Similar)
	}

	// The exact-match refusal (409) is UNCHANGED: adding "Qwen: Qwen3-32B" again,
	// character for character, still refuses.
	rec2 := addModel(t, `{"name":"Qwen: Qwen3-32B","cells":{}}`)
	if rec2.Code != http.StatusConflict {
		t.Errorf("re-adding the exact name = %d, want 409 (FindExistingModel must not loosen)", rec2.Code)
	}
}
