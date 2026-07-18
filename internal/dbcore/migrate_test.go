package dbcore

import (
	"fmt"
	"modelsdb/internal/paths"
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB points the global path vars at a temp dir and opens a fresh DB
// connection (base schema, no migration) for migration/backup tests.
func setupTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := openConn(paths.DbFile); err != nil {
		t.Fatalf("openConn: %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			DB.Close()
		}
	})
}

func TestMigrateFreshSetsVersion(t *testing.T) {
	setupTestDB(t)
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	v, err := currentSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	// Idempotent: a second migrate is a no-op and must not error.
	if err := migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateBacksUpNonEmptyDB(t *testing.T) {
	setupTestDB(t)
	// A pre-versioning DB: user_version stays 0 and it already holds data.
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('x','manual','t')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dbBackups()) == 0 {
		t.Fatal("expected a pre-migration backup for a non-empty DB, found none")
	}
	if v, _ := currentSchemaVersion(); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
}

func TestRestoreFromBackupRoundTrips(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('a','manual','t')`); err != nil {
		t.Fatal(err)
	}
	bp, err := backupDB(0)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('b','manual','t')`); err != nil {
		t.Fatal(err)
	}
	if n, _ := CountModels(); n != 2 {
		t.Fatalf("pre-restore count = %d, want 2", n)
	}
	if err := restoreFromBackup(bp); err != nil {
		t.Fatalf("restoreFromBackup: %v", err)
	}
	if n, _ := CountModels(); n != 1 {
		t.Fatalf("post-restore count = %d, want 1 (backup had only 'a')", n)
	}
}

func TestPruneBackupsKeepsCap(t *testing.T) {
	setupTestDB(t)
	if err := os.MkdirAll(paths.BackupsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < paths.MaxBackupsToKeep+5; i++ {
		f := filepath.Join(paths.BackupsDir, fmt.Sprintf("modelsdb-v0-x%04d.db", i))
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups()
	if got := len(dbBackups()); got != paths.MaxBackupsToKeep {
		t.Fatalf("after prune: %d backups, want %d", got, paths.MaxBackupsToKeep)
	}
}
