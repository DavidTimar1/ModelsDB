package app

import (
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// freeListingCatalogServer serves an OpenRouter-shaped payload holding a model
// and the free endpoint OpenRouter lists beside it. The free listing carries the
// shape that makes it awkward: a name ending in "(free)" and the PAID model's
// canonical slug.
//
// Neither model carries an "id", which is what keeps the run offline - see
// sentinelCatalogServer for why.
func freeListingCatalogServer(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"name":"Meta: Llama 3.3 70B Instruct","canonical_slug":"meta-llama/llama-3.3-70b-instruct","pricing":{"prompt":"0.000001","completion":"0.000002"}},
			{"name":"Meta: Llama 3.3 70B Instruct (free)","canonical_slug":"meta-llama/llama-3.3-70b-instruct","pricing":{"prompt":"0","completion":"0"}},
			{"name":"OpenAI: gpt-oss-20b (Free)","canonical_slug":"openai/gpt-oss-20b","pricing":{"prompt":"0","completion":"0"}}
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

// TestARefreshUpdatesAFreeListingLikeAnyOtherModel pins the decision that this
// app treats every model OpenRouter returns alike. A free listing is a real
// endpoint offered under a name of its own, and what a model is called decides
// nothing about whether a refresh writes onto its row. All three are seeded
// first so the refresh meets rows that already exist, which is the path a name
// could have been discriminated against on.
func TestARefreshUpdatesAFreeListingLikeAnyOtherModel(t *testing.T) {
	migratedDB(t)
	captureLog(t)
	freeListingCatalogServer(t)

	for _, name := range []string{
		"Meta: Llama 3.3 70B Instruct",
		"Meta: Llama 3.3 70B Instruct (free)",
		"OpenAI: gpt-oss-20b (Free)",
	} {
		if _, err := store.UpsertAPIModel(map[string]interface{}{"name": name}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	for _, name := range []string{
		"Meta: Llama 3.3 70B Instruct",
		"Meta: Llama 3.3 70B Instruct (free)",
		"OpenAI: gpt-oss-20b (Free)",
	} {
		if !store.ModelExists(name) {
			t.Errorf("%q was dropped by the refresh", name)
		}
	}

	// And it is reachable, which is the point of updating it: the API's row
	// count matches the table's, none added and none dropped.
	rows, err := processModels()
	if err != nil {
		t.Fatalf("processModels: %v", err)
	}
	served := filtered(t, rows, url.Values{"include_hidden": {"1"}})
	stored, err := dbcore.CountModels()
	if err != nil {
		t.Fatalf("CountModels: %v", err)
	}
	if len(served) != stored || stored != 3 {
		t.Errorf("the API answered with %d rows and the table holds %d, want 3 of each", len(served), stored)
	}
}
