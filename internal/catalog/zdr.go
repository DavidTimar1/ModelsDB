// Derive each OpenRouter model's Zero Data Retention (ZDR) flag.
//
// ZDR mirrors OpenRouter's website "Zero Data Retention" filter: a model is ZDR
// if ANY of its serving providers has dataPolicy.retainsPrompts == false.
// A model no OpenRouter listing covers is not written here.
//
// ZDR is NOT in the public API. It is derived from OpenRouter's unofficial
// frontend data:
//   - one bulk call to /api/frontend/v1/all-providers gives a provider -> policy map
//   - one public /api/v1/models/<id>/endpoints call per model lists its providers
//
// There is no bulk frontend models endpoint, so per-model endpoint fetches run
// through a bounded worker pool. The whole derivation fails soft: any network or
// parse failure logs a warning and leaves existing zdr values unchanged, so a
// failed ZDR pass never aborts the catalog refresh or crashes startup.

package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	zdrWorkers     = 8
	zdrHTTPTimeout = 20 * time.Second
	// zdrMaxBodyBytes bounds one provider-policy or endpoint answer.
	zdrMaxBodyBytes = 32 << 20
)

// AllProvidersURL is the bulk frontend endpoint fetchProviderPolicies reads. It
// is a package-level var, not a const, so a test can point it at a local
// httptest.Server alongside ApiUrl and run FetchModels with no real request to
// openrouter.ai for the provider-policy map either. The PER-MODEL endpoint
// address is built in modelEndpointsURL from each model's id, which is escaped
// segment by segment, dot segments included, so an id from the feed can never change the path. It is not
// injectable, so a test exercising FetchModels with a model that carries an id
// still reaches the real network there (harmlessly: a failure there only skips
// that model's ZDR value, per DeriveZDR's fail-soft design).
var AllProvidersURL = "https://openrouter.ai/api/frontend/v1/all-providers"

// ModelEndpointsBase is the address the per-model endpoint lists are read under.
// It is a package-level var so the browser exercise of a testhooks build can
// point it at a local server and measure the pacing with no request to
// openrouter.ai.
var ModelEndpointsBase = "https://openrouter.ai/api/v1/models"

// zdrClient is a dedicated client with a per-request timeout so a single slow
// endpoint cannot stall the whole refresh.
var zdrClient = shared.NewHTTPClient(zdrHTTPTimeout)

// fetchProviderPolicies returns a map of provider slug -> retainsPrompts from the
// bulk frontend all-providers endpoint (one call).
func fetchProviderPolicies(pc *pacer) (map[string]bool, error) {
	body, err := zdrGet(AllProvidersURL, pc)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Data []struct {
			Slug       string `json:"slug"`
			DataPolicy struct {
				RetainsPrompts bool `json:"retainsPrompts"`
			} `json:"dataPolicy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse all-providers JSON: %w", err)
	}
	policies := make(map[string]bool, len(doc.Data))
	for _, p := range doc.Data {
		if p.Slug != "" {
			policies[p.Slug] = p.DataPolicy.RetainsPrompts
		}
	}
	if len(policies) == 0 {
		return nil, fmt.Errorf("all-providers returned no providers")
	}
	return policies, nil
}

// modelEndpointTags fetches a model's provider list from the public endpoints API
// and returns each endpoint's provider slug (the part of the tag before any '/',
// e.g. "google-vertex/global" -> "google-vertex").
func modelEndpointTags(id string, pc *pacer) ([]string, error) {
	body, err := zdrGet(modelEndpointsURL(id), pc)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Data struct {
			Endpoints []struct {
				Tag string `json:"tag"`
			} `json:"endpoints"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse endpoints JSON: %w", err)
	}
	slugs := make([]string, 0, len(doc.Data.Endpoints))
	for _, e := range doc.Data.Endpoints {
		if e.Tag == "" {
			continue
		}
		slugs = append(slugs, strings.SplitN(e.Tag, "/", 2)[0])
	}
	return slugs, nil
}

// modelEndpointsURL is the endpoints address of one OpenRouter model id. Each
// "/"-separated part of the id is escaped on its own, so a stray "?", "#" or ".."
// in the feed's id stays inside its segment.
func modelEndpointsURL(id string) string {
	parts := strings.Split(id, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
		if part == "." || part == ".." {
			parts[i] = strings.ReplaceAll(part, ".", "%2E")
		}
	}
	return ModelEndpointsBase + "/" + strings.Join(parts, "/") + "/endpoints"
}

// zdrGet performs a GET (paced and retried, see getWithRetry) and returns the
// body for a 200 response, refusing one longer than zdrMaxBodyBytes.
func zdrGet(url string, pc *pacer) ([]byte, error) {
	resp, err := getWithRetry(zdrClient, url, pc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, zdrMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > zdrMaxBodyBytes {
		return nil, fmt.Errorf("body is larger than the %d byte limit", zdrMaxBodyBytes)
	}
	return body, nil
}

// isZDR applies the any-provider rule: ZDR is true if any serving provider keeps
// no prompts (retainsPrompts == false). Providers missing from the policy map are
// treated as retaining (conservative: do not claim ZDR we cannot confirm).
func isZDR(providerSlugs []string, policies map[string]bool) bool {
	for _, slug := range providerSlugs {
		if retains, ok := policies[slug]; ok && !retains {
			return true
		}
	}
	return false
}

// zdrResult carries one model's derived flag back from a worker.
type zdrResult struct {
	name string
	zdr  bool
}

// DeriveZDR derives and stores what OpenRouter's providers say about every
// OpenRouter model. It fetches the provider policy map once, then resolves each
// model's providers through a bounded worker pool. It is fail-soft: on a global
// failure (policy map fetch) it logs and leaves every stored answer untouched;
// per-model fetch failures are skipped (that model keeps its existing value).
// The error it returns says the pass did not do its job, either because a
// global step failed or because at least half of the per-model requests did;
// the refresh still carries on, and reports the pass to the caller as failed.
//
// A MODEL NO OPENROUTER LISTING COVERS IS NOT WRITTEN HERE AT ALL. There is no
// company serving it, so nobody keeps a copy of what is sent to it, and that is
// worked out when the model is read rather than stored on every row - which is
// what keeps the rule in one place instead of two.
//
// Only the ids in listedIDs are asked about: a row the feed no longer lists has
// no endpoints to read, and asking would only count a 404 as a failure.
func DeriveZDR(listedIDs map[string]bool, pc *pacer) error {
	policies, err := fetchProviderPolicies(pc)
	if err != nil {
		log.Printf("Warning: ZDR skipped (provider policy fetch failed): %v", err)
		return fmt.Errorf("provider policy fetch failed: %w", err)
	}

	type job struct{ name, id string }
	listed, err := store.ModelsOfSourceWithAnID("openrouter")
	if err != nil {
		log.Printf("Warning: ZDR skipped (model query failed): %v", err)
		return fmt.Errorf("model query failed: %w", err)
	}
	jobs := make([]job, 0, len(listed))
	for _, m := range listed {
		if listedIDs[m.ID] {
			jobs = append(jobs, job{m.Name, m.ID})
		}
	}
	if len(jobs) == 0 {
		return nil
	}

	log.Printf("=== Deriving ZDR for %d OpenRouter models (%d providers, %d workers) ===",
		len(jobs), len(policies), pc.workers())

	jobCh := make(chan job)
	resultCh := make(chan zdrResult)

	var wg sync.WaitGroup
	var failMu sync.Mutex
	failures := 0
	for w := 0; w < pc.workers(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				slugs, err := modelEndpointTags(j.id, pc)
				if err != nil {
					failMu.Lock()
					failures++
					failMu.Unlock()
					continue // leave this model's existing zdr value unchanged
				}
				resultCh <- zdrResult{name: j.name, zdr: isZDR(slugs, policies)}
			}
		}()
	}

	// Close resultCh once all workers finish.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Feed jobs, then signal no-more-work, in a separate goroutine so the main
	// goroutine can drain results concurrently (workers send on resultCh).
	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
	}()

	updated := 0
	for res := range resultCh {
		store.RecordZDR(res.name, res.zdr)
		updated++
	}

	log.Printf("  ok ZDR derived for %d models (%d endpoint fetches failed -> kept prior value)", updated, failures)
	if mostFetchesFailed(failures, len(jobs)) {
		return fmt.Errorf("%d of %d endpoint fetches failed", failures, len(jobs))
	}
	return nil
}
