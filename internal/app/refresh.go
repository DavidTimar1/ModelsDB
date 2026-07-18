// Package app - on-demand full DB refresh controller.
//
// A full refresh runs the same three phases the CLI exposes, in sequence:
// fetchModels() (OpenRouter catalog + ZDR), then runCollections(), then
// runPricing(). It is triggered over HTTP (POST /api/update) so a long-running
// server instance can refresh without a restart.
//
// Concurrency model: a single shared refreshState protected by a mutex. The
// mutex (not a channel) is the right tool here because the state is a shared
// data structure read by the status endpoint and written by one background
// goroutine - there is no producer/consumer handoff. A single-flight guard
// (the running flag) ensures only one refresh runs at a time; the background
// goroutine has a clear lifetime - it executes the three phases and exits,
// releasing the flag in a deferred finalizer so even a panic cannot wedge it.
package app

import (
	"fmt"
	"log"
	"modelsdb/internal/catalog"
	"sync"
	"time"
)

// refreshPhase names the phase a running refresh is currently in.
type refreshPhase string

const (
	phaseIdle        refreshPhase = "idle"
	phaseCatalog     refreshPhase = "catalog"
	phaseCollections refreshPhase = "collections"
	phasePricing     refreshPhase = "pricing"
)

// refreshState holds the shared status of the on-demand refresh. All fields are
// guarded by mu; never read or write them without holding the lock.
type refreshState struct {
	mu         sync.Mutex
	running    bool
	phase      refreshPhase
	startedAt  time.Time
	finishedAt time.Time
	errors     []string
}

// refresh is the single process-wide refresh controller.
var refresh = &refreshState{phase: phaseIdle}

// refreshStatus is the JSON-serialisable snapshot returned by the status
// endpoint. Times are RFC3339 strings (empty when zero).
type refreshStatus struct {
	Running    bool     `json:"running"`
	Phase      string   `json:"phase"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at"`
	Errors     []string `json:"errors"`
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
	return refreshStatus{
		Running:    s.running,
		Phase:      string(s.phase),
		StartedAt:  fmtTime(s.startedAt),
		FinishedAt: fmtTime(s.finishedAt),
		Errors:     errs,
	}
}

// start attempts to begin a refresh. It returns false if one is already
// running (single-flight guard). On success it launches the background
// goroutine and returns true immediately.
func (s *refreshState) start() bool {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return false
	}
	s.running = true
	s.phase = phaseCatalog
	s.startedAt = time.Now()
	s.finishedAt = time.Time{}
	s.errors = nil
	s.mu.Unlock()

	go s.run()
	return true
}

// setPhase updates the current phase under the lock.
func (s *refreshState) setPhase(p refreshPhase) {
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

// run executes the three refresh phases in sequence. A phase error is recorded
// but does NOT abort the chain (mirrors startup's "update failures are
// non-fatal" philosophy). The deferred finalizer always clears the running
// flag, so a panic in a phase cannot leave the controller stuck.
func (s *refreshState) run() {
	defer func() {
		if r := recover(); r != nil {
			s.addError(fmt.Sprintf("panic: %v", r))
			log.Printf("ERROR refresh panicked: %v", r)
		}
		s.mu.Lock()
		s.running = false
		s.phase = phaseIdle
		s.finishedAt = time.Now()
		s.mu.Unlock()
		log.Printf("=== Full refresh finished ===")
	}()

	log.Printf("=== Full refresh started (catalog -> collections -> pricing) ===")

	s.setPhase(phaseCatalog)
	if err := catalog.FetchModels(); err != nil {
		log.Printf("Warning: refresh catalog phase failed: %v", err)
		s.addError("catalog: " + err.Error())
	}

	s.setPhase(phaseCollections)
	if err := catalog.RunCollections(); err != nil {
		log.Printf("Warning: refresh collections phase failed: %v", err)
		s.addError("collections: " + err.Error())
	}

	s.setPhase(phasePricing)
	if err := catalog.RunPricing(); err != nil {
		log.Printf("Warning: refresh pricing phase failed: %v", err)
		s.addError("pricing: " + err.Error())
	}
}
