// How a refresh treats OpenRouter's servers: slower when they ask for it, and
// slower still when the run is the silent daily one.
//
// WHY IT EXISTS. A refresh sends about 1,400 requests, 8 at a time, to
// endpoints OpenRouter does not document for this use. A person who pressed
// Update models waits on it, so that speed stays. Two things protect the other
// side: every request backs off when the server answers 429 or 503 (the retry
// loop below), and the daily run, which nobody waits on, sends its requests one
// at a time with a pause between them (the pacer).

package catalog

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// The knobs of the pause and of the back-off. They are variables so a test can
// shrink them to milliseconds; nothing at run time changes them.
var (
	// pacingGap and pacingJitter set the pause between two requests of a paced
	// run: the gap plus or minus up to the jitter, so 0.8 to 1.2 seconds.
	pacingGap    = time.Second
	pacingJitter = 200 * time.Millisecond
	// retryFirstWait is the back-off after the first 429 or 503 that carries no
	// Retry-After; each further retry doubles it up to retryWaitCap.
	retryFirstWait = 2 * time.Second
	// retryWaitCap bounds both the back-off and a server's Retry-After.
	retryWaitCap = 60 * time.Second
	// maxRetries is how many times one request is sent again before it counts as failed.
	maxRetries = 5
	// sleepFor is how the pacer and the retry loop wait.
	sleepFor = time.Sleep
)

// pacer spaces the requests of one paced run. A nil *pacer is an unpaced run:
// it never waits.
type pacer struct{ started bool }

// newPacer returns the pacer of a paced run.
func newPacer() *pacer { return &pacer{} }

// wait pauses before a request, except the run's first. The caller sends its
// requests one at a time, so the pauses are the spacing.
func (p *pacer) wait() {
	if p == nil {
		return
	}
	if !p.started {
		p.started = true
		return
	}
	pause := pacingGap
	if pacingJitter > 0 {
		pause += time.Duration(rand.Int63n(int64(2*pacingJitter)+1)) - pacingJitter
	}
	sleepFor(pause)
}

// workers is how many requests a per-model pass keeps in flight: one when the
// run is paced, zdrWorkers otherwise.
func (p *pacer) workers() int {
	if p != nil {
		return 1
	}
	return zdrWorkers
}

// slowDown says whether a status is OpenRouter asking the caller to ease off.
func slowDown(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// retryWait is how long to wait before retry number attempt (0 for the first):
// the server's Retry-After when it gave one, else retryFirstWait doubled per
// attempt, and never more than retryWaitCap. Retry-After is read as seconds or
// as an HTTP date.
func retryWait(resp *http.Response, attempt int, now time.Time) time.Duration {
	if h := resp.Header.Get("Retry-After"); h != "" {
		if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
			return min(time.Duration(secs)*time.Second, retryWaitCap)
		}
		if at, err := http.ParseTime(h); err == nil {
			return min(max(at.Sub(now), 0), retryWaitCap)
		}
	}
	wait := retryFirstWait
	for i := 0; i < attempt && wait < retryWaitCap; i++ {
		wait *= 2
	}
	return min(wait, retryWaitCap)
}

// getWithRetry sends a GET, first pausing as the pacer says, and sends it again
// after a 429 or 503 up to maxRetries times, waiting as retryWait says. It
// returns the last response, whatever its status, for the caller to judge: a
// request that is still refused after its retries comes back as the 429 or 503,
// which the caller counts as a failure. The caller closes the body.
func getWithRetry(client *http.Client, url string, pc *pacer) (*http.Response, error) {
	pc.wait()
	for attempt := 0; ; attempt++ {
		resp, err := client.Get(url)
		if err != nil || !slowDown(resp.StatusCode) || attempt >= maxRetries {
			return resp, err
		}
		wait := retryWait(resp, attempt, time.Now())
		resp.Body.Close()
		sleepFor(wait)
	}
}
