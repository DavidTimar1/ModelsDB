package catalog

import (
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"path/filepath"
	"testing"
)

// catalogTestDB gives one test its own fully-migrated temp database, so a
// catalog test's store reads and writes touch nothing on this machine.
func catalogTestDB(t *testing.T) {
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
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
}

// TestFeedColumnMappingTargetsKnownColumns proves every column key named in
// feedPathColumns is a column the store actually ships, so a typo here would
// otherwise show a shipped path as "New" or "Not imported" forever instead of
// "Column: <label>".
func TestFeedColumnMappingTargetsKnownColumns(t *testing.T) {
	catalogTestDB(t)
	for path, key := range feedPathColumns {
		if _, err := store.ColumnByKey(key); err != nil {
			t.Errorf("feedPathColumns[%q] = %q, which is not a column the store knows: %v", path, key, err)
		}
	}
}
