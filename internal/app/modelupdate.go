// The opt-in daily model update: once a day, while the server runs, the same
// refresh the Update models button starts is started with nobody pressing it.
//
// WHY IT EXISTS. The server refreshes the catalog once per process start and
// never again, so a server that stays up for weeks serves a catalog as old as
// its last start or its last button press. The switch (the app_settings row
// dailyUpdateSettingKey, set from the Info dialog) makes the refresh repeat.
//
// WHY IT LOOKS LIKE THE BACKUP SCHEDULER AND NOT LIKE THE RELEASE CHECKER. The
// loop wakes often and asks a pure question, "is a run due now", so ticking the
// switch takes effect within an hour instead of after a day of sleeping, and
// the question takes the time as an argument so a test can force it. The
// release checker is skipped on a development build; this is not, because the
// development sandbox is where the feature is tested.
//
// THE RECORD OF THE LAST RUN. Every refresh that finishes, whoever started it
// (start-up, the button, this scheduler), is recorded in modelUpdateLast and in
// its own cache-dir file. It is machine state like backup-schedule.json and for
// the same reason: a restored database must not tell this install that it
// already updated today. It holds no model data. The page reads it through
// GET /api/update/status to learn that a run it did not start has finished, and
// to show the outcome of a run that failed.

package app

import (
	"encoding/json"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"os"
	"sync"
	"time"
)

// dailyUpdateSettingKey is the app_settings row the Info dialog's switch writes.
// It is not a server.* key, because the page may not write those.
const dailyUpdateSettingKey = "daily_model_update"

// The outcomes a finished run is recorded with.
const (
	modelUpdateOK      = "ok"
	modelUpdatePartial = "partial"
	modelUpdateFailed  = "failed"
)

// The two clocks of the scheduler. They are variables so the testhooks build
// (modelupdate_testhooks.go) can shorten them; nothing at run time changes them.
var (
	// modelUpdateWakeInterval is how often the loop asks whether a run is due.
	modelUpdateWakeInterval = time.Hour
	// modelUpdateInterval is how long after the last finished run the next one is due.
	modelUpdateInterval = 24 * time.Hour
)

// scheduledRunOptions is the one choice about what a scheduled run does that a
// reader's answer may change. SkipPerModelPasses says whether it includes the
// two per-model passes (zero data retention and throughput), which send one
// request per model, about 1,400 in all, to OpenRouter's frontend endpoints;
// true leaves them to the start-up update and the button. Paced says the run
// sends its requests one at a time with a pause of about a second between them,
// which takes around 25 minutes and is why nobody is shown it: the daily run is
// silent. The start-up update and the button are not paced; pacing them as well
// means giving them these options.
var scheduledRunOptions = catalog.Options{SkipPerModelPasses: false, Paced: true}

// modelUpdateRun is the record of the last refresh that finished.
type modelUpdateRun struct {
	// Seq counts finished runs, across restarts. A page compares it with the
	// value it read at load to learn that a run finished after it loaded.
	Seq        int    `json:"seq"`
	FinishedAt string `json:"finished_at"`
	// Outcome is "" until a run has finished, then modelUpdateOK, modelUpdatePartial
	// (the feed was stored but a fail-soft pass did not do its job) or modelUpdateFailed.
	Outcome string `json:"outcome"`
	// Error is the feed failure when Outcome is modelUpdateFailed.
	Error string `json:"error"`
	// FailedParts names the passes that did not do their job, in the words shown.
	FailedParts []string `json:"failed_parts"`
	// Scheduled is true when the daily scheduler started the run.
	Scheduled bool `json:"scheduled"`
}

var (
	modelUpdateMu   sync.Mutex
	modelUpdateLast modelUpdateRun
)

// loadModelUpdateCache reads the record of the last run from the cache dir. A
// missing or unreadable file means no run is known.
func loadModelUpdateCache() {
	b, err := os.ReadFile(paths.ModelUpdateCacheFile)
	if err != nil {
		return
	}
	var run modelUpdateRun
	if json.Unmarshal(b, &run) != nil {
		return
	}
	modelUpdateMu.Lock()
	modelUpdateLast = run
	modelUpdateMu.Unlock()
}

// saveModelUpdateCache writes the record atomically. A failed write is logged
// and costs only the persistence: the in-memory record still serves this process.
func saveModelUpdateCache(run modelUpdateRun) {
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(paths.CacheDir, 0755)
	if err := paths.WriteFileAtomic(paths.ModelUpdateCacheFile, b, 0644); err != nil {
		log.Printf("Warning: could not write the model-update record: %v", err)
	}
}

// currentModelUpdateRun answers a copy of the record of the last finished run.
func currentModelUpdateRun() modelUpdateRun {
	modelUpdateMu.Lock()
	defer modelUpdateMu.Unlock()
	run := modelUpdateLast
	run.FailedParts = append([]string{}, run.FailedParts...)
	return run
}

// recordModelUpdateRun records a refresh that has just finished. A feed failure
// (err) makes the run failed; otherwise any failed pass in the report makes it
// partial. Every finished run counts, so a daily run is due a day after the last
// run of any kind.
func recordModelUpdateRun(report catalog.Report, err error, scheduled bool) {
	run := modelUpdateRun{
		FinishedAt:  dbcore.NowStamp(),
		Outcome:     modelUpdateOK,
		FailedParts: append([]string{}, report.FailedParts...),
		Scheduled:   scheduled,
	}
	switch {
	case err != nil:
		run.Outcome = modelUpdateFailed
		run.Error = err.Error()
		run.FailedParts = []string{}
	case len(report.FailedParts) > 0:
		run.Outcome = modelUpdatePartial
	}
	modelUpdateMu.Lock()
	run.Seq = modelUpdateLast.Seq + 1
	modelUpdateLast = run
	modelUpdateMu.Unlock()
	saveModelUpdateCache(run)
}

// modelUpdateDue is the scheduler's pure question: given the last finished run
// and the time now, should a run start. It is due when none has finished yet,
// when the last one failed (so a failure is retried at the next wake, not after
// a day), and when a day has passed since the last one finished. A stamp in the
// future, left by a clock that moved backward, counts as due rather than
// holding the scheduler off until the clock catches up.
func modelUpdateDue(last modelUpdateRun, now time.Time) bool {
	if last.Outcome == "" || last.Outcome == modelUpdateFailed {
		return true
	}
	finished, err := time.Parse(time.RFC3339, last.FinishedAt)
	if err != nil || finished.After(now) {
		return true
	}
	return now.Sub(finished) >= modelUpdateInterval
}

// dailyUpdateEnabled reads the switch. It is ON until the reader stores it as
// off, so an install that never touched it updates daily. It is read at every
// wake, so changing it takes effect at the next one.
func dailyUpdateEnabled() bool {
	return store.ReadSettingBoolOr(dailyUpdateSettingKey, true)
}

// runScheduledModelUpdate starts a refresh when the switch is on and a run is
// due at now, and reports whether it started one. It starts through the same
// single-flight controller as the button, so a run already under way is the run:
// nothing starts a second one, and a button press during it joins it. Turning
// the switch off while a run is under way does not cancel the run; it only
// stops the next one being started.
func runScheduledModelUpdate(now time.Time) bool {
	if !dailyUpdateEnabled() {
		return false
	}
	if !modelUpdateDue(currentModelUpdateRun(), now) {
		return false
	}
	if !refresh.start(true) {
		return false
	}
	log.Printf("Daily model update: run started")
	return true
}

// startModelUpdateScheduler launches the background loop that wakes every
// modelUpdateWakeInterval and starts a run when one is due. It always starts the
// loop and sleeps before the first wake: the start-up update has just run, and
// the switch is read at each wake, so the loop itself decides nothing. The
// goroutine lives as long as the process, like the backup scheduler's.
func startModelUpdateScheduler() {
	go func() {
		for {
			time.Sleep(modelUpdateWakeInterval)
			runScheduledModelUpdate(time.Now())
		}
	}()
}

// runStartupModelUpdate is the refresh every server start makes before it
// serves. A failure must not take the server down, so it is logged and the
// existing catalog is served. It is recorded like any other run, so the
// scheduler does not start a second one seconds later.
func runStartupModelUpdate() {
	report, err := catalog.FetchModelsWith(catalog.Options{})
	recordModelUpdateRun(report, err, false)
	if err != nil {
		log.Printf("Warning: startup model update failed, serving existing data: %v", err)
		return
	}
	log.Printf("Startup update: %d model(s) added to the library", report.Created)
}
