package app

import (
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cannedOpenRouterModel is one upstream record carrying every kind of fact the
// catalogue records: an id to take the author from, the two architecture texts,
// the parameter list, the provider's limits, the dates, the reasoning answer and
// a price no column of models holds.
//
// It deliberately carries NO "id" of its own, for the same reason
// TestFetchModelsWritesNoFileArtefact's record does not: the ZDR pass builds its
// per-model URL inline from the id, so a record carrying one would reach the
// real openrouter.ai from a test. The author is derived from that id, so the
// author fact is proved in the catalog package instead, where the derivation
// can be exercised without a network at all
// (TestOpenRouterFactsReadWhatNoColumnHolds).
const cannedOpenRouterModel = `{"data":[{
  "name":"Test: Refresh Model",
  "canonical_slug":"test-vendor/refresh-model",
  "context_length":8192,
  "architecture":{"input_modalities":["text"],"output_modalities":["text"],
                  "tokenizer":"Qwen3","instruct_type":"chatml"},
  "supported_parameters":["tools","temperature"],
  "top_provider":{"context_length":8192,"max_completion_tokens":4096,"is_moderated":true},
  "knowledge_cutoff":"2026-02-16",
  "reasoning":{"mandatory":false},
  "pricing":{"prompt":"0.000001","completion":"0.000002","input_cache_read":"0.00000005"}
}]}`

// fakeUpstream points the catalog fetch, the ZDR provider map and the video
// and image listings at a local server, and puts every address back
// afterwards. The video and image listings answer empty: a test exercising
// them by name points VideoModelsURL/ImageModelsURL at its own server
// instead (see dedicatedlistings_test.go), and every other test must still
// never reach the real openrouter.ai for them.
func fakeUpstream(t *testing.T, payload string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
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
		server.Close()
	})
}

// seedRefreshModel upserts the row cannedOpenRouterModel names, matched by its
// slug, so a refresh has something already in the library to update - the
// refresh itself creates no row, so every test below that exercises it starts
// from one seeded this way rather than from the fetch itself.
func seedRefreshModel(t *testing.T) {
	t.Helper()
	if _, err := store.UpsertAPIModel(catalog.NormalizeModel(map[string]interface{}{
		"name": "Test: Refresh Model", "canonical_slug": "test-vendor/refresh-model",
	})); err != nil {
		t.Fatal(err)
	}
}

// columnsByName reads the reader's own columns back as a heading-to-key map,
// lower-cased, which is how every lookup by heading in the app compares them.
func columnsByName(t *testing.T) map[string]string {
	t.Helper()
	cols, err := ownColumnsList()
	if err != nil {
		t.Fatalf("list the columns of the reader's own: %v", err)
	}
	out := map[string]string{}
	for _, c := range cols {
		out[strings.ToLower(c.Label)] = c.Key
	}
	return out
}

// TestTheCatalogFetchRecordsWhatOpenRouterStates is the missing half of the
// OpenRouter pull: a fresh install that fetches the catalog for the first time
// must end up with every fact upstream states in a column of OpenRouter's, where
// the table can sort, filter and show it - not with the whole record buried in
// one blob where nothing can - and with a column made for every leaf no
// shipped column reads.
func TestTheCatalogFetchRecordsWhatOpenRouterStates(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}

	want := map[string]string{
		"tokenizer":             "Qwen3",
		"instruct_type":         "chatml",
		"supported_parameters":  "tools, temperature",
		"max_completion_tokens": "4096",
		"moderated":             "yes",
		"knowledge_cutoff":      "2026-02-16",
		"reasoning_mandatory":   "no",
		"price_cache_read":      "0.00000005",
	}
	for key, expected := range want {
		if got := cellValue(t, "Test: Refresh Model", key); got != expected {
			t.Errorf("%q holds %q, want %q", key, got, expected)
		}
	}
	// And the one leaf no shipped column reads, top_provider.context_length,
	// becomes a column of the app's own in this same refresh: keyed by the
	// whole path, hidden, sealed, and already holding the value. It is not a
	// column of the reader's - the refresh makes none of those.
	made, err := store.ColumnByKey("top_provider_context_length")
	if err != nil {
		t.Fatalf("the refresh did not make top_provider_context_length: %v", err)
	}
	if made.Visible || !made.Sealed || made.ValueType != store.ValueTypeNumber {
		t.Errorf("top_provider_context_length is visible=%v sealed=%v %s, want hidden, sealed, number", made.Visible, made.Sealed, made.ValueType)
	}
	if got := cellValue(t, "Test: Refresh Model", "top_provider_context_length"); got != "8192" {
		t.Errorf("top_provider_context_length holds %q, want 8192", got)
	}
	for name := range columnsByName(t) {
		if name != "rating" && name != "ocr" {
			t.Errorf("the refresh created the column %q, which is the reader's to make", name)
		}
	}
}

// TestTheCatalogFetchAnswersUnderAColumnThatAlreadyExists is the other half of
// the maintainer's sentence: a column for a fact always already exists (it is
// shipped and sealed the moment the app seeds its columns, never a column the
// reader mints for themselves any more), so the fetched value must land in
// THAT column and never spawn a second one beside it.
func TestTheCatalogFetchAnswersUnderAColumnThatAlreadyExists(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)

	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns first: %v", err)
	}
	existing, ok := columnCalled(t, "Tokenizer")
	if !ok {
		t.Fatal("no shipped \"Tokenizer\" column exists after seeding")
	}
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}

	found := 0
	for _, c := range listColumns(t) {
		if strings.EqualFold(c.Label, "Tokenizer") {
			found++
			if c.Key != existing.Key {
				t.Errorf("the refresh answered under the column %q, want the existing %q", c.Key, existing.Key)
			}
		}
	}
	if found != 1 {
		t.Errorf("there are %d Tokenizer columns after the refresh, want 1", found)
	}
	value, present, err := ownValueOf("Test: Refresh Model", existing.Key)
	if err != nil {
		t.Fatalf("read the existing column back: %v", err)
	}
	if !present || value != "Qwen3" {
		t.Errorf("the existing column holds %q (present=%v), want \"Qwen3\"", value, present)
	}
}

// TestAFactColumnCannotBeDeletedThroughARefresh replaces the old regression for
// a window that closed a different way: a fact's column is shipped and sealed
// the moment it is seeded, so it can never be deleted at all - not by the
// reader, and not by anything a refresh does. What used to be "the refresh must
// not recreate a column the reader deleted" is now "there is no way to delete
// it in the first place", which DeleteColumn's own seal check already
// guarantees; this pins that the guarantee still holds for a real fact column
// after a real refresh has run.
func TestAFactColumnCannotBeDeletedThroughARefresh(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	tokenizer, ok := columnCalled(t, "Tokenizer")
	if !ok {
		t.Fatal("no \"Tokenizer\" column exists after a refresh")
	}
	if !tokenizer.Sealed {
		t.Fatalf("Tokenizer is not sealed after a refresh, want it shipped-and-sealed like every fact column")
	}
	if err := store.DeleteColumn(tokenizer.Key); err == nil {
		t.Error("DeleteColumn removed a sealed fact column, want it refused")
	}
	if got := cellValue(t, "Test: Refresh Model", "tokenizer"); got != "Qwen3" {
		t.Errorf("what OpenRouter states reads as %q, want \"Qwen3\" still recorded", got)
	}
}

// TestARefreshNeverTouchesWhatIsHisOrResearched is the guard that makes an
// unattended pass safe. The refresh runs on every startup over the whole
// catalog: a note is his alone, and a value researched into a column the feed
// never states is kept, because the refresh writes only what it read.
func TestARefreshNeverTouchesWhatIsHisOrResearched(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the first FetchModels: %v", err)
	}
	// expiration_date is a column the feed above never states; parameters is
	// one a repository measures and the feed never reaches.
	if err := store.SaveCells("Test: Refresh Model", map[string]string{
		"notes": "what I know it to be", "expiration_date": "2027-01-01", "parameters": "7",
	}); err != nil {
		t.Fatalf("type the values in: %v", err)
	}

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the second FetchModels: %v", err)
	}
	for key, want := range map[string]string{
		"notes": "what I know it to be", "expiration_date": "2027-01-01", "parameters": "7",
	} {
		value, _, err := ownValueOf("Test: Refresh Model", key)
		if err != nil {
			t.Fatalf("read %s back: %v", key, err)
		}
		if value != want {
			t.Errorf("the refresh replaced %s with %q, want %q", key, value, want)
		}
	}
}

// TestARefreshOutranksAResearchedValueOnceItStatesTheFact is the other side
// of that rule: research fills a gap only until the source states the fact.
// A tokenizer researched before the feed carried one shows until the feed
// does, then the feed's answer shows and the researched one sits beneath it.
func TestARefreshOutranksAResearchedValueOnceItStatesTheFact(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, strings.Replace(cannedOpenRouterModel, `"tokenizer":"Qwen3",`, ``, 1))
	seedRefreshModel(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the first FetchModels: %v", err)
	}
	if err := setOwnValue("Test: Refresh Model", "tokenizer", "my finding"); err != nil {
		t.Fatalf("research the tokenizer: %v", err)
	}
	if value, _, _ := ownValueOf("Test: Refresh Model", "tokenizer"); value != "my finding" {
		t.Fatalf("tokenizer = %q before the feed states one, want the researched value", value)
	}

	fakeUpstream(t, cannedOpenRouterModel)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the second FetchModels: %v", err)
	}
	value, present, err := ownValueOf("Test: Refresh Model", "tokenizer")
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if !present || value != "Qwen3" {
		t.Errorf("tokenizer holds %q (present=%v) once the feed states it, want \"Qwen3\"", value, present)
	}
	if got := sourceStates(t, "Test: Refresh Model", "tokenizer", "research"); got != "my finding" {
		t.Errorf("the researched tokenizer = %q, want it kept beneath the feed's", got)
	}
}

// TestARefreshRecordsEveryFactWhetherYouKeepAColumnOrNot is what replaced the
// tick. A source column is not a choice - it holds what the source said - so a
// fact the reader keeps no column of their own for is still recorded, and is
// still there the day they decide they want it.
func TestARefreshRecordsEveryFactWhetherYouKeepAColumnOrNot(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	for _, key := range []string{
		"tokenizer", "instruct_type",
		"moderated", "knowledge_cutoff",
	} {
		if cellValue(t, "Test: Refresh Model", key) == "" {
			t.Errorf("%q was stated upstream and not recorded", key)
		}
	}
	if len(columnsByName(t)) != 0 {
		t.Errorf("the refresh left the reader with %v, want none - a fresh install starts with no custom column of its own", columnsByName(t))
	}
}

// TestARefreshFillsAnEmptyCell is the first of the three cases the refresh
// decides between. A cell holding nothing has nobody's answer in it, so
// upstream's is written straight in.
func TestARefreshFillsAnEmptyCell(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)

	// "Tokenizer" is the shipped column "tokenizer" - nothing here mints a
	// column of its own, and the seeded row holds no value for it yet.
	id := "tokenizer"
	if _, present, err := ownValueOf("Test: Refresh Model", id); err != nil || present {
		t.Fatalf("the cell is not empty before the refresh (present=%v, err=%v)", present, err)
	}

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	value, present, err := ownValueOf("Test: Refresh Model", id)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if !present || value != "Qwen3" {
		t.Errorf("the empty cell holds %q (present=%v), want \"Qwen3\"", value, present)
	}
}

// TestARefreshUpdatesWhatAnEarlierRefreshWrote is the second case, and it is the
// reason the database records who wrote each value at all. Upstream revises a
// figure, and a catalog that could never take the revision would hold an answer
// that was true once and is not true now.
func TestARefreshUpdatesWhatAnEarlierRefreshWrote(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the first FetchModels: %v", err)
	}
	// "Tokenizer" is the shipped column "tokenizer", already seeded by the
	// FetchModels call above - nothing here mints a column of its own.
	id := "tokenizer"

	fakeUpstream(t, strings.Replace(cannedOpenRouterModel, `"tokenizer":"Qwen3"`, `"tokenizer":"Qwen4"`, 1))
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the second FetchModels: %v", err)
	}
	value, _, err := ownValueOf("Test: Refresh Model", id)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if value != "Qwen4" {
		t.Errorf("the cell holds %q, want the revised \"Qwen4\"", value)
	}
}

// TestAValueTheFeedStatesCannotBeWrittenOverTheAPI is the third case, from
// the direction an assistant reaches it: a write through POST /api/save onto
// a column the feed states is refused naming OpenRouter and its value, and
// what the feed stated is untouched. A write into a column the feed does not
// state lands as research and the next refresh leaves it where it is.
func TestAValueTheFeedStatesCannotBeWrittenOverTheAPI(t *testing.T) {
	migratedDB(t)
	fakeUpstream(t, cannedOpenRouterModel)
	seedRefreshModel(t)
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the first FetchModels: %v", err)
	}
	rec := saveModels(t, `[{"name":"Test: Refresh Model","cells":{"tokenizer":"the one I checked"}}]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("the save answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Tokenizer holds OpenRouter's answer, Qwen3, which an update maintains; nothing typed replaces it") {
		t.Errorf("the refusal does not name OpenRouter's answer: %s", rec.Body.String())
	}
	if value, _, _ := ownValueOf("Test: Refresh Model", "tokenizer"); value != "Qwen3" {
		t.Errorf("tokenizer = %q after the refused save, want the feed's Qwen3", value)
	}

	rec = saveModels(t, `[{"name":"Test: Refresh Model","cells":{"expiration_date":"2027-01-01"}}]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the save into a gap answered %d: %s", rec.Code, rec.Body.String())
	}
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the second FetchModels: %v", err)
	}
	if value, _, _ := ownValueOf("Test: Refresh Model", "expiration_date"); value != "2027-01-01" {
		t.Errorf("the refresh replaced a value researched over the API with %q", value)
	}
}
