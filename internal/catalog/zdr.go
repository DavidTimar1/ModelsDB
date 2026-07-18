// Package catalog - derive each OpenRouter model's Zero Data Retention (ZDR) flag.
//
// ZDR mirrors OpenRouter's website "Zero Data Retention" filter: a model is ZDR
// if ANY of its serving providers has dataPolicy.retainsPrompts == false.
// Non-OpenRouter models (huggingface | collection | manual) are always ZDR.
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
	"modelsdb/internal/dbcore"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	allProvidersURL = "https://openrouter.ai/api/frontend/v1/all-providers"
	zdrWorkers      = 8
	zdrHTTPTimeout  = 20 * time.Second
)

// zdrClient is a dedicated client with a per-request timeout so a single slow
// endpoint cannot stall the whole refresh.
var zdrClient = &http.Client{Timeout: zdrHTTPTimeout}

// fetchProviderPolicies returns a map of provider slug -> retainsPrompts from the
// bulk frontend all-providers endpoint (one call).
func fetchProviderPolicies() (map[string]bool, error) {
	body, err := zdrGet(allProvidersURL)
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
func modelEndpointTags(id string) ([]string, error) {
	body, err := zdrGet("https://openrouter.ai/api/v1/models/" + id + "/endpoints")
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

// zdrGet performs a browser-style GET and returns the body for a 200 response.
func zdrGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	resp, err := zdrClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
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

// deriveZDR derives and stores the zdr flag for every OpenRouter model. It fetches
// the provider policy map once, then resolves each model's providers through a
// bounded worker pool. It is fail-soft: on a global failure (policy map fetch)
// it logs and returns nil, leaving all existing zdr values untouched; per-model
// fetch failures are skipped (that model keeps its existing value).
//
// Non-OpenRouter models are forced to zdr=1 in a single statement.
func deriveZDR() error {
	// Non-OpenRouter models are always ZDR by rule (no provider to retain prompts).
	if _, err := dbcore.DB.Exec("UPDATE models SET zdr=1 WHERE source != 'openrouter'"); err != nil {
		log.Printf("Warning: ZDR set for non-openrouter models failed: %v", err)
	}

	policies, err := fetchProviderPolicies()
	if err != nil {
		log.Printf("Warning: ZDR skipped (provider policy fetch failed): %v", err)
		return nil
	}

	type job struct{ name, id string }
	rows, err := dbcore.DB.Query("SELECT name, id FROM models WHERE source='openrouter' AND id != ''")
	if err != nil {
		log.Printf("Warning: ZDR skipped (model query failed): %v", err)
		return nil
	}
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.name, &j.id) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	if len(jobs) == 0 {
		return nil
	}

	log.Printf("=== Deriving ZDR for %d OpenRouter models (%d providers, %d workers) ===",
		len(jobs), len(policies), zdrWorkers)

	jobCh := make(chan job)
	resultCh := make(chan zdrResult)

	var wg sync.WaitGroup
	var failMu sync.Mutex
	failures := 0
	for w := 0; w < zdrWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				slugs, err := modelEndpointTags(j.id)
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
		flag := 0
		if res.zdr {
			flag = 1
		}
		if _, err := dbcore.DB.Exec("UPDATE models SET zdr=? WHERE name=?", flag, res.name); err != nil {
			log.Printf("Warning: ZDR store for %q failed: %v", res.name, err)
			continue
		}
		updated++
	}

	log.Printf("  ok ZDR derived for %d models (%d endpoint fetches failed -> kept prior value)", updated, failures)
	return nil
}
