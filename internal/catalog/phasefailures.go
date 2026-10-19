package catalog

import (
	"fmt"
	"strings"
)

// maxReportedFailures caps how many individual reasons a phase's error names.
// A listing that is down can produce a failure for every model it carries, and
// an error message holding hundreds of them is unreadable in a status panel and
// useless in a log. The COUNT is always exact; only the list of reasons is
// trimmed.
const maxReportedFailures = 5

// phaseFailures collects what one refresh phase could not do.
//
// IT EXISTS BECAUSE A PARTIAL FAILURE IS NORMAL HERE. A refresh phase reads
// several independent listings, and one unreachable listing must not discard the
// models the others produced - so a phase finishes the work it can. Without
// somewhere to put the failures, "finish anyway" and "tell the caller" are in
// conflict, and the phase ends up returning success from a run where nothing
// worked. This holds the failures so the phase can do both: complete, and then
// answer with one error naming what it missed.
type phaseFailures struct {
	reasons []string
	count   int
}

// record notes one thing the phase could not do. The message is what a person
// reads in the status panel, so it names the listing, model or step.
func (f *phaseFailures) record(format string, args ...interface{}) {
	f.count++
	if len(f.reasons) < maxReportedFailures {
		f.reasons = append(f.reasons, fmt.Sprintf(format, args...))
	}
}

// asError is the phase's answer: nil when nothing failed, otherwise one error
// carrying the exact number of failures and the first few reasons.
func (f *phaseFailures) asError() error {
	if f.count == 0 {
		return nil
	}
	listed := strings.Join(f.reasons, "; ")
	if f.count > len(f.reasons) {
		return fmt.Errorf("%d failure(s), the first %d being: %s", f.count, len(f.reasons), listed)
	}
	return fmt.Errorf("%d failure(s): %s", f.count, listed)
}

// mostFetchesFailed says whether a per-model pass lost at least half of its
// requests, which is what separates a block or an outage from the stray
// timeout a few hundred requests always draw. A pass that loses that many is
// reported to the caller as failed; one that loses fewer is not, because every
// model it could not reach simply keeps the value it had.
func mostFetchesFailed(failures, total int) bool {
	return total > 0 && failures*2 >= total
}
