package app

import (
	"errors"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/url"
	"testing"
)

// TestEveryStoredRowIsReachableThroughTheAPI is the check that caught the defect
// it exists to prevent: count the rows in the models table, then count the rows
// GET /api/models?include_hidden=1 answers with, and require the two to be the
// same number.
//
// That comparison is the whole point. A row the API cannot list is a row
// POST /api/save answers 404 for and the page cannot draw, so it exists and no
// verb in this app can reach it - and nothing says so, because a shorter list
// looks exactly like a smaller catalog. The rows below are the shapes that were
// silently dropped once: a name carrying "(free)", and a hidden row.
func TestEveryStoredRowIsReachableThroughTheAPI(t *testing.T) {
	migratedDB(t)

	for _, name := range []string{
		"OpenAI: GPT-5",
		"Meta: Llama 3.3 70B Instruct (free)",
		"NVIDIA: Nemotron 3 Ultra (Free)",
		"Google: Gemma 4 26B A4B ", // upstream really does ship a trailing space
		"A build somebody hid",
	} {
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
	}
	if _, err := dbcore.DB.Exec("UPDATE models SET hidden=1 WHERE name=?", "A build somebody hid"); err != nil {
		t.Fatalf("hide a row: %v", err)
	}

	stored, err := dbcore.CountModels()
	if err != nil {
		t.Fatalf("CountModels: %v", err)
	}
	rows, err := processModels()
	if err != nil {
		t.Fatalf("processModels: %v", err)
	}
	served := filtered(t, rows, url.Values{"include_hidden": {"1"}})
	if len(served) != stored {
		t.Fatalf("the API answered with %d rows and the models table holds %d - %d row(s) are stored and unreachable",
			len(served), stored, stored-len(served))
	}

	seen := map[string]bool{}
	for _, r := range served {
		seen[shared.GetStr(r, "name")] = true
	}
	for _, name := range []string{"Meta: Llama 3.3 70B Instruct (free)", "NVIDIA: Nemotron 3 Ultra (Free)", "A build somebody hid"} {
		if !seen[name] {
			t.Errorf("%q is stored but the API did not return it", name)
		}
	}
}

// TestAHiddenRowIsWithheldOnlyUntilTheRequestAsksForIt is the other half: the
// row count above must not be bought by serving hidden rows to a caller that
// never heard of include_hidden. Hiding stays a FILTER the request chooses, and
// never a reason a row cannot be reached at all.
func TestAHiddenRowIsWithheldOnlyUntilTheRequestAsksForIt(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("A build somebody hid", map[string]string{}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if _, err := dbcore.DB.Exec("UPDATE models SET hidden=1 WHERE name=?", "A build somebody hid"); err != nil {
		t.Fatalf("hide a row: %v", err)
	}
	rows, err := processModels()
	if err != nil {
		t.Fatalf("processModels: %v", err)
	}
	if got := len(filtered(t, rows, url.Values{})); got != 0 {
		t.Errorf("the default answer carried %d hidden row(s), want 0", got)
	}
	if got := len(filtered(t, rows, url.Values{"include_hidden": {"1"}})); got != 1 {
		t.Errorf("include_hidden=1 answered with %d rows, want 1", got)
	}
}

// TestNoPathCreatesARowWithNoName is the decision the reachability rule forces.
// A row is addressed by its name and by nothing else, so a row stored under an
// empty name could never be read back, written to, hidden or deleted - it would
// be exactly the unreachable row this app must not hold. Every path that creates
// a row therefore refuses one, rather than the read path dropping it afterwards.
func TestNoPathCreatesARowWithNoName(t *testing.T) {
	migratedDB(t)

	for _, name := range []string{"", "   "} {
		if err := store.AddModelWithCells(name, map[string]string{}); !errors.Is(err, store.ErrNoModelName) {
			t.Errorf("AddModel(%q) = %v, want ErrNoModelName", name, err)
		}
		if _, err := store.UpsertAPIModel(map[string]interface{}{"name": name}); !errors.Is(err, store.ErrNoModelName) {
			t.Errorf("UpsertAPIModel(%q) = %v, want ErrNoModelName", name, err)
		}
		if err := store.ImportFullRecord(map[string]interface{}{"name": name}, "huggingface", "chat"); !errors.Is(err, store.ErrNoModelName) {
			t.Errorf("ImportFullRecord(%q) = %v, want ErrNoModelName", name, err)
		}
		if err := store.ImportVariation(store.Variation{Parent: "OpenAI: GPT-5", Name: name}); !errors.Is(err, store.ErrNoModelName) {
			t.Errorf("ImportVariation(%q) = %v, want ErrNoModelName", name, err)
		}
	}

	if n, err := dbcore.CountModels(); err != nil || n != 0 {
		t.Errorf("the table holds %d row(s) after every nameless write (err %v), want 0", n, err)
	}
}
