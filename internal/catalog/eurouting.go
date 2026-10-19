// Derive each OpenRouter model's EU routing flag.
//
// OpenRouter serves an EU address (In-Region Routing) that reaches only
// endpoints onboarded in the EU. Its public model list answers, for
// region=eu, exactly the models such an endpoint exists for, so ONE request
// answers every row. A model in that list is "yes"; every other OpenRouter
// model in the same refresh's feed is "no".
//
// The answer is a single list, so a wrong list is wrong for every row at once.
// An unknown query parameter is silently ignored by OpenRouter and answers the
// FULL feed, so a renamed region parameter would mark every model "yes". The
// pass therefore writes nothing unless the list passes euListIsPlausible, and
// it never clears a stored value on failure.

package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"time"
)

const (
	// euHTTPTimeout bounds the one EU list request, as zdrHTTPTimeout does.
	euHTTPTimeout = 20 * time.Second
	// euMaxBodyBytes bounds the EU list body.
	euMaxBodyBytes = 32 << 20
	// euImplausibleShare is the part of the feed (as a fraction) at which the
	// EU list is taken to be the whole feed under another name: the sign of a
	// region parameter OpenRouter now ignores.
	euImplausibleShare = 0.9
)

// EURoutingURL is the OpenRouter list of models served through its EU address.
// It is a package-level var, not a const, so a test can point it at a local
// httptest.Server and run the refresh with no request to openrouter.ai.
var EURoutingURL = "https://openrouter.ai/api/v1/models?region=eu&output_modalities=all"

// euClient carries the shared plain browser User-Agent and a timeout.
var euClient = shared.NewHTTPClient(euHTTPTimeout)

// fetchEUModelIDs returns the ids of the models in OpenRouter's EU list.
func fetchEUModelIDs(pc *pacer) (map[string]bool, error) {
	resp, err := getWithRetry(euClient, EURoutingURL, pc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, euMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > euMaxBodyBytes {
		return nil, fmt.Errorf("body is larger than the %d byte limit", euMaxBodyBytes)
	}
	var doc struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse EU list JSON: %w", err)
	}
	if doc.Data == nil {
		return nil, fmt.Errorf("the answer has no data list")
	}
	ids := make(map[string]bool, len(doc.Data))
	for _, m := range doc.Data {
		if m.ID != "" {
			ids[m.ID] = true
		}
	}
	return ids, nil
}

// euListIsPlausible says whether the EU list can be a real region filter of
// this refresh's feed. It is not when it is empty, when it shares no id with
// the feed, or when it is 90% or more of the feed's size.
func euListIsPlausible(eu map[string]bool, listedIDs map[string]bool) error {
	if len(eu) == 0 {
		return fmt.Errorf("the EU list is empty")
	}
	common := 0
	for id := range eu {
		if listedIDs[id] {
			common++
		}
	}
	if common == 0 {
		return fmt.Errorf("the EU list shares no id with the feed")
	}
	if float64(len(eu)) >= euImplausibleShare*float64(len(listedIDs)) {
		return fmt.Errorf("the EU list holds %d of the feed's %d models, which looks like the region filter was ignored", len(eu), len(listedIDs))
	}
	return nil
}

// DeriveEURouting stores, for every OpenRouter row whose id is in listedIDs,
// whether OpenRouter's EU list holds it. It is fail-soft: on any request or
// sanity failure it logs one warning and writes nothing, so every stored value
// stays and the refresh carries on; the error it returns tells the caller the
// pass wrote nothing.
//
// A row outside listedIDs (a model OpenRouter dropped) keeps its last value,
// and a row that is not an OpenRouter row is never written.
func DeriveEURouting(listedIDs map[string]bool, pc *pacer) error {
	eu, err := fetchEUModelIDs(pc)
	if err != nil {
		log.Printf("Warning: EU routing skipped (EU list fetch failed): %v", err)
		return fmt.Errorf("EU list fetch failed: %w", err)
	}
	if err := euListIsPlausible(eu, listedIDs); err != nil {
		log.Printf("Warning: EU routing skipped (%v)", err)
		return err
	}
	listed, err := store.ModelsOfSourceWithAnID("openrouter")
	if err != nil {
		log.Printf("Warning: EU routing skipped (model query failed): %v", err)
		return fmt.Errorf("model query failed: %w", err)
	}
	written := 0
	for _, m := range listed {
		if !listedIDs[m.ID] {
			continue
		}
		store.RecordEURouting(m.Name, eu[m.ID])
		written++
	}
	log.Printf("  ok EU routing derived for %d models (%d in the EU list)", written, len(eu))
	return nil
}
