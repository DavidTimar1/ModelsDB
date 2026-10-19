package dbcore

import (
	"database/sql"
	"modelsdb/internal/paths"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A pre-migration snapshot holds writes that are still in the WAL, because it
// is made by SQLite and not by copying modelsdb.db, which can be behind them.
func TestPreMigrationSnapshotHoldsWritesStillInTheWAL(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('in the wal','manual','t')`); err != nil {
		t.Fatal(err)
	}
	bp, err := backupDB(0)
	if err != nil {
		t.Fatal(err)
	}
	ro, err := sql.Open("sqlite", "file:"+bp+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	var n int
	if err := ro.QueryRow("SELECT COUNT(*) FROM models WHERE name='in the wal'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("the snapshot holds %d copies of the WAL-resident row (err %v), want 1", n, err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(bp); info.Mode().Perm() != 0600 {
			t.Errorf("snapshot mode = %v, want 0600: it carries the reader's notes", info.Mode().Perm())
		}
	}
	left, _ := filepath.Glob(filepath.Join(paths.BackupsDir, "*.tmp"))
	if len(left) != 0 {
		t.Errorf("working files left behind: %v", left)
	}
}

func TestCopyFileIsCompleteAndKeepsAnExistingMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "fresh.db")
	if err := copyFile(src, fresh); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing.db")
	if err := os.WriteFile(existing, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, existing); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{fresh, existing} {
		if b, _ := os.ReadFile(p); string(b) != "payload" {
			t.Errorf("%s holds %q", p, b)
		}
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(fresh); info.Mode().Perm() != 0600 {
			t.Errorf("a new copy has mode %v, want 0600", info.Mode().Perm())
		}
		if info, _ := os.Stat(existing); info.Mode().Perm() != 0640 {
			t.Errorf("an overwritten copy has mode %v, want the old 0640", info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("folder holds %d entries, want src, fresh and existing only", len(entries))
	}
	if err := copyFile(filepath.Join(dir, "missing.db"), filepath.Join(dir, "x.db")); err == nil {
		t.Error("copying a missing file succeeded")
	}
}

func TestDBBackupsListOldestFirstAndOnlyDatabaseFiles(t *testing.T) {
	setupTestDB(t)
	if err := os.MkdirAll(paths.BackupsDir, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for name, age := range map[string]time.Duration{"new.db": 0, "old.db": 48 * time.Hour, "mid.db": 24 * time.Hour} {
		p := filepath.Join(paths.BackupsDir, name)
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(paths.BackupsDir, "note.txt"), []byte("x"), 0600)

	var got []string
	for _, e := range dbBackups() {
		got = append(got, e.Name())
	}
	if len(got) != 3 || got[0] != "old.db" || got[1] != "mid.db" || got[2] != "new.db" {
		t.Errorf("backups = %v, want old, mid, new", got)
	}
}

// A model count that cannot be read must stop the migration before it touches
// anything, never skip the safety backup and carry on.
func TestMigrationStopsWhenTheModelCountCannotBeRead(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec("DROP TABLE cells"); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec("DROP TABLE models"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(); err == nil {
		t.Fatal("migrate went on without being able to count the models")
	}
	if v, _ := currentSchemaVersion(); v != 0 {
		t.Errorf("user_version moved to %d although the migration was refused", v)
	}
}
