package app

import (
	"net/http"
	"testing"

	"modelsdb/internal/store"
)

// Reading a model's repository again, asked for from the model's own details.
//
// THE QUESTION THIS FILE ASKS IS "WHAT ELSE MOVED". The details dialog names
// the columns a re-import will overwrite before it sends anything, and that
// promise is only worth making if it is the whole truth - so this asserts over
// EVERY cell the row holds rather than over a handful of named ones: the
// repository's own columns are rewritten, and every other cell in the catalog
// is byte for byte what it was.

// cellsOfModel is every value one model holds, keyed by column.
func cellsOfModel(t *testing.T, name string) map[string]string {
	t.Helper()
	cells, err := store.ModelCells(name)
	if err != nil {
		t.Fatalf("read the cells of %q: %v", name, err)
	}
	return cells
}

// TestAReimportRewritesTheRepositorysColumnsAndNothingElse is the promise the
// re-import panel makes, checked over the whole row: a note, a favourite star
// and a value in a column of the reader's own are exactly as they were, and the
// repository's own columns hold what the repository states.
func TestAReimportRewritesTheRepositorysColumnsAndNothingElse(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	// A model imported once, then written on by hand the way somebody uses this
	// app: a note, a star, a column they invented, and a researched figure on a
	// column the repository did not state.
	if rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("the first import answered %d: %s", rec.Code, rec.Body.String())
	}
	mine, err := ownColumnKey(t, "How it felt", store.ValueTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCells(testImportName, map[string]string{
		"notes":             "my own note, in my own words",
		"favorite":          "yes",
		"active_parameters": "1.5",
		mine:                "quick on my machine",
	}); err != nil {
		t.Fatal(err)
	}

	before := cellsOfModel(t, testImportName)
	if before["repository"] != testImportRepo {
		t.Fatalf("the row was not imported from %s: %v", testImportRepo, before["repository"])
	}

	// The repository is emptied of one of its own figures and re-read, so the
	// re-import has something to change as well as something to leave alone.
	if err := store.WriteCells(store.SourceHuggingFace, testImportName,
		map[string]string{"weights_size_gb": "1"}); err != nil {
		t.Fatal(err)
	}

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`","reimport":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the re-import answered %d, want 200: %s", rec.Code, rec.Body.String())
	}

	after := cellsOfModel(t, testImportName)

	// stateHuggingFace answers whether the repository holds the winning answer
	// for that key - the one thing a re-import is allowed to move.
	stateHuggingFace := func(key string) bool {
		t.Helper()
		answers, err := store.AnswersFor(testImportName, key)
		if err != nil {
			t.Fatalf("read who answers %s: %v", key, err)
		}
		return answers.Winner == string(store.SourceHuggingFace)
	}

	// Every cell that is NOT one the repository itself won is exactly as it was,
	// and none has appeared or gone.
	for key, was := range before {
		if stateHuggingFace(key) {
			continue
		}
		if after[key] != was {
			t.Errorf("%s is %q after the re-import, want the stored %q untouched", key, after[key], was)
		}
	}
	for key := range after {
		if stateHuggingFace(key) {
			continue
		}
		if _, held := before[key]; !held {
			t.Errorf("the re-import created %s, and it writes the repository's columns only", key)
		}
	}

	// The personal answers the panel names, asked one by one so a failure says
	// which of them moved.
	for key, want := range map[string]string{
		"notes":    "my own note, in my own words",
		"favorite": "yes",
		mine:       "quick on my machine",
	} {
		if after[key] != want {
			t.Errorf("%s = %q after the re-import, want %q", key, after[key], want)
		}
	}
	// A researched figure on a column the repository does not state is kept;
	// the re-import promise says so.
	if after["active_parameters"] != "1.5" {
		t.Errorf("active_parameters is %q, want the researched 1.5 kept where the repository states nothing", after["active_parameters"])
	}

	// And what the importer MEASURES was read again: the figure that was ruined
	// above is the repository's own again.
	if after["weights_size_gb"] == "1" || after["weights_size_gb"] == "" {
		t.Errorf("the repository's weights size is %q, want it read again", after["weights_size_gb"])
	}
	if after["repository"] != testImportRepo {
		t.Errorf("the repository column is %q, want %q", after["repository"], testImportRepo)
	}
}
