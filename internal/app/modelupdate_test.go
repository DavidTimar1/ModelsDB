package app

import (
	"encoding/json"
	"modelsdb/internal/catalog"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// modelUpdateFeed is a canned OpenRouter that can be told to fail and to hold a
// request open, so a test can look at the app while a refresh is in flight.
type modelUpdateFeed struct {
	mu      sync.Mutex
	failing bool
	// noIDs serves the three models without an id, which leaves the per-model
	// passes (whose request a canned feed cannot intercept) with nothing to fetch,
	// so a run with every pass on stays offline.
	noIDs bool
	// hold, when non-nil, is waited on by the feed request; arrived is sent to
	// first so the test knows the refresh is inside it.
	hold    chan struct{}
	arrived chan struct{}
}

func (f *modelUpdateFeed) setFailing(v bool) {
	f.mu.Lock()
	f.failing = v
	f.mu.Unlock()
}

// modelUpdateSetup gives a test a migrated database, its own cache dir for the
// record of the last run, the scheduled run's per-model passes switched off
// (their per-model request is the one a canned feed cannot intercept), and a
// canned feed of three models with an EU list that is a plausible region filter.
func modelUpdateSetup(t *testing.T) *modelUpdateFeed {
	t.Helper()
	migratedDB(t)
	cacheDir := t.TempDir()
	origCache, origFile := paths.CacheDir, paths.ModelUpdateCacheFile
	paths.CacheDir = cacheDir
	paths.ModelUpdateCacheFile = filepath.Join(cacheDir, "model-update.json")
	modelUpdateMu.Lock()
	modelUpdateLast = modelUpdateRun{}
	modelUpdateMu.Unlock()
	origOpts := scheduledRunOptions
	scheduledRunOptions = catalog.Options{SkipPerModelPasses: true}

	feed := &modelUpdateFeed{}
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		feed.mu.Lock()
		failing, hold, arrived, noIDs := feed.failing, feed.hold, feed.arrived, feed.noIDs
		feed.mu.Unlock()
		if arrived != nil {
			arrived <- struct{}{}
		}
		if hold != nil {
			<-hold
		}
		if failing {
			http.Error(w, "the feed is down", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if noIDs {
			w.Write([]byte(`{"data":[
				{"name":"Daily: A","canonical_slug":"daily/a"},
				{"name":"Daily: B","canonical_slug":"daily/b"},
				{"name":"Daily: C","canonical_slug":"daily/c"}
			]}`))
			return
		}
		w.Write([]byte(`{"data":[
			{"name":"Daily: A","canonical_slug":"daily/a","id":"daily/a"},
			{"name":"Daily: B","canonical_slug":"daily/b","id":"daily/b"},
			{"name":"Daily: C","canonical_slug":"daily/c","id":"daily/c"}
		]}`))
	})
	mux.HandleFunc("/eu", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"daily/a"}]}`))
	})
	for _, path := range []string{"/videos", "/images", "/all-providers"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[]}`))
		})
	}
	server := httptest.NewServer(mux)

	origAPI, origProviders := catalog.ApiUrl, catalog.AllProvidersURL
	origVideo, origImage, origEU := catalog.VideoModelsURL, catalog.ImageModelsURL, catalog.EURoutingURL
	catalog.ApiUrl = server.URL + "/models"
	catalog.EURoutingURL = server.URL + "/eu"
	catalog.AllProvidersURL = server.URL + "/all-providers"
	catalog.VideoModelsURL = server.URL + "/videos"
	catalog.ImageModelsURL = server.URL + "/images"

	t.Cleanup(func() {
		// A refresh still running would write into the next test's database.
		waitForRefreshToStop(t)
		server.Close()
		catalog.ApiUrl, catalog.AllProvidersURL = origAPI, origProviders
		catalog.VideoModelsURL, catalog.ImageModelsURL, catalog.EURoutingURL = origVideo, origImage, origEU
		scheduledRunOptions = origOpts
		paths.CacheDir, paths.ModelUpdateCacheFile = origCache, origFile
	})
	return feed
}

// waitForRefreshToStop returns once the process-wide refresh controller is idle.
func waitForRefreshToStop(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for refresh.snapshot().Running {
		if time.Now().After(deadline) {
			t.Fatal("the refresh did not finish within 15 seconds")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// switchDailyUpdate sets the Info dialog's switch the way the page stores it.
func switchDailyUpdate(t *testing.T, on bool) {
	t.Helper()
	value := json.RawMessage("false")
	if on {
		value = json.RawMessage("true")
	}
	if err := store.WriteSettings(map[string]json.RawMessage{dailyUpdateSettingKey: value}, nil); err != nil {
		t.Fatalf("store the switch: %v", err)
	}
}

// aDayAndAMinuteAfterNow is a clock reading at which any run recorded now is due.
func aDayAndAMinuteAfterNow() time.Time {
	return time.Now().Add(24*time.Hour + time.Minute)
}

func TestModelUpdateDue(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	stamp := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	for _, c := range []struct {
		name string
		last modelUpdateRun
		want bool
	}{
		{"never run", modelUpdateRun{}, true},
		{"ok a minute ago", modelUpdateRun{Outcome: modelUpdateOK, FinishedAt: stamp(time.Minute)}, false},
		{"ok 23 hours ago", modelUpdateRun{Outcome: modelUpdateOK, FinishedAt: stamp(23 * time.Hour)}, false},
		{"ok exactly a day ago", modelUpdateRun{Outcome: modelUpdateOK, FinishedAt: stamp(24 * time.Hour)}, true},
		{"ok two days ago", modelUpdateRun{Outcome: modelUpdateOK, FinishedAt: stamp(48 * time.Hour)}, true},
		{"partly failed an hour ago counts as a run", modelUpdateRun{Outcome: modelUpdatePartial, FinishedAt: stamp(time.Hour)}, false},
		{"failed a minute ago is retried at the next wake", modelUpdateRun{Outcome: modelUpdateFailed, FinishedAt: stamp(time.Minute)}, true},
		{"stamp in the future from a clock moved back", modelUpdateRun{Outcome: modelUpdateOK, FinishedAt: now.Add(72 * time.Hour).Format(time.RFC3339)}, true},
		{"stamp that does not parse", modelUpdateRun{Outcome: modelUpdateOK, FinishedAt: "yesterday"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := modelUpdateDue(c.last, now); got != c.want {
				t.Errorf("modelUpdateDue = %v, want %v", got, c.want)
			}
		})
	}
}

// With the switch off nothing starts and nothing is written, however overdue.
func TestScheduledModelUpdateDoesNothingWhenTheSwitchIsOff(t *testing.T) {
	modelUpdateSetup(t)
	switchDailyUpdate(t, false)

	if runScheduledModelUpdate(aDayAndAMinuteAfterNow()) {
		t.Fatal("a run started with the switch off")
	}
	if refresh.snapshot().Running {
		t.Error("the refresh controller is running with the switch off")
	}
	if _, err := os.Stat(paths.ModelUpdateCacheFile); !os.IsNotExist(err) {
		t.Errorf("a record was written with the switch off (stat err = %v)", err)
	}
}

// The switch is on until it is stored as off: with no stored row at all, a run
// that is due starts.
func TestScheduledModelUpdateIsOnWhenNothingWasEverStored(t *testing.T) {
	modelUpdateSetup(t)

	if !dailyUpdateEnabled() {
		t.Fatal("the switch reads as off on a database that never stored it")
	}
	if !runScheduledModelUpdate(time.Now()) {
		t.Error("no run started on an install that never touched the switch")
	}
	waitForRefreshToStop(t)
}

// With the switch on and a run due, a scheduled run goes through the same
// controller, is marked scheduled while it runs, stores the feed, and is
// recorded. It writes nothing but the database and the record.
func TestScheduledModelUpdateRunsWhenDueAndRecordsIt(t *testing.T) {
	feed := modelUpdateSetup(t)
	switchDailyUpdate(t, true)
	feed.hold, feed.arrived = make(chan struct{}), make(chan struct{}, 1)

	if !runScheduledModelUpdate(time.Now()) {
		t.Fatal("no run started although none had ever finished")
	}
	<-feed.arrived
	if st := refresh.snapshot(); !st.Running || !st.Scheduled {
		t.Errorf("while the scheduled run is in flight: running=%v scheduled=%v, want both true", st.Running, st.Scheduled)
	}
	close(feed.hold)
	waitForRefreshToStop(t)

	st := refresh.snapshot()
	if st.Scheduled {
		t.Error("Scheduled still true after the run finished")
	}
	if st.Added != 3 {
		t.Errorf("added = %d, want 3", st.Added)
	}
	last := st.LastRun
	if last.Seq != 1 || last.Outcome != modelUpdateOK || !last.Scheduled || last.FinishedAt == "" {
		t.Errorf("last run = %+v, want seq 1, ok, scheduled, with a finish time", last)
	}
	if modelUpdateDue(last, time.Now()) {
		t.Error("a run that has just finished is already due again")
	}
	assertOnlyTheDatabaseInDataDir(t, "a scheduled model update")
}

// A run that is under way is the run: a second scheduled start and a button
// press are both refused (the button gets its 409), and turning the switch off
// does not cancel the run in flight but stops the next one being started.
func TestScheduledModelUpdateIsSingleFlightAndSwitchingOffLetsItFinish(t *testing.T) {
	feed := modelUpdateSetup(t)
	switchDailyUpdate(t, true)
	feed.hold, feed.arrived = make(chan struct{}), make(chan struct{}, 1)

	if !runScheduledModelUpdate(time.Now()) {
		t.Fatal("the scheduled run did not start")
	}
	<-feed.arrived
	if runScheduledModelUpdate(aDayAndAMinuteAfterNow()) {
		t.Error("a second scheduled run started while one was in flight")
	}
	if refresh.start(false) {
		t.Error("the button started a second refresh while a scheduled one was in flight")
	}
	rec := httptest.NewRecorder()
	handleUpdate(rec, httptest.NewRequest(http.MethodPost, "/api/update", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("POST /api/update during a scheduled run answered %d, want 409", rec.Code)
	}

	switchDailyUpdate(t, false)
	close(feed.hold)
	waitForRefreshToStop(t)

	if got := currentModelUpdateRun(); got.Seq != 1 || got.Outcome != modelUpdateOK {
		t.Errorf("the run switched off mid-flight was not recorded as finished: %+v", got)
	}
	if runScheduledModelUpdate(aDayAndAMinuteAfterNow()) {
		t.Error("a run started after the switch was turned off")
	}
}

// A feed failure is recorded with its reason, is due again at the very next
// wake, and the record is read back after a restart.
func TestAFailedScheduledRunIsRecordedRetriedAndSurvivesARestart(t *testing.T) {
	feed := modelUpdateSetup(t)
	switchDailyUpdate(t, true)
	feed.setFailing(true)

	if !runScheduledModelUpdate(time.Now()) {
		t.Fatal("the scheduled run did not start")
	}
	waitForRefreshToStop(t)

	last := currentModelUpdateRun()
	if last.Outcome != modelUpdateFailed || last.Error == "" || last.Seq != 1 {
		t.Fatalf("last run = %+v, want a failed run with a reason", last)
	}
	if !modelUpdateDue(last, time.Now()) {
		t.Error("a failed run is not due at the next wake")
	}

	// A restart: the in-memory record is gone, the file is read back.
	modelUpdateMu.Lock()
	modelUpdateLast = modelUpdateRun{}
	modelUpdateMu.Unlock()
	loadModelUpdateCache()
	if got := currentModelUpdateRun(); got.Seq != last.Seq || got.Outcome != modelUpdateFailed || got.Error != last.Error {
		t.Errorf("after a restart the record is %+v, want %+v", got, last)
	}

	// The retry, with the feed back, succeeds, takes the failure down and counts on.
	feed.setFailing(false)
	if !runScheduledModelUpdate(time.Now()) {
		t.Fatal("the retry did not start")
	}
	waitForRefreshToStop(t)
	if got := currentModelUpdateRun(); got.Outcome != modelUpdateOK || got.Error != "" || got.Seq != 2 {
		t.Errorf("after the retry the record is %+v, want ok, no error, seq 2", got)
	}
}

// A run whose feed was stored but whose later pass failed counts as a run: it is
// named in the record and is not retried before the day is out.
func TestAPartlyFailedRunNamesThePartAndCountsAsARun(t *testing.T) {
	modelUpdateSetup(t)

	recordModelUpdateRun(catalog.Report{FailedParts: []string{catalog.PartEU, catalog.PartSpeed}}, nil, true)
	got := currentModelUpdateRun()
	if got.Outcome != modelUpdatePartial || len(got.FailedParts) != 2 || got.FailedParts[0] != "EU routing" || got.FailedParts[1] != "throughput" {
		t.Errorf("record = %+v, want a partial run naming EU routing and throughput", got)
	}
	if modelUpdateDue(got, time.Now().Add(time.Hour)) {
		t.Error("a partly failed run is retried within the day")
	}
}

// The server's own start-up refresh is a run: it is recorded, so no scheduled
// run follows it seconds later, and a failure there is recorded too.
func TestTheStartupUpdateIsRecordedAsARun(t *testing.T) {
	feed := modelUpdateSetup(t)
	feed.noIDs = true
	switchDailyUpdate(t, true)

	runStartupModelUpdate()
	got := currentModelUpdateRun()
	if got.Seq != 1 || got.Scheduled || got.Outcome == "" {
		t.Fatalf("after the start-up update the record is %+v, want seq 1, not scheduled, with an outcome", got)
	}
	if runScheduledModelUpdate(time.Now().Add(time.Hour)) {
		t.Error("a scheduled run started an hour after the start-up update")
	}

	feed.setFailing(true)
	runStartupModelUpdate()
	if got := currentModelUpdateRun(); got.Seq != 2 || got.Outcome != modelUpdateFailed {
		t.Errorf("after a failed start-up update the record is %+v, want seq 2, failed", got)
	}
}

// A run the button starts is a run too: it moves the record and not the
// scheduler's reference, because there is only one reference and it is the last
// run of any kind.
func TestAButtonRunIsRecordedAsARun(t *testing.T) {
	feed := modelUpdateSetup(t)
	feed.noIDs = true

	if !refresh.start(false) {
		t.Fatal("the button could not start a refresh")
	}
	waitForRefreshToStop(t)

	got := currentModelUpdateRun()
	if got.Seq != 1 || got.Scheduled {
		t.Errorf("after a button run the record is %+v, want seq 1, not scheduled", got)
	}
	switchDailyUpdate(t, true)
	if runScheduledModelUpdate(time.Now().Add(time.Hour)) {
		t.Error("a scheduled run started an hour after a button run")
	}
	if !runScheduledModelUpdate(aDayAndAMinuteAfterNow()) {
		t.Error("no scheduled run started a day after a button run")
	}
	waitForRefreshToStop(t)
}

// The record holds the outcome of a run and nothing about the catalog.
func TestTheModelUpdateRecordHoldsNoModelData(t *testing.T) {
	modelUpdateSetup(t)
	recordModelUpdateRun(catalog.Report{Created: 3, FailedParts: []string{catalog.PartEU}}, nil, true)

	b, err := os.ReadFile(paths.ModelUpdateCacheFile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"seq": true, "finished_at": true, "outcome": true, "error": true, "failed_parts": true, "scheduled": true}
	for key := range fields {
		if !allowed[key] {
			t.Errorf("the record holds %q, which is not part of the outcome of a run", key)
		}
	}
}

// GET /api/update/status carries what the page needs to stay silent for a
// scheduled run and to learn that a run finished.
func TestUpdateStatusCarriesTheLastRunAndWhetherTheRunIsScheduled(t *testing.T) {
	feed := modelUpdateSetup(t)
	switchDailyUpdate(t, true)
	feed.hold, feed.arrived = make(chan struct{}), make(chan struct{}, 1)

	read := func() map[string]interface{} {
		rec := httptest.NewRecorder()
		handleUpdateStatus(rec, httptest.NewRequest(http.MethodGet, "/api/update/status", nil))
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode the status: %v", err)
		}
		return body
	}

	before := read()
	if run, _ := before["last_run"].(map[string]interface{}); run["outcome"] != "" || run["seq"] != float64(0) {
		t.Errorf("before any run last_run = %v, want seq 0 and no outcome", before["last_run"])
	}
	runScheduledModelUpdate(time.Now())
	<-feed.arrived
	if during := read(); during["running"] != true || during["scheduled"] != true {
		t.Errorf("during a scheduled run the status is running=%v scheduled=%v, want both true", during["running"], during["scheduled"])
	}
	close(feed.hold)
	waitForRefreshToStop(t)
	after := read()
	run, _ := after["last_run"].(map[string]interface{})
	if _, isList := run["failed_parts"].([]interface{}); !isList {
		t.Errorf("last_run.failed_parts = %v, want a list (never null)", run["failed_parts"])
	}
	if run["seq"] != float64(1) || run["outcome"] != modelUpdateOK || run["scheduled"] != true {
		t.Errorf("after the run last_run = %v, want seq 1, ok, scheduled", run)
	}
}
