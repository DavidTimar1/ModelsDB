package app

import (
	"net/http"
	"strings"
	"testing"

	"modelsdb/internal/store"
)

// everyWritableColumn is one valid value per column POST /api/save accepts on
// an empty cell, so a test can offer each of them to a model and read the
// answer. The values are deliberately harmless - they are written and then
// read back only to learn whether the write was allowed at all.
var everyWritableColumn = []struct{ column, send string }{
	{"id", `"vendor/one"`},
	{"slug", `"vendor/one-v1"`},
	{"repository", `"Vendor/One"`},
	{"description", `"a description"`},
	{"created_at", `"2026-01-01T00:00:00Z"`},
	{"model_type", `"chat"`},
	{"context_length", `"8192"`},
	{"input_modalities", `"[\"text\"]"`},
	{"output_modalities", `"[\"text\"]"`},
	{"supports_reasoning", `"yes"`},
	{"price_prompt", `"0.000001"`},
	{"price_completion", `"0.000002"`},
	{"price_image", `"0.01"`},
	{"price_audio", `"0.02"`},
	{"price_internal_reasoning", `"0.03"`},
	{"price_unit", `"per 1M tokens"`},
	{"notes", `"a note"`},
	{"favorite", `"yes"`},
	{"supports_tools", `"yes"`},
	{"moe", `"no"`},
}

// TestNoEmptyColumnIsRefusedOnAnyModel is what a reader of this API can rely
// on when deciding whether a value is worth researching: on a row nothing
// automatic has described, every column is accepted, whatever route created
// the row. The one refusal is the rule itself: a column an automatic source
// already states on this model, which is refused naming that source.
func TestNoEmptyColumnIsRefusedOnAnyModel(t *testing.T) {
	migratedDB(t)

	// Two rows nothing automatic has filled: one imported from a repository
	// that stated only its own address, and one added by hand.
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Acme: Video One", "hf_slug": "acme/video-one",
	}, "huggingface", "video"); err != nil {
		t.Fatal(err)
	}
	if rec := addModel(t, `{"name":"Manual: Only Here","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	for _, name := range []string{"Acme: Video One", "Manual: Only Here"} {
		for _, c := range everyWritableColumn {
			// The repository stated the type and the address of the imported row.
			if name == "Acme: Video One" && (c.column == "model_type" || c.column == "repository") {
				continue
			}
			rec := saveModels(t, `[{"name":"`+name+`","cells":{"`+c.column+`":`+c.send+`}}]`)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: writing %s answered %d, want it accepted: %s",
					name, c.column, rec.Code, rec.Body.String())
			}
		}
	}

	// A row OpenRouter describes refuses the columns OpenRouter states, and
	// says so.
	if _, err := store.UpsertAPIModel(map[string]interface{}{
		"name": "Vendor: Upstream", "id": "vendor/upstream",
	}); err != nil {
		t.Fatal(err)
	}
	rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"id":"vendor/other"}}]`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "OpenRouter's answer, vendor/upstream") {
		t.Errorf("typing over OpenRouter's id answered %d, want 400 naming OpenRouter's value: %s", rec.Code, rec.Body.String())
	}
}
