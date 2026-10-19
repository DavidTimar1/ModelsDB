package dbcore

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/paths"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestCountCellsSeesCascadeLoss(t *testing.T) {
	setupTestDB(t)
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('M','manual','t')`); err != nil {
		t.Fatal(err)
	}
	colID := makeOwnColumn(t, "rating_of_mine", "Rating of mine")
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','hand','3','t')`, colID); err != nil {
		t.Fatal(err)
	}

	before, err := countCells()
	if err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("countCells before = %d, want 1", before)
	}

	res, err := DB.Exec("DELETE FROM models WHERE name='M'")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("deleted %d model rows, want 1", n)
	}
	after, err := countCells()
	if err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("countCells after deleting the model = %d, want 0 (the cascade is what makes the guard necessary)", after)
	}
}

// makeOwnColumn creates one column of the reader's own and answers its row
// number, so a test can store a value the guard counts. column_key must
// already be dot-free: post-v41, columns has no namespace column and its own
// CHECK refuses a dotted key. It also declares 'hand' as the column's one
// source in column_sources, because v41's cells_source_is_declared trigger
// refuses any cell whose source is not declared there.
func makeOwnColumn(t *testing.T, key, label string) int64 {
	t.Helper()
	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES (?,?,'text',?)`, key, label, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'hand',0)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// The two environment variables that make the abort test possible: with the
// first set, the test binary it is the child process of
// TestMigrateAbortsWhenStoredValuesWouldBeLost; migrateAbortDirEnv hands the
// child the parent's temp directory to work in, so the parent both inspects the
// result and cleans it up.
const (
	migrateAbortChildEnv = "MODELSDB_TEST_MIGRATE_ABORT_CHILD"
	migrateAbortDirEnv   = "MODELSDB_TEST_MIGRATE_ABORT_DIR"
)

// TestMigrateAbortsWhenStoredValuesWouldBeLost drives the guard itself: a
// migration that destroys stored values must abort and leave the pre-migration
// backup restored over the database, rather than commit the loss.
//
// The abort exits the process, which would end the whole test run, so the real
// work happens in a re-executed copy of this test binary and the parent checks
// what it left behind. The registered migrations cannot destroy anything, so the
// child swaps in a deliberately destructive one - the guard has to hold for the
// migration nobody has written yet, not only for today's.
func TestMigrateAbortsWhenStoredValuesWouldBeLost(t *testing.T) {
	if os.Getenv(migrateAbortChildEnv) == "1" {
		runMigrateAbortChild()
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMigrateAbortsWhenStoredValuesWouldBeLost$")
	cmd.Env = append(os.Environ(), migrateAbortChildEnv+"=1", migrateAbortDirEnv+"="+dir)
	out, cmdErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(cmdErr, &exitErr) {
		t.Fatalf("the destructive migration should have exited non-zero, got err=%v; output:\n%s", cmdErr, out)
	}
	if !strings.Contains(string(out), "stored values dropped from 1 to 0 cells") {
		t.Errorf("the abort should name the loss it detected; output:\n%s", out)
	}
	if !strings.Contains(string(out), "pre-migration backup was restored") {
		t.Errorf("the abort should report restoring the backup; output:\n%s", out)
	}

	// The database left on disk must be the pre-migration one: the model row and
	// the value stored about it are both back.
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := openConn(paths.DbFile); err != nil {
		t.Fatalf("reopen the restored DB: %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			DB.Close()
		}
	})
	if n, err := countCells(); err != nil || n != 1 {
		t.Errorf("restored DB holds %d stored values (err=%v), want 1", n, err)
	}
	if n, _ := CountModels(); n != 1 {
		t.Errorf("restored DB holds %d models, want 1", n)
	}
}

// runMigrateAbortChild is the child half of
// TestMigrateAbortsWhenStoredValuesWouldBeLost. It builds a current database
// holding one model and one custom-column value, then re-runs migrate() with the
// registry swapped for a single migration that wipes the models table - and so
// never returns, because the guard aborts the process.
func runMigrateAbortChild() {
	dir := os.Getenv(migrateAbortDirEnv)
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := openConn(paths.DbFile); err != nil {
		panic(err)
	}
	if err := migrate(); err != nil {
		panic(err)
	}
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('M','manual','t')`); err != nil {
		panic(err)
	}
	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('rating_of_mine','Rating of mine','text','t')`)
	if err != nil {
		panic(err)
	}
	colID, err := res.LastInsertId()
	if err != nil {
		panic(err)
	}
	if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'hand',0)`, colID); err != nil {
		panic(err)
	}
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','hand','3','t')`, colID); err != nil {
		panic(err)
	}

	// Roll the recorded version back one step and offer a destructive migration
	// in place of the real one, so migrate() has something pending to apply.
	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion-1)); err != nil {
		panic(err)
	}
	migrations = []migration{{version: schemaVersion, apply: func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM models")
		return err
	}}}
	if err := migrate(); err != nil {
		panic(err)
	}
	panic("migrate() returned; the stored-value guard did not abort a destructive migration")
}

// migrateThrough runs only the registered migrations up to and including
// target, then restores the full registry - so a test written against one
// migration's own output (a table a LATER migration goes on to drop, such as
// derived_inputs) can still inspect that output directly, even though a real
// carried-forward database runs every migration in one unbroken migrate()
// call all the way to schemaVersion.
func migrateThrough(t *testing.T, target int) {
	t.Helper()
	original := migrations
	limited := make([]migration, 0, len(original))
	for _, m := range original {
		if m.version <= target {
			limited = append(limited, m)
		}
	}
	migrations = limited
	t.Cleanup(func() { migrations = original })
	if err := migrate(); err != nil {
		t.Fatalf("migrate through v%d: %v", target, err)
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
