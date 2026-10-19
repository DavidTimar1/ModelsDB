package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"modelsdb/internal/store"
)

// colourColumn asks the server to say what colour a column paints its cells,
// exactly as the column dialog does.
func colourColumn(t *testing.T, key, rules string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(`{"key":` + asJSON(key) + `,"color_rules":` + rules + `}`)
	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodPatch, "/api/columns", body))
	return rec
}

// TestAnyColumnCanBeColouredByWhatItHolds walks the three shapes over HTTP, on
// three columns the reader invented, and reads each one back off the column
// list - which is where the browser gets them.
func TestAnyColumnCanBeColouredByWhatItHolds(t *testing.T) {
	migratedDB(t)
	number, err := ownColumnKey(t, "Speed score", store.ValueTypeNumber, nil)
	if err != nil {
		t.Fatal(err)
	}
	flag, err := ownColumnKey(t, "Tried it", store.ValueTypeFlag, nil)
	if err != nil {
		t.Fatal(err)
	}
	choice, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}

	if rec := colourColumn(t, number,
		`{"thresholds":[{"below":10,"color":"#ef4444"},{"below":100,"color":"#eab308"}],"otherwise":"#22c55e"}`); rec.Code != http.StatusOK {
		t.Fatalf("colour a number column: %d %s", rec.Code, rec.Body.String())
	}
	if rec := colourColumn(t, flag, `{"yes":"#22c55e","no":"#ef4444"}`); rec.Code != http.StatusOK {
		t.Fatalf("colour a flag column: %d %s", rec.Code, rec.Body.String())
	}
	if rec := colourColumn(t, choice, `{"options":{"gold":"#eab308","silver":"#94a3b8"}}`); rec.Code != http.StatusOK {
		t.Fatalf("colour a choice column: %d %s", rec.Code, rec.Body.String())
	}

	byKey := map[string]store.Column{}
	for _, c := range listColumns(t) {
		byKey[c.Key] = c
	}
	if r := byKey[number].Colors; r == nil || len(r.Thresholds) != 2 || r.Thresholds[0].Color != "#ef4444" || r.Otherwise != "#22c55e" {
		t.Errorf("the number column answers %+v", r)
	}
	if r := byKey[flag].Colors; r == nil || r.Yes != "#22c55e" || r.No != "#ef4444" {
		t.Errorf("the flag column answers %+v", r)
	}
	if r := byKey[choice].Colors; r == nil || r.Options["gold"] != "#eab308" {
		t.Errorf("the choice column answers %+v", r)
	}

	// Clearing them is sending none, and it leaves the column where it was.
	if rec := colourColumn(t, choice, `{}`); rec.Code != http.StatusOK {
		t.Fatalf("clear the colours: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range listColumns(t) {
		if c.Key == choice && c.Colors != nil {
			t.Errorf("%s still answers %+v after being cleared", choice, c.Colors)
		}
	}
}

// TestATextColumnIsRefusedColours, and a list column with it: neither has an
// order to shade along or a fixed set of values to colour one by one, so the
// refusal says which kinds do.
func TestATextColumnIsRefusedColours(t *testing.T) {
	migratedDB(t)
	text, err := ownColumnKey(t, "Sizing notes", store.ValueTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := ownColumnKey(t, "Tags", store.ValueTypeList, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{text, list} {
		rec := colourColumn(t, key, `{"thresholds":[{"below":1,"color":"#22c55e"}],"otherwise":"#ef4444"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400: %s", key, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "number") {
			t.Errorf("the refusal for %s does not say which kinds take colours: %s", key, rec.Body.String())
		}
	}
}

// TestAColourRequestThatCannotBeMeantIsRefused: a colour that is not a hex code,
// thresholds in the wrong order, and an option the column does not offer. Each
// answers 400 and nothing is stored.
func TestAColourRequestThatCannotBeMeantIsRefused(t *testing.T) {
	migratedDB(t)
	number, err := ownColumnKey(t, "Speed score", store.ValueTypeNumber, nil)
	if err != nil {
		t.Fatal(err)
	}
	choice, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ key, rules string }{
		{number, `{"thresholds":[{"below":1,"color":"red"}],"otherwise":"#ef4444"}`},
		{number, `{"thresholds":[{"below":9,"color":"#22c55e"},{"below":1,"color":"#eab308"}],"otherwise":"#ef4444"}`},
		{choice, `{"options":{"platinum":"#22c55e"}}`},
		{choice, `{"yes":"#22c55e","no":"#ef4444"}`},
	} {
		if rec := colourColumn(t, c.key, c.rules); rec.Code != http.StatusBadRequest {
			t.Errorf("%s with %s answered %d, want 400: %s", c.key, c.rules, rec.Code, rec.Body.String())
		}
	}
	for _, c := range listColumns(t) {
		if (c.Key == number || c.Key == choice) && c.Colors != nil {
			t.Errorf("%s stored %+v after every request was refused", c.Key, c.Colors)
		}
	}
}

// TestTheTwoPriceColumnsAreColouredOnAFreshInstall. The colouring the table has
// always painted a price with is now an ordinary rule those two columns carry,
// so it arrives with the catalog rather than with the browser's settings file.
func TestTheTwoPriceColumnsAreColouredOnAFreshInstall(t *testing.T) {
	migratedDB(t)
	// What the server does at startup, and the only thing a fresh install does
	// before anybody looks at it.
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
	coloured := map[string]*store.ColorRules{}
	for _, c := range listColumns(t) {
		if c.Colors != nil {
			coloured[c.Key] = c.Colors
		}
	}
	for _, key := range []string{"price_prompt", "price_completion"} {
		rules := coloured[key]
		if rules == nil {
			t.Fatalf("%s is not coloured on a fresh install", key)
		}
		if len(rules.Thresholds) != 2 || rules.Thresholds[0].Below != 1 || rules.Thresholds[1].Below != 2 ||
			rules.Otherwise != "#ef4444" {
			t.Errorf("%s ships %+v, want under one green, under two yellow, the rest red", key, rules)
		}
		delete(coloured, key)
	}
	if len(coloured) != 0 {
		t.Errorf("%v ship coloured too, and only the two price columns do", coloured)
	}
}

// TestAChangeRequestNamingNeitherIsRefused. A PATCH says what to change, so one
// that says nothing is a mistake rather than a no-op.
func TestAChangeRequestNamingNeitherIsRefused(t *testing.T) {
	migratedDB(t)
	key, err := ownColumnKey(t, "Speed score", store.ValueTypeNumber, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodPatch, "/api/columns",
		strings.NewReader(`{"key":`+asJSON(key)+`}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a PATCH naming neither a label nor colours answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
