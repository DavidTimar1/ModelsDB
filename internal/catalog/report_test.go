package catalog

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

// reportServer serves a one-model feed whose id-less record keeps the run
// offline (neither per-model pass has a job to fetch), answers each sibling path
// with the status given for it in statuses (200 when absent), and counts the
// requests each path received.
func reportServer(t *testing.T, statuses map[string]int) *map[string]int {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	mux := http.NewServeMux()
	for _, path := range []string{"/models", "/videos", "/images", "/all-providers", "/stats", "/eu"} {
		path := path
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[path]++
			mu.Unlock()
			if code, ok := statuses[path]; ok && code != http.StatusOK {
				http.Error(w, "refused", code)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if path == "/models" {
				w.Write([]byte(`{"data":[{"name":"Report: One","canonical_slug":"report/one"}]}`))
				return
			}
			w.Write([]byte(`{"data":[]}`))
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	origAPI, origVideo, origImage, origProviders, origStats, origEU := ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL, StatsEndpointBase, EURoutingURL
	ApiUrl = server.URL + "/models"
	VideoModelsURL = server.URL + "/videos"
	ImageModelsURL = server.URL + "/images"
	AllProvidersURL = server.URL + "/all-providers"
	StatsEndpointBase = server.URL + "/stats"
	EURoutingURL = server.URL + "/eu"
	t.Cleanup(func() {
		ApiUrl, VideoModelsURL, ImageModelsURL, AllProvidersURL, StatsEndpointBase, EURoutingURL = origAPI, origVideo, origImage, origProviders, origStats, origEU
	})
	return &hits
}

// A pass that did not do its job is named in the report, in the words the page
// shows, and the refresh still stores the feed.
func TestFetchModelsWithNamesEveryPassThatFailed(t *testing.T) {
	catalogTestDB(t)
	reportServer(t, map[string]int{
		"/videos": http.StatusInternalServerError, "/all-providers": http.StatusInternalServerError,
	})

	report, err := FetchModelsWith(Options{})
	if err != nil {
		t.Fatalf("FetchModelsWith: %v", err)
	}
	if report.Created != 1 {
		t.Errorf("Created = %d, want 1: a failed pass must not stop the feed being stored", report.Created)
	}
	// The EU list is empty in this server, which the pass refuses as implausible.
	want := []string{PartListings, PartZDR, PartEU}
	if !reflect.DeepEqual(report.FailedParts, want) {
		t.Errorf("FailedParts = %v, want %v", report.FailedParts, want)
	}
}

// Leaving the per-model passes out requests neither of their endpoints, and a
// pass that was left out is not reported as a failed one.
func TestFetchModelsWithCanLeaveOutThePerModelPasses(t *testing.T) {
	catalogTestDB(t)
	hits := reportServer(t, map[string]int{"/all-providers": http.StatusInternalServerError})

	report, err := FetchModelsWith(Options{SkipPerModelPasses: true})
	if err != nil {
		t.Fatalf("FetchModelsWith: %v", err)
	}
	if n := (*hits)["/all-providers"]; n != 0 {
		t.Errorf("the provider policy endpoint was asked %d time(s), want 0", n)
	}
	if n := (*hits)["/stats"]; n != 0 {
		t.Errorf("the stats endpoint was asked %d time(s), want 0", n)
	}
	for _, part := range report.FailedParts {
		if part == PartZDR || part == PartSpeed {
			t.Errorf("FailedParts holds %q although that pass was left out", part)
		}
	}
}

// A refresh whose passes all work reports none failed.
func TestFetchModelsWithReportsNothingFailedWhenEveryPassWorked(t *testing.T) {
	catalogTestDB(t)
	dedicatedListingServer(t, `{"data":[
		{"name":"Report: Held","canonical_slug":"report/held","id":"report/held"},
		{"name":"Report: B","canonical_slug":"report/b","id":"report/b"},
		{"name":"Report: C","canonical_slug":"report/c","id":"report/c"}
	]}`, `{"data":[]}`, `{"data":[]}`)
	// An EU list that is a plausible region filter: it shares an id with the feed
	// and is under 90% of its size.
	euServer(t, 200, `{"data":[{"id":"report/held"}]}`)

	report, err := FetchModelsWith(Options{SkipPerModelPasses: true})
	if err != nil {
		t.Fatalf("FetchModelsWith: %v", err)
	}
	if len(report.FailedParts) != 0 {
		t.Errorf("FailedParts = %v, want none", report.FailedParts)
	}
}

// The per-model passes count as failed only when at least half their requests
// did, so the stray timeout a few hundred requests always draw does not make a
// day look broken.
func TestMostFetchesFailed(t *testing.T) {
	for _, c := range []struct {
		failures, total int
		want            bool
	}{
		{0, 0, false}, {0, 686, false}, {3, 686, false}, {342, 686, false},
		{343, 686, true}, {686, 686, true}, {1, 2, true}, {1, 3, false},
	} {
		if got := mostFetchesFailed(c.failures, c.total); got != c.want {
			t.Errorf("mostFetchesFailed(%d, %d) = %v, want %v", c.failures, c.total, got, c.want)
		}
	}
}

// DeriveEURouting says whether it wrote anything.
func TestDeriveEURoutingReturnsWhyItWroteNothing(t *testing.T) {
	listed := euRows(t)
	euServer(t, 200, `{"data":[{"id":"v/a"}]}`)
	if err := DeriveEURouting(listed, nil); err != nil {
		t.Errorf("a good answer returned %v, want nil", err)
	}
	euServer(t, 500, `refused`)
	if err := DeriveEURouting(listed, nil); err == nil {
		t.Error("a refused answer returned nil, want the reason nothing was written")
	}
}

// DeriveZDR says it did not do its job when the provider policies cannot be read.
func TestDeriveZDRReturnsAnErrorWhenThePoliciesCannotBeRead(t *testing.T) {
	catalogTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "refused", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	orig := AllProvidersURL
	AllProvidersURL = server.URL
	t.Cleanup(func() { AllProvidersURL = orig })

	if err := DeriveZDR(map[string]bool{"v/a": true}, nil); err == nil {
		t.Error("DeriveZDR returned nil with an unreadable policy map, want an error")
	}
}
