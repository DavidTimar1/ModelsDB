package catalog

import (
	"bytes"
	"log"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dedicatedListingServer serves the general listing, the video listing and
// the image listing off one httptest.Server, each body given verbatim, and
// points ApiUrl, VideoModelsURL, ImageModelsURL and AllProvidersURL at it -
// the last so DeriveZDR's provider-policy fetch, which every FetchModels call
// makes, never reaches the real network either, and StatsEndpointBase likewise
// for the throughput pass. Every var is restored on cleanup.
func dedicatedListingServer(t *testing.T, general, video, image string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(general))
	})
	mux.HandleFunc("/videos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(video))
	})
	mux.HandleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(image))
	})
	mux.HandleFunc("/all-providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/eu", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	origAPI, origVideo, origImage, origProviders, origStats := ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL, StatsEndpointBase
	ApiUrl = server.URL + "/models"
	origEU := EURoutingURL
	t.Cleanup(func() { EURoutingURL = origEU })
	EURoutingURL = server.URL + "/eu"
	VideoModelsURL = server.URL + "/videos"
	ImageModelsURL = server.URL + "/images"
	AllProvidersURL = server.URL + "/all-providers"
	StatsEndpointBase = server.URL + "/stats"
	t.Cleanup(func() {
		ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL, StatsEndpointBase = origAPI, origVideo, origImage, origProviders, origStats
	})
}

// TestDedicatedListingsJoinByIDAndNeverRewriteTheGeneralListing covers C2,
// C3 and C6 together: the video and image listing's records join onto the
// rows the general listing made, by id; a record whose id matches no row is
// skipped and creates nothing; and a value the general listing already wrote
// (description) is never replaced by the shorter, truncated one a dedicated
// listing carries for the same model.
func TestDedicatedListingsJoinByIDAndNeverRewriteTheGeneralListing(t *testing.T) {
	catalogTestDB(t)
	dedicatedListingServer(t,
		`{"data":[
			{"name":"ByteDance: Seedance 2.5","canonical_slug":"bytedance/seedance-2.5","id":"bytedance/seedance-2.5",
			 "description":"The full, untruncated description the general listing carries."},
			{"name":"OpenAI: GPT Image 1","canonical_slug":"openai/gpt-image-1","id":"openai/gpt-image-1",
			 "description":"An image model, from the general listing."}
		]}`,
		`{"data":[
			{"id":"bytedance/seedance-2.5","name":"whatever the video listing calls it",
			 "description":"The full, untr...",
			 "supported_frame_images":["first_frame","last_frame"],
			 "pricing_skus":{"duration_seconds":"5","cents_per_second_output":"0.02"}},
			{"id":"nomatch/no-such-model","name":"a video model no general record carries",
			 "supported_frame_images":["first_frame"]}
		]}`,
		`{"data":[
			{"id":"openai/gpt-image-1","name":"whatever the image listing calls it",
			 "supported_parameters":{"input_references":{"type":"range","min":0,"max":16}}}
		]}`,
	)

	if _, _, _, err := FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}

	// C6: exactly the two rows the general listing named, none for the
	// unmatched video record.
	names, err := heldNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || !names["ByteDance: Seedance 2.5"] || !names["OpenAI: GPT Image 1"] {
		t.Fatalf("held names = %v, want exactly the two general-listing rows", names)
	}

	// C3: the general listing's description stands, never the video listing's
	// truncated copy of the same fact.
	cells, err := store.ModelCells("ByteDance: Seedance 2.5")
	if err != nil {
		t.Fatal(err)
	}
	if got := cells["description"]; got != "The full, untruncated description the general listing carries." {
		t.Errorf("description = %q, want the general listing's own text, not the video listing's", got)
	}

	// C7: frame_images, joined by id.
	if got := cells["frame_images"]; got != `["first_frame","last_frame"]` {
		t.Errorf("frame_images = %q, want [\"first_frame\",\"last_frame\"]", got)
	}

	// C12: pricing_skus is one list column, one member per key in feed order,
	// "key: price" - never pricing_skus_duration_seconds or the like.
	if got := cells["pricing_skus"]; got != `["duration_seconds: 5","cents_per_second_output: 0.02"]` {
		t.Errorf("pricing_skus = %q, want the two SKUs as one member each in feed order", got)
	}
	if _, err := store.ColumnByKey("pricing_skus_duration_seconds"); err == nil {
		t.Errorf("a per-key pricing_skus_duration_seconds column was made, want pricing_skus alone")
	}

	// C8, C9: the image listing's structured reference-image ceiling.
	imgCells, err := store.ModelCells("OpenAI: GPT Image 1")
	if err != nil {
		t.Fatal(err)
	}
	if got := imgCells["max_reference_images"]; got != "16" {
		t.Errorf("max_reference_images = %q, want 16", got)
	}
	if got := imgCells["reference_images"]; got != "yes" {
		t.Errorf("reference_images = %q, want yes", got)
	}

	// C10: the other two input_references sub-keys go through the door as
	// ordinary columns, unmapped.
	minCol, err := store.ColumnByKey("supported_parameters_input_references_min")
	if err != nil {
		t.Fatalf("supported_parameters.input_references.min made no column: %v", err)
	}
	if !minCol.Sealed || minCol.Visible {
		t.Errorf("supported_parameters_input_references_min = %+v, want hidden and sealed like any door column", minCol)
	}

	// C7, C8: mapped to shipped columns, so the door made no second, path-keyed
	// column for either.
	if _, err := store.ColumnByKey("supported_frame_images"); err == nil {
		t.Errorf("the door made its own column for supported_frame_images, want it mapped to frame_images alone")
	}
	if _, err := store.ColumnByKey("supported_parameters_input_references_max"); err == nil {
		t.Errorf("the door made its own column for input_references.max, want it mapped to max_reference_images alone")
	}
}

// TestADedicatedListingThatFailsCostsOnlyItself covers C4: a listing that
// answers with a bad status is one phase failure, logged, and the refresh -
// the general listing's result, and the OTHER dedicated listing - still goes
// through untouched.
func TestADedicatedListingThatFailsCostsOnlyItself(t *testing.T) {
	catalogTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"name":"OpenAI: GPT Image 1","canonical_slug":"openai/gpt-image-1","id":"openai/gpt-image-1"}]}`))
	})
	mux.HandleFunc("/videos", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"openai/gpt-image-1","name":"x",
			"supported_parameters":{"input_references":{"type":"range","min":0,"max":4}}}]}`))
	})
	mux.HandleFunc("/all-providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/eu", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	origAPI, origVideo, origImage, origProviders := ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL
	ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL = server.URL+"/models", server.URL+"/videos", server.URL+"/images", server.URL+"/all-providers"
	origEU := EURoutingURL
	t.Cleanup(func() { EURoutingURL = origEU })
	EURoutingURL = server.URL + "/eu"
	t.Cleanup(func() {
		ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL = origAPI, origVideo, origImage, origProviders
	})

	var buf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(origOut) })

	if _, _, _, err := FetchModels(); err != nil {
		t.Fatalf("a failed listing must not fail the refresh: %v", err)
	}
	if !strings.Contains(buf.String(), "the video listing") {
		t.Errorf("the log does not name the video listing as the failed one: %s", buf.String())
	}

	cells, err := store.ModelCells("OpenAI: GPT Image 1")
	if err != nil {
		t.Fatal(err)
	}
	if cells["max_reference_images"] != "4" {
		t.Errorf("the image listing's own facts were lost because the video listing failed: %v", cells)
	}
}
