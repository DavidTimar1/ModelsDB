package store

import (
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"path/filepath"
	"testing"
)

// identityDB gives one test its own fully-migrated temp database, so nothing
// here reads or writes the real data dir.
func identityDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := dbcore.OpenDB(paths.DbFile); err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() {
		if dbcore.DB != nil {
			dbcore.DB.Close()
		}
	})
}

// addRow writes one model row and the cells carrying its identifiers, so a test
// can state exactly which identifiers a row carries.
func addRow(t *testing.T, name, id, hfSlug string) {
	t.Helper()
	if _, err := dbcore.DB.Exec(`INSERT INTO models (name, source, updated_at)
		VALUES (?,?,?)`, name, "openrouter", dbcore.NowStamp()); err != nil {
		t.Fatalf("insert %q: %v", name, err)
	}
	if _, err := SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
	if err := WriteCells(SourceOpenRouter, name, map[string]string{"id": id}); err != nil {
		t.Fatalf("record the id of %q: %v", name, err)
	}
	if err := WriteCells(SourceHuggingFace, name, map[string]string{"repository": hfSlug}); err != nil {
		t.Fatalf("record the repository of %q: %v", name, err)
	}
}

// TestFindExistingModelRecognisesARepositoryUnderAnotherName is the defect this
// lookup exists for: the catalog holds the model under OpenRouter's name for it,
// and the same repository arrives with a name derived from its address.
func TestFindExistingModelRecognisesARepositoryUnderAnotherName(t *testing.T) {
	identityDB(t)
	addRow(t, "inclusionAI: Ling 3.0 Tiny (free)", "inclusionai/ling-3.0-tiny:free", "inclusionAI/Ling-3.0-tiny")

	match, held, err := FindExistingModel("inclusionAI: Ling-3.0-tiny", "", "inclusionAI/Ling-3.0-tiny")
	if err != nil {
		t.Fatal(err)
	}
	if !held {
		t.Fatal("the repository was not recognised, so a second row would be created for it")
	}
	if match.Name != "inclusionAI: Ling 3.0 Tiny (free)" || match.By != MatchByRepository {
		t.Errorf("matched %q on %s, want the OpenRouter row on the repository", match.Name, match.By)
	}
}

// TestFindExistingModelMatchesTheRepositoryWhateverTheCapitals: one repository is
// written several ways by the people and the sources that name it, and two
// spellings of it are not two models.
func TestFindExistingModelMatchesTheRepositoryWhateverTheCapitals(t *testing.T) {
	identityDB(t)
	addRow(t, "Qwen: Qwen3 32B", "qwen/qwen3-32b", "Qwen/Qwen3-32B")

	match, held, err := FindExistingModel("qwen: qwen3-32b", "", "qwen/qwen3-32b")
	if err != nil {
		t.Fatal(err)
	}
	if !held || match.Name != "Qwen: Qwen3 32B" {
		t.Fatalf("held=%v matched %q, want the stored row", held, match.Name)
	}
}

// TestFindExistingModelMatchesByUpstreamIDAndByName covers the two weaker
// identifiers on their own: a row carrying no repository is still recognised by
// the id it was stored under, and a row carrying neither by its name.
func TestFindExistingModelMatchesByUpstreamIDAndByName(t *testing.T) {
	identityDB(t)
	addRow(t, "OpenAI: GPT-5", "openai/gpt-5", "")
	addRow(t, "My Local Build", "", "")

	byID, held, err := FindExistingModel("Something Else", "openai/gpt-5", "")
	if err != nil {
		t.Fatal(err)
	}
	if !held || byID.Name != "OpenAI: GPT-5" || byID.By != MatchByUpstreamID {
		t.Errorf("by id: held=%v match=%+v", held, byID)
	}

	byName, held, err := FindExistingModel("My Local Build", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !held || byName.Name != "My Local Build" || byName.By != MatchByName {
		t.Errorf("by name: held=%v match=%+v", held, byName)
	}
}

// TestFindExistingModelAsksTheRepositoryThenTheIDThenTheName proves the order,
// on a catalog where all three identifiers point at DIFFERENT rows. An
// identifier that survives an upstream rename outranks one that does not.
func TestFindExistingModelAsksTheRepositoryThenTheIDThenTheName(t *testing.T) {
	identityDB(t)
	addRow(t, "Row By Repository", "other/id", "vendor/model")
	addRow(t, "Row By ID", "vendor/model-id", "")
	addRow(t, "Row By Name", "", "")

	match, held, err := FindExistingModel("Row By Name", "vendor/model-id", "vendor/model")
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	if match.Name != "Row By Repository" {
		t.Errorf("matched %q on %s, want the repository to win", match.Name, match.By)
	}

	match, held, err = FindExistingModel("Row By Name", "vendor/model-id", "")
	if err != nil || !held {
		t.Fatalf("held=%v err=%v", held, err)
	}
	if match.Name != "Row By ID" {
		t.Errorf("matched %q on %s, want the id to win over the name", match.Name, match.By)
	}
}

// TestFindExistingModelNeverMatchesOnAnEmptyIdentifier is what stops the lookup
// reporting an unrelated model: most rows carry no repository and many carry no
// id, so a blank probe that matched would find the first of them.
func TestFindExistingModelNeverMatchesOnAnEmptyIdentifier(t *testing.T) {
	identityDB(t)
	addRow(t, "A Row With Nothing", "", "")

	if match, held, err := FindExistingModel("Some New Model", "", ""); err != nil || held {
		t.Fatalf("an empty repository and id matched %+v (held=%v, err=%v)", match, held, err)
	}
}

// TestFindExistingModelPrefersAModelToOneOfItsBuilds: a variation is one
// downloadable build filed under a model, so where both answer to an identifier
// the model is the row an import belongs on - and the answer must be the same on
// every run rather than whichever row SQLite reaches first.
func TestFindExistingModelPrefersAModelToOneOfItsBuilds(t *testing.T) {
	identityDB(t)
	addRow(t, "Vendor: Model", "", "vendor/model")
	addRow(t, "Vendor: Model (Q4_K_M)", "", "vendor/model")
	if _, err := dbcore.DB.Exec(`UPDATE models SET variant_of=? WHERE name=?`,
		"Vendor: Model", "Vendor: Model (Q4_K_M)"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		match, held, err := FindExistingModel("Vendor: Model-New", "", "vendor/model")
		if err != nil || !held {
			t.Fatalf("held=%v err=%v", held, err)
		}
		if match.Name != "Vendor: Model" {
			t.Fatalf("matched %q, want the model rather than its build", match.Name)
		}
	}
}
