package dbcore

import (
	"modelsdb/internal/paths"
	"path/filepath"
	"testing"
)

// retiredTableNames are the tables the floor schema creates and the migrations
// drop; a database at the current version holds none of them.
var retiredTableNames = []string{"ignored_models", "derived_columns", "derived_inputs"}

// assertNoRetiredTables fails for each retired table the open database holds.
func assertNoRetiredTables(t *testing.T, when string) {
	t.Helper()
	for _, name := range retiredTableNames {
		var held int
		if err := DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name=?", name).Scan(&held); err != nil {
			t.Fatalf("%s: look for %s: %v", when, name, err)
		}
		if held != 0 {
			t.Errorf("%s: the database holds the retired table %s", when, name)
		}
	}
}

// TestRetiredTablesNeverSurviveAnOpen covers a fresh install, a reopen of it, and
// a file an earlier build left holding the three tables empty.
func TestRetiredTablesNeverSurviveAnOpen(t *testing.T) {
	dir := t.TempDir()
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	t.Cleanup(func() {
		if DB != nil {
			DB.Close()
		}
	})

	if err := OpenDB(paths.DbFile); err != nil {
		t.Fatalf("fresh open: %v", err)
	}
	assertNoRetiredTables(t, "fresh install")
	DB.Close()

	if err := OpenDB(paths.DbFile); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	assertNoRetiredTables(t, "reopen at the current version")

	// The state of a v49 file: the three tables re-created empty.
	if _, err := DB.Exec(schemaSQL); err != nil {
		t.Fatalf("re-create the floor tables: %v", err)
	}
	if _, err := DB.Exec("PRAGMA user_version=49"); err != nil {
		t.Fatal(err)
	}
	DB.Close()
	if err := OpenDB(paths.DbFile); err != nil {
		t.Fatalf("open the v49 file: %v", err)
	}
	assertNoRetiredTables(t, "migrated from v49")
	if v, _ := currentSchemaVersion(); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
}

// TestRetiredTablesWithRowsAreNotDiscarded pins that the drop refuses a table
// that holds anything.
func TestRetiredTablesWithRowsAreNotDiscarded(t *testing.T) {
	setupTestDB(t)
	if err := migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(schemaSQL); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec("INSERT INTO ignored_models(kind, model_key) VALUES ('name','x')"); err != nil {
		t.Fatal(err)
	}
	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := theRetiredTablesAreDropped(tx); err == nil {
		t.Fatal("a retired table holding a row was dropped")
	}
}
