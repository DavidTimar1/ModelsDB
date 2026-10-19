package catalog

import (
	"fmt"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSleeps replaces the wait with a recorder, so no test sleeps for real, and
// returns a reader of the waits asked for so far.
func fakeSleeps(t *testing.T) func() []time.Duration {
	t.Helper()
	var mu sync.Mutex
	var waits []time.Duration
	orig := sleepFor
	sleepFor = func(d time.Duration) {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
	}
	t.Cleanup(func() { sleepFor = orig })
	return func() []time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Duration{}, waits...)
	}
}

func secs(n ...int) []time.Duration {
	out := make([]time.Duration, len(n))
	for i, s := range n {
		out[i] = time.Duration(s) * time.Second
	}
	return out
}

func sameWaits(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// retryWait obeys Retry-After in seconds or as a date, caps it, and otherwise
// doubles from the first wait up to the cap.
func TestRetryWait(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	withHeader := func(v string) *http.Response {
		return &http.Response{Header: http.Header{"Retry-After": {v}}}
	}
	none := &http.Response{Header: http.Header{}}
	for _, c := range []struct {
		name    string
		resp    *http.Response
		attempt int
		want    time.Duration
	}{
		{"seconds", withHeader("7"), 0, 7 * time.Second},
		{"seconds over the cap", withHeader("600"), 0, 60 * time.Second},
		{"zero seconds", withHeader("0"), 3, 0},
		{"date 9 s ahead", withHeader(now.Add(9 * time.Second).Format(http.TimeFormat)), 0, 9 * time.Second},
		{"date in the past", withHeader(now.Add(-time.Hour).Format(http.TimeFormat)), 0, 0},
		{"date over the cap", withHeader(now.Add(time.Hour).Format(http.TimeFormat)), 0, 60 * time.Second},
		{"garbage falls back to back-off", withHeader("soon"), 1, 4 * time.Second},
		{"first back-off", none, 0, 2 * time.Second},
		{"second back-off", none, 1, 4 * time.Second},
		{"fifth back-off", none, 4, 32 * time.Second},
		{"sixth back-off is capped", none, 5, 60 * time.Second},
		{"far attempt is capped", none, 40, 60 * time.Second},
	} {
		if got := retryWait(c.resp, c.attempt, now); got != c.want {
			t.Errorf("%s: retryWait = %v, want %v", c.name, got, c.want)
		}
	}
}

// answersInOrder serves the given statuses one request at a time, then 200,
// and counts the requests. retryAfter, when set, rides on every non-200.
func answersInOrder(t *testing.T, retryAfter string, statuses ...int) (*httptest.Server, *int32) {
	t.Helper()
	var n int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(atomic.AddInt32(&n, 1)) - 1
		status := http.StatusOK
		if i < len(statuses) {
			status = statuses[i]
		}
		if status != http.StatusOK && retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	return server, &n
}

func getStatus(t *testing.T, url string, pc *pacer) int {
	t.Helper()
	resp, err := getWithRetry(shared.NewHTTPClient(5*time.Second), url, pc)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// A 429 with Retry-After waits that long and sends the request again.
func TestAnswer429WithRetryAfterWaitsAsToldAndSucceeds(t *testing.T) {
	waits := fakeSleeps(t)
	server, n := answersInOrder(t, "3", 429, 429)
	if got := getStatus(t, server.URL, nil); got != 200 {
		t.Fatalf("status = %d, want 200 after two retries", got)
	}
	if !sameWaits(waits(), secs(3, 3)) || *n != 3 {
		t.Errorf("waits = %v after %d requests, want [3s 3s] after 3", waits(), *n)
	}
}

// A 503 without Retry-After backs off 2 s, 4 s, 8 s.
func TestAnswer503WithoutRetryAfterBacksOffExponentially(t *testing.T) {
	waits := fakeSleeps(t)
	server, _ := answersInOrder(t, "", 503, 503, 503)
	if got := getStatus(t, server.URL, nil); got != 200 {
		t.Fatalf("status = %d, want 200", got)
	}
	if !sameWaits(waits(), secs(2, 4, 8)) {
		t.Errorf("waits = %v, want [2s 4s 8s]", waits())
	}
}

// A request still refused after five retries is handed back as the 429, and
// the server saw six requests: the first and five retries.
func TestARequestRefusedForeverGivesUpAfterFiveRetries(t *testing.T) {
	waits := fakeSleeps(t)
	server, n := answersInOrder(t, "", 429, 429, 429, 429, 429, 429, 429, 429)
	if got := getStatus(t, server.URL, nil); got != 429 {
		t.Fatalf("status = %d, want the final 429", got)
	}
	if *n != 6 || !sameWaits(waits(), secs(2, 4, 8, 16, 32)) {
		t.Errorf("%d requests and waits %v, want 6 and [2s 4s 8s 16s 32s]", *n, waits())
	}
}

// Other failures are not retried: a 500 or 404 is the caller's to count.
func TestOtherStatusesAreNotRetried(t *testing.T) {
	waits := fakeSleeps(t)
	for _, status := range []int{404, 500, 502} {
		server, n := answersInOrder(t, "", status)
		if got := getStatus(t, server.URL, nil); got != status || *n != 1 {
			t.Errorf("status %d: got %d after %d requests, want one request", status, got, *n)
		}
	}
	if len(waits()) != 0 {
		t.Errorf("waited %v, want no wait", waits())
	}
}

// A paced run pauses 0.8 to 1.2 s before every request but its first; an
// unpaced run never pauses.
func TestPacerPausesBetweenRequestsOnly(t *testing.T) {
	waits := fakeSleeps(t)
	server, _ := answersInOrder(t, "")
	pc := newPacer()
	for i := 0; i < 40; i++ {
		getStatus(t, server.URL, pc)
	}
	got := waits()
	if len(got) != 39 {
		t.Fatalf("%d pauses for 40 requests, want 39", len(got))
	}
	distinct := map[time.Duration]bool{}
	for _, d := range got {
		if d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("pause %v is outside 0.8 to 1.2 s", d)
		}
		distinct[d] = true
	}
	if len(distinct) < 5 {
		t.Errorf("only %d distinct pauses in 39, want jitter", len(distinct))
	}
	var unpaced *pacer
	before := len(waits())
	for i := 0; i < 5; i++ {
		getStatus(t, server.URL, unpaced)
	}
	if len(waits()) != before {
		t.Error("an unpaced run paused")
	}
}

func TestWorkersAreOneWhenPacedAndEightOtherwise(t *testing.T) {
	var unpaced *pacer
	if unpaced.workers() != 8 || newPacer().workers() != 1 {
		t.Errorf("workers = %d unpaced and %d paced, want 8 and 1", unpaced.workers(), newPacer().workers())
	}
}

// perModelServer serves the provider map and one endpoint list per model, and
// records the most requests it ever had in flight at once. The status function
// decides each per-model answer from the request count of that model.
func perModelServer(t *testing.T, status func(path string, tries int) int) (maxInFlight *int32, tries func(path string) int) {
	t.Helper()
	var inFlight, peak int32
	var mu sync.Mutex
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/all-providers") {
			w.Write([]byte(`{"data":[{"slug":"p","dataPolicy":{"retainsPrompts":false}}]}`))
			return
		}
		now := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			old := atomic.LoadInt32(&peak)
			if now <= old || atomic.CompareAndSwapInt32(&peak, old, now) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		seen[r.URL.Path]++
		n := seen[r.URL.Path]
		mu.Unlock()
		if s := status(r.URL.Path, n); s != http.StatusOK {
			w.WriteHeader(s)
			return
		}
		w.Write([]byte(`{"data":{"endpoints":[{"tag":"p/x"}]}}`))
	}))
	t.Cleanup(server.Close)
	origP, origE := AllProvidersURL, ModelEndpointsBase
	AllProvidersURL, ModelEndpointsBase = server.URL+"/all-providers", server.URL+"/ep"
	t.Cleanup(func() { AllProvidersURL, ModelEndpointsBase = origP, origE })
	return &peak, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return seen[path]
	}
}

func seedOpenRouterModels(t *testing.T, count int) map[string]bool {
	t.Helper()
	catalogTestDB(t)
	listed := map[string]bool{}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("v/m%02d", i)
		if _, err := store.UpsertAPIModel(NormalizeModel(map[string]interface{}{"name": "M" + id, "id": id})); err != nil {
			t.Fatal(err)
		}
		listed[id] = true
	}
	return listed
}

// The button and start-up send eight per-model requests at a time; a paced run
// sends one at a time.
func TestPerModelPassKeepsEightWorkersUnpacedAndOneWhenPaced(t *testing.T) {
	fakeSleeps(t)
	listed := seedOpenRouterModels(t, 24)
	peak, _ := perModelServer(t, func(string, int) int { return 200 })
	if err := DeriveZDR(listed, nil); err != nil {
		t.Fatal(err)
	}
	if *peak < 4 || *peak > 8 {
		t.Errorf("unpaced peak in flight = %d, want several and at most 8", *peak)
	}
	*peak = 0
	if err := DeriveZDR(listed, newPacer()); err != nil {
		t.Fatal(err)
	}
	if *peak != 1 {
		t.Errorf("paced peak in flight = %d, want 1", *peak)
	}
}

// A model whose endpoint list is refused with 429 for a while is retried and
// still gets its value; a pass whose requests are all refused for good is a
// failed pass.
func TestPerModelPassRetriesA429AndFailsWhenEverythingIsRefused(t *testing.T) {
	waits := fakeSleeps(t)
	listed := seedOpenRouterModels(t, 4)
	_, tries := perModelServer(t, func(path string, n int) int {
		if strings.Contains(path, "m01") && n <= 2 {
			return 429
		}
		return 200
	})
	if err := DeriveZDR(listed, nil); err != nil {
		t.Fatalf("a pass with one briefly refused model returned %v, want nil", err)
	}
	if got := tries("/ep/v/m01/endpoints"); got != 3 {
		t.Errorf("the refused model was asked %d times, want 3", got)
	}
	if !sameWaits(waits(), secs(2, 4)) {
		t.Errorf("waits = %v, want [2s 4s]", waits())
	}
	cells, _ := store.ModelCells("Mv/m01")
	if cells["zdr"] == "" {
		t.Error("the retried model got no zdr value")
	}

	perModelServer(t, func(string, int) int { return 429 })
	if err := DeriveZDR(listed, nil); err == nil {
		t.Error("a pass refused everywhere returned nil, want a failed pass")
	}
}
