package app

import (
	"encoding/json"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// filtered runs the query grammar exactly as the handler does - over the
// columns this catalog really holds - and fails the test if the query is one the
// server refuses.
func filtered(t *testing.T, rows []map[string]interface{}, q url.Values) []map[string]interface{} {
	t.Helper()
	columns, err := store.ListColumns()
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	out, err := filterAndProject(rows, q, columns)
	if err != nil {
		t.Fatalf("filterAndProject(%v): %v", q, err)
	}
	return out
}

// aCatalogToQuery stores three models whose cells cover every kind of column the
// grammar can be asked about: figures, text, a list, a flag, and a column of the
// reader's own.
func aCatalogToQuery(t *testing.T) {
	t.Helper()
	migratedDB(t)

	write := func(name string, src store.Source, values map[string]string) {
		t.Helper()
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil && !strings.Contains(err.Error(), "already") {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
		if err := store.WriteCells(src, name, values); err != nil {
			t.Fatalf("write %s cells on %q: %v", src, name, err)
		}
	}

	write("Claude Opus", store.SourceOpenRouter, map[string]string{
		"context_length":    "200000",
		"price_prompt":      "0.000003",
		"input_modalities":  `["text"]`,
		"output_modalities": `["text"]`,
		"supports_tools":    "yes",
		"description":       "An assistant that handles long diffs.",
	})
	write("Flux Image", store.SourceOpenRouter, map[string]string{
		"model_type":        "image",
		"input_modalities":  `["text","image"]`,
		"output_modalities": `["image"]`,
	})
	write("Gemini Flash", store.SourceOpenRouter, map[string]string{
		"context_length": "1000000",
		"price_prompt":   "0.0000001",
	})
	write("Gemini Flash", store.SourceHand, map[string]string{
		"notes":    "fast enough for a batch job",
		"favorite": "yes",
	})
}

// queried names the models GET /api/models answers with for that query string,
// sorted so a failure reads the same way every run.
func queried(t *testing.T, query string) []string {
	t.Helper()
	rows := servedModels(t, query)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, shared.GetStr(r, "name"))
	}
	sort.Strings(out)
	return out
}

// refused drives GET /api/models and returns the refusal, failing the test when
// the request was answered instead.
func refused(t *testing.T, query string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models"+query, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/models%s answered %d, want 400: %s", query, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func eqStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAFilterNamesAColumnByItsKey is the whole read grammar: one parameter per
// column, and four ways to ask about one. It covers every kind of column,
// because the kind is what decides which of the four a column takes.
func TestAFilterNamesAColumnByItsKey(t *testing.T) {
	aCatalogToQuery(t)

	tests := []struct {
		about string
		query string
		want  []string
	}{
		{"an exact value in a text column", "?model_type=image", []string{"Flux Image"}},
		{"capitals ignored", "?model_type=IMAGE", []string{"Flux Image"}},
		{"one member of a list column", "?input_modalities=image", []string{"Flux Image"}},
		{"a member every model states", "?input_modalities=text", []string{"Claude Opus", "Flux Image"}},
		{"a flag, written as the other word for yes", "?favorite=true", []string{"Gemini Flash"}},
		{"at least this figure", "?context_length=min:500000", []string{"Gemini Flash"}},
		{"at most this figure", "?price_prompt=max:0.000001", []string{"Gemini Flash"}},
		{"text holding a phrase", "?description=contains:long%20diffs", []string{"Claude Opus"}},
		{"a column nobody has filled", "?notes=", []string{"Claude Opus", "Flux Image"}},
		{"two filters are AND-combined", "?favorite=yes&context_length=min:500000", []string{"Gemini Flash"}},
		{"the display name, on its own", "?name=flux", []string{"Flux Image"}},
	}
	for _, tt := range tests {
		t.Run(tt.about, func(t *testing.T) {
			if got := queried(t, tt.query); !eqStr(got, tt.want) {
				t.Errorf("GET /api/models%s served %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// TestAColumnOfHisOwnIsFilterableTheMomentItExists is the point of filtering by
// key rather than by a list of named parameters: a column nobody wrote into this
// program is queried exactly like a shipped one, with no code change.
func TestAColumnOfHisOwnIsFilterableTheMomentItExists(t *testing.T) {
	aCatalogToQuery(t)

	if _, err := store.CreateOwnColumn(store.ColumnSpec{Label: "Runs on my laptop"}); err != nil {
		t.Fatalf("make a column of his own: %v", err)
	}
	if err := store.WriteCells(store.SourceHand, "Claude Opus",
		map[string]string{"runs_on_my_laptop": "no - too big"}); err != nil {
		t.Fatalf("write a column of his own: %v", err)
	}
	if err := store.WriteCells(store.SourceHand, "Gemini Flash",
		map[string]string{"runs_on_my_laptop": "yes"}); err != nil {
		t.Fatalf("write a column of his own: %v", err)
	}

	if got := queried(t, "?runs_on_my_laptop=yes"); !eqStr(got, []string{"Gemini Flash"}) {
		t.Errorf("filtering by an invented column served %v, want Gemini Flash", got)
	}
	if got := queried(t, "?runs_on_my_laptop=contains:big"); !eqStr(got, []string{"Claude Opus"}) {
		t.Errorf("searching inside an invented column served %v, want Claude Opus", got)
	}
}

// TestFieldsProjectsIdentityAndColumns pins what a narrowed row carries: the
// identity fields named, and a cells object holding only the columns named.
func TestFieldsProjectsIdentityAndColumns(t *testing.T) {
	aCatalogToQuery(t)

	rows := servedModels(t, "?name=gemini&fields=name,notes,context_length")
	if len(rows) != 1 {
		t.Fatalf("served %d rows, want 1", len(rows))
	}
	row := rows[0]
	if len(row) != 2 {
		t.Fatalf("a projected row carries %v, want name and cells only", row)
	}
	if got := shared.GetStr(row, "name"); got != "Gemini Flash" {
		t.Errorf("name = %q", got)
	}
	cells, ok := row["cells"].(map[string]interface{})
	if !ok {
		t.Fatalf("cells is %T, want an object", row["cells"])
	}
	if len(cells) != 2 || cells["notes"] != "fast enough for a batch job" || cells["context_length"] != "1000000" {
		t.Errorf("projected cells = %v, want only the two columns asked for", cells)
	}
}

// TestEveryValueIsAnsweredAsTheTextItIsStoredAs is the promise that makes the
// row shape worth having: what a read gives back is what the database holds,
// character for character, so nothing has to be parsed or re-rendered to be
// written back.
func TestEveryValueIsAnsweredAsTheTextItIsStoredAs(t *testing.T) {
	aCatalogToQuery(t)

	rows := servedModels(t, "?name=claude")
	if len(rows) != 1 {
		t.Fatalf("served %d rows, want 1", len(rows))
	}
	cells, _ := rows[0]["cells"].(map[string]interface{})
	for column, want := range map[string]string{
		"context_length":   "200000",
		"price_prompt":     "0.000003",
		"input_modalities": `["text"]`,
		"supports_tools":   "yes",
	} {
		if got, _ := cells[column].(string); got != want {
			t.Errorf("%s = %q, want the stored text %q", column, got, want)
		}
	}
	if _, held := cells["notes"]; held {
		t.Error("a column this model has no value in should be absent, not empty")
	}

	// The identity fields ride beside the cells, and nothing about the model
	// itself is anywhere else.
	for field, want := range map[string]interface{}{
		"name": "Claude Opus", "source": "manual", "hidden": false,
		"variant_of": "",
	} {
		if got := rows[0][field]; got != want {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
}

// TestAnUnknownParameterIsRefusedAndNamesTheNearestColumns is the refusal that
// replaces silence. A filter this server ignored would answer with the whole
// catalog while looking like a narrow answer, which is the one failure a caller
// cannot see.
func TestAnUnknownParameterIsRefusedAndNamesTheNearestColumns(t *testing.T) {
	aCatalogToQuery(t)

	body := refused(t, "?type=image")
	for _, want := range []string{`"type" is not a parameter`, "model_type"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refusal never says %q: %s", want, body)
		}
	}
	if !strings.Contains(body, "key is close") || !strings.Contains(body, "label is close") {
		t.Errorf("the refusal offers near matches by key or by label, but not both: %s", body)
	}

	// A misspelt key is offered the key it misspells.
	if body := refused(t, "?favourite=yes"); !strings.Contains(body, "favorite") {
		t.Errorf("a misspelt key was not offered the real one: %s", body)
	}
	// And so is a name inside fields=.
	if body := refused(t, "?name=claude&fields=name,note"); !strings.Contains(body, "notes") {
		t.Errorf("fields= naming an unknown column was not offered the real one: %s", body)
	}
}

// TestAComparisonAColumnCannotAnswerIsRefused covers the other half of the
// grammar's honesty: asking for a figure comparison on text, or a substring
// search in a number, is a question that column cannot answer, so it is refused
// rather than quietly matching nothing.
func TestAComparisonAColumnCannotAnswerIsRefused(t *testing.T) {
	aCatalogToQuery(t)

	if body := refused(t, "?model_type=min:3"); !strings.Contains(body, "comparison of figures") {
		t.Errorf("min: on a text column was refused without saying why: %s", body)
	}
	if body := refused(t, "?context_length=contains:200"); !strings.Contains(body, "contains: searches inside text") {
		t.Errorf("contains: on a number column was refused without saying why: %s", body)
	}
	if body := refused(t, "?context_length=min:many"); !strings.Contains(body, "is not one") {
		t.Errorf("min: without a figure was refused without saying why: %s", body)
	}
}

// TestTheColumnListAnswersEveryColumn pins GET /api/columns: the one list a
// caller needs to read every other answer by key, with no id and no routing -
// a column's declared sources (columnsources.go) are what used to be spread
// across a computed column's routes_to.
func TestTheColumnListAnswersEveryColumn(t *testing.T) {
	migratedDB(t)
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the columns this app ships: %v", err)
	}

	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodGet, "/api/columns", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/columns answered %d: %s", rec.Code, rec.Body.String())
	}
	var answered []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &answered); err != nil {
		t.Fatalf("body is not a JSON array: %v (%s)", err, rec.Body.String())
	}

	stored, err := store.ListColumns()
	if err != nil {
		t.Fatalf("ListColumns: %v", err)
	}
	if len(answered) != len(stored) {
		t.Fatalf("the route answered %d columns and the catalog holds %d", len(answered), len(stored))
	}

	byKey := map[string]map[string]interface{}{}
	for _, c := range answered {
		key, _ := c["key"].(string)
		byKey[key] = c
		if _, hasID := c["id"]; hasID {
			t.Errorf("%s carries an id; nothing over the network speaks in ids", key)
		}
	}
	for _, want := range []struct{ key, label, valueType string }{
		{"context_length", "Context length", "number"},
		{"notes", "Note", "text"},
		{"input_modalities", "Takes in", "list"},
	} {
		got := byKey[want.key]
		if got == nil {
			t.Fatalf("the answer never names %s", want.key)
		}
		if got["label"] != want.label || got["value_type"] != want.valueType {
			t.Errorf("%s = %v, want label %q and value_type %q", want.key, got, want.label, want.valueType)
		}
	}
	if got := byKey["input_modalities"]["resolve"]; got != "union" {
		t.Errorf(`input_modalities resolve = %v, want "union"`, got)
	}
	if got := byKey["notes"]["resolve"]; got != "" && got != "first_stated" {
		t.Errorf("notes resolve = %v, want the default", got)
	}
}

// TestTheColumnListCarriesKindAndSources pins what the page draws every
// column's sentence from: one word for what the column is, who may state it
// in rank order, and what a worked-out column is worked out from. One request
// carries it for every column, so the page never asks per column.
func TestTheColumnListCarriesKindAndSources(t *testing.T) {
	migratedDB(t)
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the columns this app ships: %v", err)
	}
	if _, err := store.CreateOwnColumn(store.ColumnSpec{Label: "Rating"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodGet, "/api/columns", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/columns answered %d: %s", rec.Code, rec.Body.String())
	}
	var answered []struct {
		Key         string   `json:"key"`
		Kind        string   `json:"kind"`
		Sources     []string `json:"sources"`
		DerivedFrom string   `json:"derived_from"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answered); err != nil {
		t.Fatalf("body is not a JSON array: %v (%s)", err, rec.Body.String())
	}
	byKey := map[string]struct {
		Kind        string
		Sources     string
		DerivedFrom string
	}{}
	for _, c := range answered {
		byKey[c.Key] = struct {
			Kind        string
			Sources     string
			DerivedFrom string
		}{c.Kind, strings.Join(c.Sources, ","), c.DerivedFrom}
		if c.Sources == nil {
			t.Errorf("%s carries no sources list at all", c.Key)
		}
	}
	for _, want := range []struct{ key, kind, sources, from string }{
		{"context_length", "fetched", "openrouter,huggingface,research", ""},
		{"price_prompt", "fetched", "openrouter,research", ""},
		{"parameters", "measured", "huggingface,research", ""},
		{"moe", "derived", "huggingface,openrouter,research", "the words of the OpenRouter description"},
		{"supports_tools", "derived", "openrouter,research", "OpenRouter's supported parameters (tools or tool_choice)"},
		{"model_type", "derived", "openrouter,huggingface,research", "what OpenRouter says the model gives out, and its description for audio"},
		{"added", "stamped", "app", ""},
		{"notes", "yours", "hand", ""},
		{"rating", "yours", "hand", ""},
	} {
		got := byKey[want.key]
		if got.Kind != want.kind || got.Sources != want.sources || got.DerivedFrom != want.from {
			t.Errorf("%s = kind %q, sources %q, derived_from %q; want %q, %q, %q",
				want.key, got.Kind, got.Sources, got.DerivedFrom, want.kind, want.sources, want.from)
		}
	}
}

// TestTheOldNamedFiltersAreGone pins the clean cut. Each of these parameters
// this API used to take now answers 400 naming it, rather than being ignored and
// answering with every model - which is what an old caller would read as a
// catalog where nothing matched its filter. "zdr" is not among them: the pivot
// gave that attribute a column of its own under that bare name, so ?zdr= is a
// real filter again and is pinned as one below.
func TestTheOldNamedFiltersAreGone(t *testing.T) {
	aCatalogToQuery(t)

	for _, query := range []string{
		"?type=image", "?input=image", "?output=text", "?tool=yes", "?reasoning=yes",
		"?min_context=200000", "?max_in=1", "?max_out=5", "?all=1",
	} {
		body := refused(t, query)
		parameter := strings.SplitN(strings.TrimPrefix(query, "?"), "=", 2)[0]
		if !strings.Contains(body, `"`+parameter+`"`) {
			t.Errorf("GET /api/models%s was refused without naming %q: %s", query, parameter, body)
		}
	}
}

// TestZDRFiltersByItsOwnBareKey is the other half of that cut: the one old
// parameter name the pivot handed back, because the attribute now holds a
// column called exactly that.
func TestZDRFiltersByItsOwnBareKey(t *testing.T) {
	aCatalogToQuery(t)

	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models?zdr=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("?zdr=true answered %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// TestByAsksWhoAnsweredTheColumn covers the provenance filter: by:you and
// by:not_you ask who won a column, not what it holds, so they match against
// the row's answered_by map rather than its cells. Only Gemini Flash carries a
// hand answer for notes in the fixture.
func TestByAsksWhoAnsweredTheColumn(t *testing.T) {
	aCatalogToQuery(t)

	if got := queried(t, "?notes=by:you"); !eqStr(got, []string{"Gemini Flash"}) {
		t.Errorf("?notes=by:you served %v, want Gemini Flash", got)
	}
	if got := queried(t, "?notes=by:not_you"); !eqStr(got, []string{"Claude Opus", "Flux Image"}) {
		t.Errorf("?notes=by:not_you served %v, want Claude Opus and Flux Image", got)
	}

	// A source the column does not declare is refused rather than silently
	// matching nothing, and the refusal names the ones it does.
	if body := refused(t, "?added=by:you"); !strings.Contains(body, `"hand" does not state it`) || !strings.Contains(body, "added is stated by app") {
		t.Errorf("?added=by:you was refused without saying why: %s", body)
	}
	if body := refused(t, "?notes=by:maybe"); !strings.Contains(body, `"maybe" does not state it`) {
		t.Errorf("?notes=by:maybe was refused without saying why: %s", body)
	}
}

// TestByTakesADeclaredSourceName is the other half of by:: any source the
// column declares can be asked for by name, or excluded with not_, so
// "which rows did research answer" and "which rows did OpenRouter answer" are
// both questions, and a column of the reader's own answers by:you only where
// somebody typed into it.
func TestByTakesADeclaredSourceName(t *testing.T) {
	aCatalogToQuery(t)

	if _, err := store.CreateOwnColumn(store.ColumnSpec{Label: "Deployment"}); err != nil {
		t.Fatalf("create the reader's own column: %v", err)
	}
	if err := store.WriteCells(store.SourceHand, "Claude Opus", map[string]string{"deployment": "on-prem"}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCells(store.SourceHand, "Gemini Flash", map[string]string{"favorite": "no"}); err != nil {
		t.Fatal(err)
	}
	// Flux Image has no context length from anywhere; research fills it.
	if err := store.WriteCells(store.SourceResearch, "Flux Image", map[string]string{"context_length": "4096"}); err != nil {
		t.Fatal(err)
	}
	// Claude Opus has OpenRouter's, and a researched value beneath it that
	// OpenRouter outranks.
	if err := store.WriteCells(store.SourceResearch, "Claude Opus", map[string]string{"context_length": "180000"}); err != nil {
		t.Fatal(err)
	}

	if got := queried(t, "?context_length=by:research"); !eqStr(got, []string{"Flux Image"}) {
		t.Errorf("?context_length=by:research served %v, want Flux Image alone: OpenRouter outranks the research on Claude Opus", got)
	}
	if got := queried(t, "?context_length=by:openrouter"); !eqStr(got, []string{"Claude Opus", "Gemini Flash"}) {
		t.Errorf("?context_length=by:openrouter served %v, want Claude Opus and Gemini Flash", got)
	}
	if got := queried(t, "?context_length=by:not_openrouter"); !eqStr(got, []string{"Flux Image"}) {
		t.Errorf("?context_length=by:not_openrouter served %v, want Flux Image", got)
	}
	if got := queried(t, "?deployment=by:you"); !eqStr(got, []string{"Claude Opus"}) {
		t.Errorf("?deployment=by:you served %v, want Claude Opus", got)
	}
	if got := queried(t, "?favorite=by:hand"); !eqStr(got, []string{"Gemini Flash"}) {
		t.Errorf("?favorite=by:hand served %v, want Gemini Flash: you is an alias of hand", got)
	}
	// A fetched column no longer declares hand, so by:you on it is refused.
	if body := refused(t, "?context_length=by:you"); !strings.Contains(body, `"hand" does not state it`) {
		t.Errorf("?context_length=by:you was refused without saying why: %s", body)
	}

	// by: ANDs with any other test on the same request like every other filter.
	if got := queried(t, "?context_length=by:openrouter&favorite=by:not_you"); !eqStr(got, []string{"Claude Opus"}) {
		t.Errorf("?context_length=by:openrouter&favorite=by:not_you served %v, want Claude Opus", got)
	}
	if got := queried(t, "?context_length=by:research&favorite=by:you"); len(got) != 0 {
		t.Errorf("?context_length=by:research&favorite=by:you served %v, want nothing", got)
	}
}
