package app

import (
	"bytes"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// sentinelCatalogServer serves an OpenRouter-shaped payload holding two priced
// models and one carrying the "-1" placeholder price, then points the refresh at
// it. Nothing here reaches openrouter.ai.
//
// None of the canned models carries an "id", which is what keeps the run fully
// offline: deriveZDR's per-model endpoint fetch is the one request in a refresh
// that still builds its URL inline from a model's id, and its job list is
// `WHERE source='openrouter' AND id != ”`, so id-less models leave it empty.
func sentinelCatalogServer(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"name":"Test: Priced One","canonical_slug":"test/priced-one","pricing":{"prompt":"0.000001","completion":"0.000002"}},
			{"name":"Test: Router","canonical_slug":"test/router","pricing":{"prompt":"-1","completion":"-1"}},
			{"name":"Test: Priced Two","canonical_slug":"test/priced-two","pricing":{"prompt":"0.000003","completion":"0.000004"}}
		]}`))
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

// twiceListedCatalogServer serves a feed carrying ONE model under TWO records,
// which is what separates "rows created" from "records the refresh thought were
// missing". Offline for the same reason sentinelCatalogServer is: no record
// carries an id, so the ZDR pass has no job to fetch.
func twiceListedCatalogServer(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"name":"Test: Listed Twice","canonical_slug":"test/listed-twice","pricing":{"prompt":"0.000001","completion":"0.000002"}},
			{"name":"Test: Listed Twice","canonical_slug":"test/listed-twice","pricing":{"prompt":"0.000001","completion":"0.000002"}}
		]}`))
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

// captureLog redirects the standard logger into a buffer for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// TestAnUnpriceableModelIsStoredWithNoPrice pins the rule that a model
// OpenRouter will not quote a price for is kept, not discarded. "-1" upstream
// means the model is no longer servable there; it is still a real model, so it
// gets a row like any other and simply has no price.
//
// The row is what makes this matter. It carries the reader's own notes,
// favourite and custom-column values, which exist in no other file, and a
// delete would cascade them away - so a model going off sale would silently
// destroy what somebody wrote about it.
func TestAnUnpriceableModelIsStoredWithNoPrice(t *testing.T) {
	migratedDB(t)
	sentinelCatalogServer(t)
	// Seeded first so the refresh MEETS all three rather than creating them:
	// clearing a price that was already stored is the case this test is for.
	for _, m := range []struct{ name, slug string }{
		{"Test: Priced One", "test/priced-one"},
		{"Test: Router", "test/router"},
		{"Test: Priced Two", "test/priced-two"},
	} {
		if _, err := store.UpsertAPIModel(catalog.NormalizeModel(map[string]interface{}{
			"name": m.name, "canonical_slug": m.slug,
		})); err != nil {
			t.Fatalf("seed %s: %v", m.name, err)
		}
	}
	buf := captureLog(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}

	stored, err := dbcore.CountModels()
	if err != nil {
		t.Fatalf("count models: %v", err)
	}
	if stored != 3 {
		t.Fatalf("the database holds %d models, want all 3 - the unpriceable one included", stored)
	}
	if !store.ModelExists("Test: Router") {
		t.Fatal("the model OpenRouter would not price was not stored; it must be kept")
	}

	prompt := colStr(t, "Test: Router", "price_prompt")
	completion := colStr(t, "Test: Router", "price_completion")
	if prompt != "" || completion != "" {
		t.Errorf("prices are %q/%q, want both empty - the -1 placeholder is not a price", prompt, completion)
	}

	out := buf.String()
	if !strings.Contains(out, "3 model(s) stored from the OpenRouter feed") {
		t.Errorf("the summary must count all 3 rows it stored, got:\n%s", out)
	}
	if !strings.Contains(out, "0 model(s) added to the library by this refresh") {
		t.Errorf("all 3 were already here, so the summary must report 0 added, got:\n%s", out)
	}
}

// TestAPriceThatGoesAwayClearsTheFigureAndKeepsTheNote is the case the rule
// exists for: a model that HAD a real price, and personal data written against
// it, then stops being priced upstream. The figures must go and everything the
// reader wrote must stay.
func TestAPriceThatGoesAwayClearsTheFigureAndKeepsTheNote(t *testing.T) {
	migratedDB(t)

	priced := map[string]interface{}{
		"name": "Test: Was Priced", "canonical_slug": "test/was-priced",
		"pricing": map[string]interface{}{"prompt": "0.000001", "completion": "0.000002"},
	}
	if _, err := store.UpsertAPIModel(priced); err != nil {
		t.Fatalf("seed the priced model: %v", err)
	}
	if err := store.SaveCells("Test: Was Priced", map[string]string{"notes": "my own note", "favorite": "yes"}); err != nil {
		t.Fatalf("write the personal fields: %v", err)
	}

	unpriceable := map[string]interface{}{
		"name": "Test: Was Priced", "canonical_slug": "test/was-priced",
		"pricing": map[string]interface{}{"prompt": "-1", "completion": "-1"},
	}
	if _, err := store.UpsertAPIModel(unpriceable); err != nil {
		t.Fatalf("refresh it as unpriceable: %v", err)
	}

	if !store.ModelExists("Test: Was Priced") {
		t.Fatal("the row is gone - it must never be deleted for want of a price")
	}
	prompt := colStr(t, "Test: Was Priced", "price_prompt")
	completion := colStr(t, "Test: Was Priced", "price_completion")
	notes := colStr(t, "Test: Was Priced", "notes")
	favorite := colInt(t, "Test: Was Priced", "favorite")
	if prompt != "" || completion != "" {
		t.Errorf("prices are %q/%q, want both cleared - the old figures have stopped being true", prompt, completion)
	}
	if notes != "my own note" {
		t.Errorf("notes = %q, want the note kept", notes)
	}
	if favorite != 1 {
		t.Errorf("favorite = %d, want it kept", favorite)
	}
}

// TestARefreshStoresEveryFeedModel is the direct proof of the rule this app now
// runs on: every record the feed carries becomes a row, whether or not the
// catalog held it before. Nothing is seeded, so every row the refresh leaves
// behind is one it created, and the reported count has to be all three.
func TestARefreshStoresEveryFeedModel(t *testing.T) {
	migratedDB(t)
	sentinelCatalogServer(t)
	buf := captureLog(t)

	added, _, _, err := catalog.FetchModels()
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}

	stored, err := dbcore.CountModels()
	if err != nil {
		t.Fatalf("count models: %v", err)
	}
	if stored != 3 {
		t.Fatalf("the refresh stored %d row(s), want all 3 the feed listed", stored)
	}
	if added != 3 {
		t.Fatalf("the refresh reported %d added, want 3 - every row it left behind is one it created", added)
	}

	out := buf.String()
	if !strings.Contains(out, "3 model(s) added to the library by this refresh") {
		t.Errorf("the summary must report all 3 as added, got:\n%s", out)
	}
}

// TestARowTheRefreshCreatesIsVisibleAndNotAVariation pins how a created row
// arrives: on the list, and as a model of its own rather than a build of
// another. Both are read back out of the models table rather than assumed from
// the insert, because the schema default is what decides hidden.
func TestARowTheRefreshCreatesIsVisibleAndNotAVariation(t *testing.T) {
	migratedDB(t)
	sentinelCatalogServer(t)
	captureLog(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}

	for _, name := range []string{"Test: Priced One", "Test: Router", "Test: Priced Two"} {
		identity, held, err := store.IdentityOf(name)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		if !held {
			t.Fatalf("%q was not created by the refresh", name)
		}
		if identity.Hidden {
			t.Errorf("%q arrived hidden; a row the refresh creates must be on the list", name)
		}
		if identity.VariantOf != "" {
			t.Errorf("%q arrived as a variation of %q; a feed record is a model of its own", name, identity.VariantOf)
		}
		if identity.Source != string(store.SourceOpenRouter) {
			t.Errorf("%q was stored with source %q, want openrouter", name, identity.Source)
		}
	}
}

// TestARefreshOverAFullCatalogCreatesNothing is the other half of the count: a
// catalog that already holds the whole feed must report zero, so the number on
// the page can never be the size of the feed rather than what changed. It also
// pins that a second refresh writes no second row for a name it already stored.
func TestARefreshOverAFullCatalogCreatesNothing(t *testing.T) {
	migratedDB(t)
	sentinelCatalogServer(t)
	captureLog(t)

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("first FetchModels: %v", err)
	}
	added, _, _, err := catalog.FetchModels()
	if err != nil {
		t.Fatalf("second FetchModels: %v", err)
	}
	if added != 0 {
		t.Fatalf("the second refresh reported %d added, want 0 - it held the whole feed already", added)
	}
	stored, err := dbcore.CountModels()
	if err != nil {
		t.Fatalf("count models: %v", err)
	}
	if stored != 3 {
		t.Fatalf("the catalog holds %d row(s) after two refreshes, want 3", stored)
	}
}

// TestTheCountIsRowsCreatedNotRecordsMissed pins what the number means. The feed
// here lists the same name twice, so a count of feed records the refresh
// believed were missing would say two where only one row exists.
func TestTheCountIsRowsCreatedNotRecordsMissed(t *testing.T) {
	migratedDB(t)
	twiceListedCatalogServer(t)
	captureLog(t)

	added, _, _, err := catalog.FetchModels()
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if added != 1 {
		t.Fatalf("the refresh reported %d added, want 1 - the feed lists one model, twice", added)
	}
	stored, err := dbcore.CountModels()
	if err != nil {
		t.Fatalf("count models: %v", err)
	}
	if stored != 1 {
		t.Fatalf("the catalog holds %d row(s), want 1", stored)
	}
}
