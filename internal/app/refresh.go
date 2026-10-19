// On-demand full DB refresh controller.
//
// A full refresh runs catalog.FetchModelsWith (the OpenRouter feed, the ZDR pass,
// the EU routing pass and the throughput pass).
// It is triggered over HTTP (POST /api/update) so a long-running server instance
// can refresh without a restart, and once a day by the opt-in scheduler in
// modelupdate.go. Both go through start, so there is one refresh at a time.
//
// Concurrency model: a single shared refreshState protected by a mutex. The
// mutex (not a channel) is the right tool here because the state is a shared
// data structure read by the status endpoint and written by one background
// goroutine - there is no producer/consumer handoff. A single-flight guard
// (the running flag) ensures only one refresh runs at a time; the background
// goroutine has a clear lifetime - it executes the phase and exits, releasing
// the flag in a deferred finalizer so even a panic cannot wedge it.

package app

import (
	"fmt"
	"log"
	"modelsdb/internal/catalog"
	"strings"
	"sync"
	"time"
)

// progressPhase names the phase a running refresh is currently in, which is what
// the status endpoint reports as it goes. It is deliberately NOT called after the
// refresh: the store package has its own idea of a phase - which pass OWNS a
// column, a permission question that has nothing to do with what is running right
// now - and one word meaning both is how a reader moving between the two files
// mistakes them for the same thing.
type progressPhase string

const (
	progressIdle    progressPhase = "idle"
	progressCatalog progressPhase = "catalog"
)

// refreshState holds the shared status of the on-demand refresh. All fields are
// guarded by mu; never read or write them without holding the lock.
type refreshState struct {
	mu         sync.Mutex
	running    bool
	phase      progressPhase
	startedAt  time.Time
	finishedAt time.Time
	errors     []string
	warnings   []string
	added      int
	// scheduled is true while the running refresh was started by the daily
	// scheduler rather than by a person. The page reads it to stay silent for
	// a run nobody asked for.
	scheduled bool
}

// refresh is the single process-wide refresh controller.
var refresh = &refreshState{phase: progressIdle}

// refreshStatus is the JSON-serialisable snapshot returned by the status
// endpoint. Times are RFC3339 strings (empty when zero).
type refreshStatus struct {
	Running    bool     `json:"running"`
	Phase      string   `json:"phase"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at"`
	Errors     []string `json:"errors"`
	Warnings   []string `json:"warnings"`
	// Added is how many models the finished refresh brought into the library.
	// The page reads it off the status it already polls, which is why nothing
	// about reporting the number costs a second request.
	Added int `json:"added"`
	// Scheduled is true while the running refresh was started by the daily
	// scheduler. A page that finds one running leaves its Update models button
	// and its status line alone.
	Scheduled bool `json:"scheduled"`
	// LastRun is the record of the last refresh that finished, whoever started
	// it, kept across restarts. A page learns that a run finished after it loaded
	// by comparing LastRun.Seq with the one it read at load.
	LastRun modelUpdateRun `json:"last_run"`
}

// fmtTime renders t as RFC3339, or "" when zero.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// snapshot returns the current status under the lock.
func (s *refreshState) snapshot() refreshStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := make([]string, len(s.errors))
	copy(errs, s.errors)
	warnings := make([]string, len(s.warnings))
	copy(warnings, s.warnings)
	return refreshStatus{
		Running:    s.running,
		Phase:      string(s.phase),
		StartedAt:  fmtTime(s.startedAt),
		FinishedAt: fmtTime(s.finishedAt),
		Errors:     errs,
		Warnings:   warnings,
		Added:      s.added,
		Scheduled:  s.running && s.scheduled,
		LastRun:    currentModelUpdateRun(),
	}
}

// start attempts to begin a refresh. scheduled says the daily scheduler asked
// for it rather than a person. It returns false if one is already
// running (single-flight guard). On success it launches the background
// goroutine and returns true immediately.
func (s *refreshState) start(scheduled bool) bool {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return false
	}
	s.running = true
	s.phase = progressCatalog
	s.startedAt = time.Now()
	s.finishedAt = time.Time{}
	s.errors = nil
	s.warnings = nil
	s.added = 0
	s.scheduled = scheduled
	s.mu.Unlock()

	go s.run()
	return true
}

// setPhase updates the current phase under the lock.
func (s *refreshState) setPhase(p progressPhase) {
	s.mu.Lock()
	s.phase = p
	s.mu.Unlock()
}

// addError records a phase error under the lock (fail-soft: the chain
// continues after recording).
func (s *refreshState) addError(msg string) {
	s.mu.Lock()
	s.errors = append(s.errors, msg)
	s.mu.Unlock()
}

// addWarning records something worth the reader's attention that is not a
// failure - a field the feed added or dropped since the last refresh.
func (s *refreshState) addWarning(msg string) {
	s.mu.Lock()
	s.warnings = append(s.warnings, msg)
	s.mu.Unlock()
}

// setAdded records how many models the refresh brought into the library.
func (s *refreshState) setAdded(n int) {
	s.mu.Lock()
	s.added = n
	s.mu.Unlock()
}

// run executes the refresh phase. A phase error is recorded but does NOT abort
// the chain (mirrors startup's "update failures are non-fatal" philosophy). The
// deferred finalizer records the finished run for the scheduler and the page
// BEFORE it clears the running flag, so a page that sees the refresh stop also
// sees its record, and a panic in the phase cannot leave the controller stuck.
func (s *refreshState) run() {
	var report catalog.Report
	var runErr error
	defer func() {
		if r := recover(); r != nil {
			s.addError(fmt.Sprintf("panic: %v", r))
			log.Printf("ERROR refresh panicked: %v", r)
			runErr = fmt.Errorf("panic: %v", r)
		}
		s.mu.Lock()
		scheduled := s.scheduled
		s.mu.Unlock()
		recordModelUpdateRun(report, runErr, scheduled)
		s.mu.Lock()
		s.running = false
		s.phase = progressIdle
		s.finishedAt = time.Now()
		s.mu.Unlock()
		log.Printf("=== Full refresh finished ===")
	}()

	log.Printf("=== Full refresh started (catalog) ===")

	s.setPhase(progressCatalog)
	opts := catalog.Options{}
	s.mu.Lock()
	if s.scheduled {
		opts = scheduledRunOptions
	}
	s.mu.Unlock()
	report, runErr = catalog.FetchModelsWith(opts)
	s.setAdded(report.Created)
	if runErr != nil {
		log.Printf("Warning: refresh catalog phase failed: %v", runErr)
		s.addError("catalog: " + runErr.Error())
	}
	if len(report.NewFields) > 0 {
		s.addWarning(fmt.Sprintf("the feed added %d field(s) it did not carry last refresh: %s", len(report.NewFields), strings.Join(report.NewFields, ", ")))
	}
	if len(report.VanishedFields) > 0 {
		s.addWarning(fmt.Sprintf("the feed dropped %d field(s) it carried last refresh: %s", len(report.VanishedFields), strings.Join(report.VanishedFields, ", ")))
	}
}
