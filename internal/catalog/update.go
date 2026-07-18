// Package catalog handles update logic and functionalities.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	ApiUrl = "https://openrouter.ai/api/v1/models"
)

// NormalizeModel converts a model from the OpenRouter public API
// (/api/v1/models) into the internal shape the data layer expects. All
// knowledge of the upstream schema is isolated here:
//   - pricing nested under "endpoint.pricing"
//   - tool support derives from supported_parameters -> endpoint.supports_tool_parameters
//   - id kept as-is (collection join key), canonical_slug -> slug, hugging_face_id -> hf_slug
//   - architecture.{input,output}_modalities -> top-level
//   - created (unix seconds) -> created_at (ISO-8601)
func NormalizeModel(m map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"name":        shared.GetStr(m, "name"),
		"description": shared.GetStr(m, "description"),
		"id":          shared.GetStr(m, "id"),
	}

	if s := shared.GetStr(m, "canonical_slug"); s != "" {
		out["slug"] = s
	} else {
		out["slug"] = shared.GetStr(m, "id")
	}

	if hf := shared.GetStr(m, "hugging_face_id"); hf != "" {
		out["hf_slug"] = hf
	} else {
		out["hf_slug"] = nil
	}

	if c, ok := m["created"].(float64); ok && c > 0 {
		out["created_at"] = time.Unix(int64(c), 0).UTC().Format(time.RFC3339)
	}

	out["context_length"] = m["context_length"]

	if arch, ok := m["architecture"].(map[string]interface{}); ok {
		out["input_modalities"] = arch["input_modalities"]
		out["output_modalities"] = arch["output_modalities"]
	}

	supportsTool := false
	supportsReasoning := false
	if sp, ok := m["supported_parameters"].([]interface{}); ok {
		for _, p := range sp {
			switch fmt.Sprintf("%v", p) {
			case "tools", "tool_choice":
				supportsTool = true
			case "reasoning", "include_reasoning":
				supportsReasoning = true
			}
		}
	}
	out["supports_reasoning"] = supportsReasoning
	out["hidden"] = false

	endpoint := map[string]interface{}{
		"supports_tool_parameters": supportsTool,
	}
	if pr, ok := m["pricing"].(map[string]interface{}); ok {
		endpoint["pricing"] = pr
	}
	out["endpoint"] = endpoint

	return out
}

// FetchModels pulls the OpenRouter catalog and UPSERTs it into the DB: metadata
// is refreshed, curated fields and collection-derived capabilities are kept.
func FetchModels() error {
	log.Printf("=== Fetching models from OpenRouter ===")
	resp, err := http.Get(ApiUrl)
	if err != nil {
		return fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch models: unexpected status %s from %s", resp.Status, ApiUrl)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	var doc struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("parse models JSON (got %d bytes, content-type %q): %w", len(body), resp.Header.Get("Content-Type"), err)
	}

	// Snapshot existing names so we can report which models are new.
	existing := map[string]bool{}
	if rows, err := dbcore.DB.Query("SELECT name FROM models"); err == nil {
		for rows.Next() {
			var n string
			if rows.Scan(&n) == nil {
				existing[n] = true
			}
		}
		rows.Close()
	}

	var newNames []string
	count := 0
	for _, m := range doc.Data {
		norm := NormalizeModel(m)
		name := shared.GetStr(norm, "name")
		if name == "" || strings.Contains(strings.ToLower(name), "(free)") {
			continue
		}
		if !existing[name] {
			newNames = append(newNames, name)
		}
		if err := store.UpsertAPIModel(norm); err != nil {
			return fmt.Errorf("upsert %q: %w", name, err)
		}
		count++
	}
	log.Printf("  ok Upserted %d models (%d new)", count, len(newNames))

	// Derive Zero Data Retention from OpenRouter's frontend provider policies.
	// Fail-soft: a failure here leaves existing zdr values unchanged (handled
	// inside deriveZDR) and never aborts the refresh.
	if err := deriveZDR(); err != nil {
		log.Printf("Warning: ZDR derivation failed: %v", err)
	}

	// NOTE: FetchModels deliberately does NOT export the durable file. The
	// automatic startup refresh calls it, and startup must never overwrite a
	// curated.json the user may have restored. Callers that ARE a user-initiated
	// action (the CLI `update` command, the "Update models" refresh via
	// RunCollections/RunPricing) export explicitly. See "Data durability" in
	// CLAUDE.md.

	if len(newNames) > 0 {
		sort.Strings(newNames)
		log.Printf("New models (%d): %s", len(newNames), strings.Join(newNames, ", "))
	}
	return nil
}
