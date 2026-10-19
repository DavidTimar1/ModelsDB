// Derive each OpenRouter model's Throughput: how many tokens per second the
// fastest provider serving it measures.
//
// The figure is NOT in the public API (the documented endpoints answer carries
// throughput_last_30m as null for every endpoint). It is read from the
// unofficial frontend feed behind OpenRouter's own model page, one call per
// model, which lists every provider endpoint with its measured percentiles. The
// feed can change without notice, so the whole pass fails soft: a model whose
// call fails keeps the value it had, and the refresh never fails because of it.

package catalog

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/url"
	"strings"
	"sync"
)

// StatsEndpointBase is the frontend feed that lists one model's provider
// endpoints with their measured speeds. It is a package-level var so a test can
// point it at a local httptest.Server and run the pass with no real request to
// openrouter.ai.
var StatsEndpointBase = "https://openrouter.ai/api/frontend/v1/stats/endpoint"

// standardVariant is the variant OpenRouter files a model under when its id
// carries no ":" suffix.
const standardVariant = "standard"

// statsEndpointURL is the stats address of one model: its permaslug (the
// canonical slug the feed gives the listing) and its variant. Both are query
// values, escaped as such, so a stray "&" or "#" from the feed stays inside its
// value.
func statsEndpointURL(permaslug, variant string) string {
	q := url.Values{}
	q.Set("latencyMetric", "latency")
	q.Set("perfWorkload", "text_generation")
	q.Set("permaslug", permaslug)
	q.Set("variant", variant)
	return StatsEndpointBase + "?" + q.Encode()
}

// variantOfID answers the variant an OpenRouter id names: the part after the
// first ":" ("deepseek/deepseek-r1:free" -> "free"), else the standard variant.
func variantOfID(id string) string {
	if _, variant, found := strings.Cut(id, ":"); found && strings.TrimSpace(variant) != "" {
		return strings.TrimSpace(variant)
	}
	return standardVariant
}

// fastestThroughput reads one stats answer and returns the highest p50
// throughput across its endpoints, in whole tokens per second. An endpoint with
// no stats, null stats or no p50 figure is skipped; 0 means no endpoint was
// measured, which is a real answer. An answer without a data list at all is an
// error, because that is what a changed feed looks like and it must not be read
// as "nobody is measured".
func fastestThroughput(body []byte) (int, error) {
	var doc struct {
		Data *[]struct {
			Stats *struct {
				P50Throughput *float64 `json:"p50_throughput"`
			} `json:"stats"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return 0, fmt.Errorf("parse stats JSON: %w", err)
	}
	if doc.Data == nil {
		return 0, fmt.Errorf("stats answer has no data list")
	}
	best := 0.0
	for _, endpoint := range *doc.Data {
		if endpoint.Stats == nil || endpoint.Stats.P50Throughput == nil {
			continue
		}
		if p50 := *endpoint.Stats.P50Throughput; p50 > best && !math.IsInf(p50, 0) {
			best = p50
		}
	}
	return int(math.Round(best)), nil
}

// throughputJob is one model to ask about: the row it is stored on, and the
// permaslug and variant its stats are listed under.
type throughputJob struct {
	name, permaslug, variant string
}

// throughputJobs works out, from the feed's own records, what to ask for each
// row this refresh stored. The permaslug is the one NormalizeModel stores as
// the slug cell. A row is asked about once, however many records name it.
func throughputJobs(data []map[string]interface{}, idToName map[string]string) []throughputJob {
	seen := map[string]bool{}
	var jobs []throughputJob
	for _, m := range data {
		id := shared.GetStr(m, "id")
		name, listed := idToName[id]
		if !listed || seen[name] {
			continue
		}
		seen[name] = true
		permaslug := shared.GetStr(m, "canonical_slug")
		if permaslug == "" {
			permaslug = id
		}
		jobs = append(jobs, throughputJob{name: name, permaslug: permaslug, variant: variantOfID(id)})
	}
	return jobs
}

// throughputResult carries one model's figure back from a worker.
type throughputResult struct {
	name string
	tps  int
}

// DeriveThroughput stores the fastest measured speed for every model the
// refresh just stored. It runs the same bounded worker pool as DeriveZDR, so it
// puts no more load on OpenRouter at a time than that pass does. A model whose
// call fails is skipped and keeps its previous value; a model that is no longer
// measured by anyone has its cell cleared, because an old speed is worse than
// none. The error it returns says at least half of the requests failed, which
// the refresh reports to its caller as a failed pass.
func DeriveThroughput(data []map[string]interface{}, idToName map[string]string, pc *pacer) error {
	jobs := throughputJobs(data, idToName)
	if len(jobs) == 0 {
		return nil
	}
	log.Printf("=== Deriving throughput for %d OpenRouter models (%d workers) ===", len(jobs), pc.workers())

	jobCh := make(chan throughputJob)
	resultCh := make(chan throughputResult)

	var wg sync.WaitGroup
	var failMu sync.Mutex
	failures := 0
	for w := 0; w < pc.workers(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				body, err := zdrGet(statsEndpointURL(j.permaslug, j.variant), pc)
				var tps int
				if err == nil {
					tps, err = fastestThroughput(body)
				}
				if err != nil {
					failMu.Lock()
					failures++
					failMu.Unlock()
					continue // leave this model's existing figure unchanged
				}
				resultCh <- throughputResult{name: j.name, tps: tps}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
	}()

	measured, cleared := 0, 0
	for res := range resultCh {
		store.RecordThroughput(res.name, res.tps)
		if res.tps > 0 {
			measured++
		} else {
			cleared++
		}
	}
	log.Printf("  ok throughput for %d models (%d with no measured provider, %d fetches failed -> kept prior value)", measured, cleared, failures)
	if mostFetchesFailed(failures, len(jobs)) {
		return fmt.Errorf("%d of %d stats fetches failed", failures, len(jobs))
	}
	return nil
}
