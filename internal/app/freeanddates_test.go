package app

import (
	"modelsdb/internal/store"
	"strings"
	"testing"
)

// aCatalogOfFreeAndDatedModels stores the models the hide_free and date-range
// conditions are asked about: a free variant by id, one by display name alone,
// a model priced 0 per token that is NOT a free variant, a paid model, and
// models carrying each shape created_at is stored in or no date at all.
func aCatalogOfFreeAndDatedModels(t *testing.T) {
	t.Helper()
	migratedDB(t)
	write := func(name string, values map[string]string) {
		t.Helper()
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
		if err := store.WriteCells(store.SourceOpenRouter, name, values); err != nil {
			t.Fatalf("write cells on %q: %v", name, err)
		}
	}
	write("Llama Free By Id", map[string]string{
		"id": "meta/llama:free", "price_prompt": "0", "price_completion": "0",
		"created_at": "2026-06-16T17:45:30Z",
	})
	write("Poolside Laguna (free)", map[string]string{
		"price_prompt": "0", "price_completion": "0",
		"created_at": "2026-10-10T23:59:59Z",
	})
	write("Flux Image", map[string]string{
		"id": "black/flux", "price_prompt": "0", "price_completion": "0", "model_type": "image",
		"created_at": "2022-03-02T23:29:04.000Z",
	})
	write("Claude Opus", map[string]string{
		"id": "anthropic/claude-opus", "price_prompt": "0.000003",
		"created_at": "2026-10-11T00:00:00Z", "knowledge_cutoff": "2025-03-01",
	})
	write("Undated", map[string]string{"id": "x/undated"})
}

func TestHideFreeDropsFreeVariantsByIdAndByName(t *testing.T) {
	aCatalogOfFreeAndDatedModels(t)

	got := queried(t, "?hide_free=1")
	want := []string{"Claude Opus", "Undated"}
	if !eqStr(got, want) {
		t.Errorf("hide_free=1 served %v, want %v: the :free id, the (free) name and the 0 prices go, a model with no price stays", got, want)
	}
	if all := queried(t, "?hide_free=0"); len(all) != 5 {
		t.Errorf("hide_free=0 served %v, want all five models", all)
	}
	if both := queried(t, "?hide_free=1&price_prompt=max:0.00001"); !eqStr(both, []string{"Claude Opus"}) {
		t.Errorf("hide_free beside a price condition served %v, want both to apply", both)
	}
}

// TestHideFreeNamesTheColumnsWhoseZeroPriceIsHidden holds the per-column tick:
// each named column hides its own 0, a free variant goes with any tick, a
// model with no figure is kept, and the old hide_free=1 means both columns.
func TestHideFreeNamesTheColumnsWhoseZeroPriceIsHidden(t *testing.T) {
	aCatalogOfFreeAndDatedModels(t)
	for name, values := range map[string]map[string]string{
		"Free Output Only": {"price_prompt": "0.000001", "price_completion": "0"},
		"Free Input Only":  {"price_prompt": "0.0", "price_completion": "0.000002"},
		"Paid Both":        {"price_prompt": "0.000001", "price_completion": "0.000002"},
	} {
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
		if err := store.WriteCells(store.SourceOpenRouter, name, values); err != nil {
			t.Fatalf("write cells on %q: %v", name, err)
		}
	}

	tests := []struct {
		about string
		query string
		want  []string
	}{
		{"In alone hides a 0 In and the free variants", "?hide_free=price_prompt", []string{"Claude Opus", "Free Output Only", "Paid Both", "Undated"}},
		{"Out alone hides a 0 Out, a model with no Out is kept", "?hide_free=price_completion", []string{"Claude Opus", "Free Input Only", "Paid Both", "Undated"}},
		{"both columns named hide either 0", "?hide_free=price_prompt,price_completion", []string{"Claude Opus", "Paid Both", "Undated"}},
		{"the old 1 means both", "?hide_free=1", []string{"Claude Opus", "Paid Both", "Undated"}},
		{"true and yes are 1", "?hide_free=yes", []string{"Claude Opus", "Paid Both", "Undated"}},
		{"it is ANDed with the column's own max", "?hide_free=price_completion&price_completion=max:0.000001", []string{}},
		{"off words hide nothing", "?hide_free=false", []string{"Claude Opus", "Flux Image", "Free Input Only", "Free Output Only", "Llama Free By Id", "Paid Both", "Poolside Laguna (free)", "Undated"}},
	}
	for _, tc := range tests {
		got := queried(t, tc.query)
		if !eqStr(got, tc.want) {
			t.Errorf("%s: %s served %v, want %v", tc.about, tc.query, got, tc.want)
		}
	}

	if msg := refused(t, "?hide_free=model_type"); !strings.Contains(msg, "model_type") {
		t.Errorf("hide_free naming a column of text answered %q, want a refusal naming it", msg)
	}
	if msg := refused(t, "?hide_free=nonsense"); !strings.Contains(msg, "nonsense") {
		t.Errorf("hide_free naming no column answered %q, want a refusal naming it", msg)
	}
}

func TestADateRangeIsInclusiveAndReadsBothStoredShapes(t *testing.T) {
	aCatalogOfFreeAndDatedModels(t)

	tests := []struct {
		about string
		query string
		want  []string
	}{
		{"to is inclusive of the whole last day", "?created_at=to:2026-10-10", []string{"Flux Image", "Llama Free By Id", "Poolside Laguna (free)"}},
		{"from is inclusive of the first day", "?created_at=from:2026-10-10", []string{"Claude Opus", "Poolside Laguna (free)"}},
		{"a range is both ends", "?created_at=from:2026-06-16&created_at=to:2026-06-16", []string{"Llama Free By Id"}},
		{"the milliseconds shape compares by its day", "?created_at=from:2022-03-02&created_at=to:2022-03-02", []string{"Flux Image"}},
		{"a plain date column", "?knowledge_cutoff=from:2025-03-01", []string{"Claude Opus"}},
		{"a model with no date drops out of a range", "?created_at=from:1970-01-01", []string{"Claude Opus", "Flux Image", "Llama Free By Id", "Poolside Laguna (free)"}},
	}
	for _, tt := range tests {
		t.Run(tt.about, func(t *testing.T) {
			if got := queried(t, tt.query); !eqStr(got, tt.want) {
				t.Errorf("GET /api/models%s served %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestADateRangeRefusesWhatItCannotRead(t *testing.T) {
	aCatalogOfFreeAndDatedModels(t)

	for _, query := range []string{
		"?created_at=from:2026-1-5",
		"?created_at=to:yesterday",
		"?created_at=from:",
		"?created_at=to:2026-10-10T23:59:59Z",
		"?created_at=from:2026-13-01",
		"?price_prompt=from:2026-01-01",
		"?model_type=to:2026-01-01",
	} {
		if body := refused(t, query); !strings.Contains(body, "from:") && !strings.Contains(body, "to:") {
			t.Errorf("GET /api/models%s was refused without naming the test: %s", query, body)
		}
	}
}
