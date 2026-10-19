package app

import (
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"
)

// feedServer points the catalog fetch at a local server whose feed carries
// exactly the records given, as a JSON array body, so the record/create/fill
// path runs end to end with no request to openrouter.ai.
func feedServer(t *testing.T, records string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[` + records + `]}`))
	})
	mux.HandleFunc("/all-providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/videos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/eu", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	origAPI, origProviders := catalog.ApiUrl, catalog.AllProvidersURL
	origVideo, origImage := catalog.VideoModelsURL, catalog.ImageModelsURL
	catalog.ApiUrl = server.URL + "/models"
	origEU := catalog.EURoutingURL
	t.Cleanup(func() { catalog.EURoutingURL = origEU })
	catalog.EURoutingURL = server.URL + "/eu"
	catalog.AllProvidersURL = server.URL + "/all-providers"
	catalog.VideoModelsURL = server.URL + "/videos"
	catalog.ImageModelsURL = server.URL + "/images"
	t.Cleanup(func() {
		catalog.ApiUrl = origAPI
		catalog.AllProvidersURL = origProviders
		catalog.VideoModelsURL = origVideo
		catalog.ImageModelsURL = origImage
	})
}

// feedFieldServer serves one record carrying one field no shipped column
// reads (benchmarks.artificial_analysis.coding_index), or none when
// codingIndex is empty.
func feedFieldServer(t *testing.T, codingIndex string) {
	t.Helper()
	body := `{"name":"Test: Feed Field Model","canonical_slug":"test/feed-field-model"`
	if codingIndex != "" {
		body += `,"benchmarks":{"artificial_analysis":{"coding_index":` + codingIndex + `}}`
	}
	feedServer(t, body+`}`)
}

func seedFeedFieldModel(t *testing.T) {
	t.Helper()
	if _, err := store.UpsertAPIModel(catalog.NormalizeModel(map[string]interface{}{
		"name": "Test: Feed Field Model", "canonical_slug": "test/feed-field-model",
	})); err != nil {
		t.Fatalf("seed the model: %v", err)
	}
}

// madeColumns answers every column tied to a feed path, by key, as the
// refresh left it.
func madeColumns(t *testing.T) map[string]store.Column {
	t.Helper()
	tied, err := store.ImportedFeedFieldColumns()
	if err != nil {
		t.Fatalf("ImportedFeedFieldColumns: %v", err)
	}
	out := map[string]store.Column{}
	for key := range tied {
		col, err := store.ColumnByKey(key)
		if err != nil {
			t.Fatalf("ColumnByKey(%s): %v", key, err)
		}
		out[key] = col
	}
	return out
}

// TestARefreshMakesAColumnForAFeedFieldAndFillsItInTheSameRefresh is the
// end-to-end path: ONE refresh records a leaf path the shipped columns do not
// read, makes a column for it under the whole path as its key, hidden and
// sealed and sourced openrouter then hand, and fills it from the feed already
// in memory - nobody imports anything. The next refresh keeps it current.
func TestARefreshMakesAColumnForAFeedFieldAndFillsItInTheSameRefresh(t *testing.T) {
	migratedDB(t)
	feedFieldServer(t, "37.2")
	seedFeedFieldModel(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("first FetchModels: %v", err)
	}

	made := madeColumns(t)
	col, ok := made["benchmarks_artificial_analysis_coding_index"]
	if !ok {
		t.Fatalf("the refresh made %v, want benchmarks_artificial_analysis_coding_index among them", made)
	}
	if col.Label != "Benchmarks artificial analysis coding index" {
		t.Errorf("label = %q", col.Label)
	}
	if col.ValueType != store.ValueTypeNumber {
		t.Errorf("value type = %q, want number", col.ValueType)
	}
	if col.Visible {
		t.Errorf("the column arrived visible, want hidden")
	}
	if !col.Sealed {
		t.Errorf("the column arrived unsealed, want sealed")
	}
	if got := cellValue(t, "Test: Feed Field Model", col.Key); got != "37.2" {
		t.Errorf("%s holds %q after the refresh that made it, want 37.2", col.Key, got)
	}
	if err := store.DeleteColumn(col.Key); err == nil {
		t.Errorf("DeleteColumn removed a column the refresh made, want it refused")
	}

	feedFieldServer(t, "41.5")
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("second FetchModels: %v", err)
	}
	if got := cellValue(t, "Test: Feed Field Model", col.Key); got != "41.5" {
		t.Errorf("%s holds %q after the second refresh, want 41.5", col.Key, got)
	}
	if again := madeColumns(t); len(again) != len(made) {
		t.Errorf("the second refresh made %d column(s), want none new (%v)", len(again)-len(made), again)
	}
}

// TestARefreshMakesNoColumnForAPathSomethingAlreadyReads: a path a shipped
// column reads (context_length, architecture.tokenizer, pricing.prompt) and
// the top-level name, read into the model row, make nothing; every other
// leaf with a value does.
func TestARefreshMakesNoColumnForAPathSomethingAlreadyReads(t *testing.T) {
	migratedDB(t)
	feedServer(t, `{"name":"Test: Mapped Model","canonical_slug":"test/mapped","context_length":8192,
		"architecture":{"tokenizer":"Qwen3","modality":"text->text"},"pricing":{"prompt":"0.000001"},
		"top_provider":{"context_length":8192}}`)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	made := madeColumns(t)
	for _, key := range []string{"name", "context_length", "architecture_tokenizer", "pricing_prompt"} {
		if _, held := made[key]; held {
			t.Errorf("the refresh made %s, which something already reads", key)
		}
	}
	for _, key := range []string{"architecture_modality", "top_provider_context_length"} {
		if _, held := made[key]; !held {
			t.Errorf("the refresh did not make %s (made %v)", key, made)
		}
	}
	if len(made) != 2 {
		t.Errorf("the refresh made %d column(s), want 2: %v", len(made), made)
	}
}

// TestARefreshMakesAListColumnForAnArrayOfObjects: benchmarks.design_arena is
// [] on one model and an array of leaderboard rows on another. The array's
// own path becomes ONE hidden list column - never walked into per-key
// sub-columns the way it was before override_windows.md's fix - and each row
// becomes one member, its own keys joined in the feed's own order.
func TestARefreshMakesAListColumnForAnArrayOfObjects(t *testing.T) {
	migratedDB(t)
	feedServer(t, `{"name":"Test: Empty Arena","canonical_slug":"test/empty-arena","benchmarks":{"design_arena":[]}},
		{"name":"Test: Ranked Arena","canonical_slug":"test/ranked-arena","benchmarks":{"design_arena":[{"elo":1234.5,"rank":3}]}}`)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	made := madeColumns(t)
	col, held := made["benchmarks_design_arena"]
	if !held {
		t.Fatalf("the refresh did not make benchmarks_design_arena (made %v)", made)
	}
	if col.ValueType != store.ValueTypeList {
		t.Errorf("value type = %q, want list", col.ValueType)
	}
	if _, held := made["benchmarks_design_arena_elo"]; held {
		t.Errorf("the refresh walked into design_arena's elements and made a per-key column")
	}
	if got := cellValue(t, "Test: Ranked Arena", col.Key); got != `["elo=1234.5, rank=3"]` {
		t.Errorf("%s holds %s, want [\"elo=1234.5, rank=3\"]", col.Key, got)
	}
}

// TestARefreshMakesNoColumnForAnAllNullPath: a path no model has a value at
// makes nothing, until the refresh on which some model does. An empty array
// is no value either: supported_voices is [] on every model today, and a
// column for it would hold nothing.
func TestARefreshMakesNoColumnForAnAllNullPath(t *testing.T) {
	migratedDB(t)
	feedServer(t, `{"name":"Test: Null Model","canonical_slug":"test/null","per_request_limits":null,"supported_voices":[]}`)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("first FetchModels: %v", err)
	}
	if made := madeColumns(t); len(made) != 0 {
		t.Errorf("the refresh made %v for a feed whose unread paths are null or [] everywhere", made)
	}

	feedServer(t, `{"name":"Test: Null Model","canonical_slug":"test/null","per_request_limits":"none"}`)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("second FetchModels: %v", err)
	}
	if _, held := madeColumns(t)["per_request_limits"]; !held {
		t.Errorf("the refresh on which a model first states per_request_limits did not make its column")
	}
}

// TestAListLeafIsStoredAsOneArray pins the list fill: reasoning.supported_efforts
// is an array in the feed, and the cell holds that array's members - never an
// array holding the array, which reads back as one member.
func TestAListLeafIsStoredAsOneArray(t *testing.T) {
	migratedDB(t)
	feedServer(t, `{"name":"Test: Effort Model","canonical_slug":"test/effort","reasoning":{"supported_efforts":["max","high","low"]}}`)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	col, held := madeColumns(t)["reasoning_supported_efforts"]
	if !held {
		t.Fatalf("the refresh did not make reasoning_supported_efforts")
	}
	if col.ValueType != store.ValueTypeList {
		t.Errorf("value type = %q, want list", col.ValueType)
	}
	if got := cellValue(t, "Test: Effort Model", col.Key); got != `["max","high","low"]` {
		t.Errorf("%s holds %s, want [\"max\",\"high\",\"low\"]", col.Key, got)
	}
}

// TestANumericStringPathBecomesANumberColumn: the feed quotes every price, so
// a path every model states as a numeric string is made as a number column
// and the quoted figures are accepted into it.
func TestANumericStringPathBecomesANumberColumn(t *testing.T) {
	migratedDB(t)
	feedServer(t, `{"name":"Test: Priced Model","canonical_slug":"test/priced","warm_up_seconds":"0.0000005"},
		{"name":"Test: Free Model","canonical_slug":"test/free","warm_up_seconds":"0"}`)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	col, held := madeColumns(t)["warm_up_seconds"]
	if !held {
		t.Fatalf("the refresh did not make warm_up_seconds")
	}
	if col.ValueType != store.ValueTypeNumber {
		t.Errorf("value type = %q, want number", col.ValueType)
	}
	if got := cellValue(t, "Test: Priced Model", col.Key); got != "0.0000005" {
		t.Errorf("%s holds %q, want 0.0000005", col.Key, got)
	}
	if got := cellValue(t, "Test: Free Model", col.Key); got != "0" {
		t.Errorf("%s holds %q for the free model, want 0", col.Key, got)
	}
}

// TestPricingOverridesIsOneListColumnWithOneMemberPerWindow is the app-level
// end-to-end pin for the fix override_windows.md describes: a model with
// several price windows used to show a cell pairing one window's price with
// a different window's hours (each per-key column took the first window that
// stated that key). Now pricing.overrides is ONE list column, and each
// window is its own member, whole, in the feed's own key order.
func TestPricingOverridesIsOneListColumnWithOneMemberPerWindow(t *testing.T) {
	migratedDB(t)
	feedServer(t, `{"name":"Test: Windowed Model","canonical_slug":"test/windowed","pricing":{"overrides":[
		{"prompt":"0.00000156","completion":"0.0000078","min_prompt_tokens":32000},
		{"prompt":"0.00000195","completion":"0.00000975","min_prompt_tokens":128000}
	]}}`)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	col, held := madeColumns(t)["pricing_overrides"]
	if !held {
		t.Fatalf("the refresh did not make pricing_overrides")
	}
	if col.ValueType != store.ValueTypeList {
		t.Errorf("value type = %q, want list", col.ValueType)
	}
	want := `["prompt=0.00000156, completion=0.0000078, min_prompt_tokens=32000","prompt=0.00000195, completion=0.00000975, min_prompt_tokens=128000"]`
	if got := cellValue(t, "Test: Windowed Model", col.Key); got != want {
		t.Errorf("%s holds %s, want %s", col.Key, got, want)
	}
}

// TestARefreshReportsNewAndVanishedFieldsForTheWarning checks the exact values
// refreshState.run() (internal/app/refresh.go) turns straight into a warning
// via addWarning - "the feed dropped %d field(s) ...: %s" naming the path -
// without driving run() itself, which also scrapes collections and pricing
// over the real network and has no place in an offline test.
func TestARefreshReportsNewAndVanishedFieldsForTheWarning(t *testing.T) {
	migratedDB(t)
	seedFeedFieldModel(t)

	feedFieldServer(t, "37.2")
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("first FetchModels (seeds the baseline): %v", err)
	}

	// Second refresh: the field is gone.
	feedFieldServer(t, "")
	_, newFields, vanishedFields, err := catalog.FetchModels()
	if err != nil {
		t.Fatalf("second FetchModels: %v", err)
	}
	if len(newFields) != 0 {
		t.Errorf("expected no new fields, got %v", newFields)
	}
	var sawVanished bool
	for _, p := range vanishedFields {
		if p == "benchmarks.artificial_analysis.coding_index" {
			sawVanished = true
		}
	}
	if !sawVanished {
		t.Errorf("expected benchmarks.artificial_analysis.coding_index among vanishedFields, got %v", vanishedFields)
	}
}
