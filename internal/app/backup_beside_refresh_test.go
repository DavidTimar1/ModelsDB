package app

import (
	"database/sql"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"os"
	"strings"
	"testing"
)

// TestBackupOpensBesideARunningRefresh pins that the database open a backup
// makes writes nothing a server's write lock can refuse. A server holds that
// lock for long stretches while it refreshes the catalog; the seeding and source
// declarations a normal start makes fail at once with SQLITE_BUSY under it, and
// took the backup down with them.
func TestBackupOpensBesideARunningRefresh(t *testing.T) {
	migratedDB(t)
	// A running server has opened the file at its current version at least once,
	// which is the state a backup beside it finds.
	if err := dbcore.DB.Close(); err != nil {
		t.Fatalf("close the database: %v", err)
	}
	if err := openMigratedDB(); err != nil {
		t.Fatalf("reopen the database: %v", err)
	}
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
	if err := dbcore.DB.Close(); err != nil {
		t.Fatalf("close the database: %v", err)
	}

	// A second connection stands in for the server mid-refresh: it holds the
	// write lock and does not let go until the test ends.
	server, err := sql.Open("sqlite", paths.DbFile+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open the stand-in server connection: %v", err)
	}
	t.Cleanup(func() { server.Close() })
	server.SetMaxOpenConns(1)
	tx, err := server.Begin()
	if err != nil {
		t.Fatalf("begin the stand-in server transaction: %v", err)
	}
	t.Cleanup(func() { tx.Rollback() })
	if _, err := tx.Exec("INSERT INTO models(name) VALUES ('mid-refresh')"); err != nil {
		t.Fatalf("take the write lock: %v", err)
	}

	if err := openMigratedDB(); err != nil {
		t.Fatalf("open the database beside the lock holder: %v", err)
	}
	// The set-up is real only if the extra writes of a normal start do fail here.
	if _, err := store.DeclareShippedSources(); err == nil || !strings.Contains(err.Error(), "SQLITE_BUSY") {
		t.Fatalf("declaring sources beside the lock holder should be refused with SQLITE_BUSY, got %v", err)
	}

	dst, err := dbcore.BackupTo(t.TempDir(), dbcore.TimestampedBackup)
	if err != nil {
		t.Fatalf("back up beside the lock holder: %v", err)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		t.Fatalf("the backup at %s is missing or empty: %v", dst, err)
	}
}
