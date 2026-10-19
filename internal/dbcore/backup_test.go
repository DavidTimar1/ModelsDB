package dbcore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modelsdb/internal/paths"
)

// openCopy opens a backup file as its own database, separate from the package's
// global connection, so a test can read what was actually written to disk.
func openCopy(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open the backup at %s: %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// dirEntries lists the plain filenames in a directory.
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// fileStamp is what a file looks like from the outside: whether it is there, how
// big it is, and when it was last written. Anything that writes to the file
// moves at least one of the three.
type fileStamp struct {
	absent bool
	size   int64
	mod    time.Time
}

// stampOf reads a file's stamp. A file that is not there is a valid stamp, not a
// failure: the modelsdb.db-wal sidecar exists only while a connection is open.
func stampOf(t *testing.T, path string) fileStamp {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileStamp{absent: true}
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	return fileStamp{size: info.Size(), mod: info.ModTime()}
}

// assertUnchanged fails when a file is not exactly as it was when before was
// taken, naming which of the three properties moved.
func assertUnchanged(t *testing.T, path string, before fileStamp) {
	t.Helper()
	after := stampOf(t, path)
	switch {
	case before.absent != after.absent:
		t.Errorf("the backup changed whether %s exists (was absent=%v, now absent=%v)", path, before.absent, after.absent)
	case after.absent:
		// Absent before and after, so there is nothing to compare.
	case before.size != after.size:
		t.Errorf("the backup changed the size of %s from %d to %d bytes", path, before.size, after.size)
	case !before.mod.Equal(after.mod):
		t.Errorf("the backup changed the modification time of %s from %v to %v", path, before.mod, after.mod)
	}
}

// fillWAL commits rows without checkpointing, so the write-ahead log holds data
// that modelsdb.db does not.
func fillWAL(t *testing.T, rows int) {
	t.Helper()
	for i := 0; i < rows; i++ {
		if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES (?,'manual','t')`,
			fmt.Sprintf("model-%04d", i)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
}

// TestBackupLeavesTheLiveDatabaseUntouched is the promise the whole engine rests
// on: taking a backup is a pure read. SQLite's own VACUUM INTO writes a separate
// file, so neither modelsdb.db nor the modelsdb.db-wal sidecar holding the newest
// writes may move by a byte or a second.
func TestBackupLeavesTheLiveDatabaseUntouched(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 200)

	wal := paths.DbFile + "-wal"
	if s := stampOf(t, wal); s.absent || s.size == 0 {
		t.Fatalf("test setup: %s should hold the uncheckpointed writes, got %+v", wal, s)
	}
	dbBefore := stampOf(t, paths.DbFile)
	walBefore := stampOf(t, wal)

	if _, err := BackupTo(t.TempDir(), TimestampedBackup); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}

	assertUnchanged(t, paths.DbFile, dbBefore)
	assertUnchanged(t, wal, walBefore)
}

// TestBackupCarriesTheSchemaVersion pins PRAGMA user_version across the copy. A
// backup that reset it to 0 would open as a pre-versioning database and be
// migrated from scratch when restored, running every migration again over data
// that has already had them.
func TestBackupCarriesTheSchemaVersion(t *testing.T) {
	setupTestDB(t)
	// PRAGMA user_version takes no placeholders; the value is our own constant.
	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	fillWAL(t, 5)

	path, err := BackupTo(t.TempDir(), TimestampedBackup)
	if err != nil {
		t.Fatalf("BackupTo: %v", err)
	}

	var got int
	if err := openCopy(t, path).QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		t.Fatalf("read user_version from the backup: %v", err)
	}
	if got != schemaVersion {
		t.Errorf("PRAGMA user_version in the backup = %d, want %d", got, schemaVersion)
	}
}

// TestBackupWritesNothingWhenTheDatabaseCannotBeRead pins the refusal: a backup
// that cannot be produced whole produces no file at all, not a plausible-looking
// partial one, and leaves no working file behind either.
func TestBackupWritesNothingWhenTheDatabaseCannotBeRead(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 1)
	dir := t.TempDir()

	// A closed database cannot be read, which is the failure the guard is for.
	if err := DB.Close(); err != nil {
		t.Fatalf("close the database: %v", err)
	}

	path, err := BackupTo(dir, TimestampedBackup)
	if err == nil {
		t.Fatalf("a backup that could not be taken must return an error, got the file %q", path)
	}
	if names := dirEntries(t, dir); len(names) != 0 {
		t.Errorf("a refused backup must leave the folder empty, found %v", names)
	}
}

// TestBackupFailureLeavesThePreviousBackupIntact covers the case that costs the
// most: in overwriting mode the destination name already holds the user's only
// backup, so a run that fails must not have touched it.
func TestBackupFailureLeavesThePreviousBackupIntact(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 20)
	want, err := CountModels()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	good, err := BackupTo(dir, OverwritingBackup)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}

	if err := DB.Close(); err != nil {
		t.Fatalf("close the database: %v", err)
	}
	if _, err := BackupTo(dir, OverwritingBackup); err == nil {
		t.Fatal("the second backup should have failed once the database was closed")
	}

	if names := dirEntries(t, dir); len(names) != 1 {
		t.Errorf("the folder should still hold only the earlier backup, found %v", names)
	}
	copyDB := openCopy(t, good)
	var integrity string
	if err := copyDB.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatalf("integrity check on the earlier backup: %v", err)
	}
	if integrity != "ok" {
		t.Errorf("PRAGMA integrity_check on the earlier backup = %q, want ok", integrity)
	}
	var got int
	if err := copyDB.QueryRow("SELECT COUNT(*) FROM models").Scan(&got); err != nil {
		t.Fatalf("count models in the earlier backup: %v", err)
	}
	if got != want {
		t.Errorf("the earlier backup holds %d models, it held %d before the failed run", got, want)
	}
}

// TestSynchronousSurvivesAnInterruptedBackup checks the setting the backup's
// durability rests on. SQLite documents the output of a VACUUM INTO interrupted
// by a power cut as possibly incomplete UNLESS the source database's synchronous
// setting is NORMAL (1) or FULL (2). Nothing in this app sets it, so this pins
// the value it is actually opened with rather than assuming a default holds.
func TestSynchronousSurvivesAnInterruptedBackup(t *testing.T) {
	setupTestDB(t)
	var mode int
	if err := DB.QueryRow("PRAGMA synchronous").Scan(&mode); err != nil {
		t.Fatalf("read PRAGMA synchronous: %v", err)
	}
	if mode != 1 && mode != 2 {
		t.Errorf("PRAGMA synchronous = %d, want 1 (NORMAL) or 2 (FULL); a backup interrupted by a power cut could otherwise be silently incomplete", mode)
	}
}

// TestBackupCopiesWritesStillSittingInTheWAL is the guarantee the whole backup
// exists for. The rows below are committed but never checkpointed by the test, so
// they live in the modelsdb.db-wal sidecar and are absent from modelsdb.db. A
// backup that copied the main file as it stands would open cleanly and be missing
// them, with nothing to say so.
func TestBackupCopiesWritesStillSittingInTheWAL(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 50)
	want, err := CountModels()
	if err != nil {
		t.Fatal(err)
	}
	if want == 0 {
		t.Fatal("test setup: the database should hold rows before the backup")
	}

	dir := t.TempDir()
	path, err := BackupTo(dir, TimestampedBackup)
	if err != nil {
		t.Fatalf("BackupTo: %v", err)
	}

	copyDB := openCopy(t, path)
	var got int
	if err := copyDB.QueryRow("SELECT COUNT(*) FROM models").Scan(&got); err != nil {
		t.Fatalf("count models in the backup: %v", err)
	}
	if got != want {
		t.Errorf("the backup holds %d models, the live database holds %d", got, want)
	}

	var integrity string
	if err := copyDB.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatalf("integrity check on the backup: %v", err)
	}
	if integrity != "ok" {
		t.Errorf("PRAGMA integrity_check on the backup = %q, want ok", integrity)
	}
}

// TestBackupNamingChoosesAddOrReplace covers both naming modes: timestamped
// leaves one more file per run, overwriting keeps exactly one.
func TestBackupNamingChoosesAddOrReplace(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('x','manual','t')`); err != nil {
		t.Fatal(err)
	}

	overwrite := t.TempDir()
	first, err := BackupTo(overwrite, OverwritingBackup)
	if err != nil {
		t.Fatalf("first overwriting backup: %v", err)
	}
	second, err := BackupTo(overwrite, OverwritingBackup)
	if err != nil {
		t.Fatalf("second overwriting backup: %v", err)
	}
	if first != second {
		t.Errorf("overwriting backups should reuse one name, got %q then %q", first, second)
	}
	if names := dirEntries(t, overwrite); len(names) != 1 {
		t.Errorf("overwriting mode should leave 1 file, found %v", names)
	}

	stamped := t.TempDir()
	if _, err := BackupTo(stamped, TimestampedBackup); err != nil {
		t.Fatalf("first timestamped backup: %v", err)
	}
	// fsStamp has one-second resolution, so a second run inside the same second
	// reuses the name. Naming is what this half checks, not file count.
	name := backupFileName(TimestampedBackup)
	if !strings.HasPrefix(name, backupFilePrefix+"-") || !strings.HasSuffix(name, ".db") {
		t.Errorf("timestamped backup name = %q, want %s-<stamp>.db", name, backupFilePrefix)
	}
	if name == backupFileName(OverwritingBackup) {
		t.Error("the two naming modes must not produce the same filename")
	}
}

// TestBackupLeavesEverythingElseInTheFolderAlone pins the promise that the
// destination is the user's folder, not the app's: the app writes its one file
// and prunes, renames and deletes nothing.
func TestBackupLeavesEverythingElseInTheFolderAlone(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('x','manual','t')`); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	bystanders := []string{"holiday.jpg", "notes.txt", "modelsdb-backup-19990101-000000.db"}
	for _, name := range bystanders {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("mine"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	for run := 1; run <= 3; run++ {
		if _, err := BackupTo(dir, OverwritingBackup); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}

	for _, name := range bystanders {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("the backup removed %q from the destination folder: %v", name, err)
			continue
		}
		if string(b) != "mine" {
			t.Errorf("the backup rewrote %q", name)
		}
	}
}

// TestBackupRefusesAnUnusableDestination covers the two destinations that cannot
// hold a backup, each with its own message.
func TestBackupRefusesAnUnusableDestination(t *testing.T) {
	setupTestDB(t)

	if _, err := BackupTo("   ", TimestampedBackup); err == nil {
		t.Error("an empty destination should be refused")
	}

	missing := filepath.Join(t.TempDir(), "no-such-folder")
	if _, err := BackupTo(missing, TimestampedBackup); err == nil {
		t.Error("a destination folder that does not exist should be refused")
	}

	notADir := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(notADir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupTo(notADir, TimestampedBackup); err == nil {
		t.Error("a destination that is a file should be refused")
	}
}
