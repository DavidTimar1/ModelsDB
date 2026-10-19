package dbcore

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestSnapshotNeverReplacesAFileAlreadyAtTheName is the same-second clash: a
// restore point already sits at the name the snapshot wants, and it must come
// out byte for byte as it went in, with the new copy beside it.
func TestSnapshotNeverReplacesAFileAlreadyAtTheName(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 3)
	dir := t.TempDir()
	dst := filepath.Join(dir, "modelsdb-backup-20260101-000000.db")
	if err := os.WriteFile(dst, []byte("the earlier restore point"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := vacuumSnapshot(dir, dst, false)
	if err != nil {
		t.Fatalf("snapshot beside an existing file: %v", err)
	}
	if got == dst {
		t.Fatalf("the snapshot reported the name that was already taken, %s", got)
	}
	if want := filepath.Join(dir, "modelsdb-backup-20260101-000000-2.db"); got != want {
		t.Errorf("snapshot took %s, want the next free name %s", got, want)
	}
	kept, err := os.ReadFile(dst)
	if err != nil || string(kept) != "the earlier restore point" {
		t.Errorf("the existing file was changed: %q, %v", kept, err)
	}
	if info, err := os.Stat(got); err != nil || info.Size() == 0 {
		t.Errorf("the new copy is missing or empty: %v", err)
	}
	// A third clash walks on to -3, and no working file is left behind.
	third, err := vacuumSnapshot(dir, dst, false)
	if err != nil || !strings.HasSuffix(third, "-3.db") {
		t.Errorf("third snapshot = %q, %v, want a -3 name", third, err)
	}
	for _, name := range dirEntries(t, dir) {
		if strings.HasSuffix(name, ".tmp") {
			t.Errorf("a working file was left behind: %s", name)
		}
	}
}

// TestSnapshotReplaceOverwritesOnlyWhenAsked pins the overwriting backup's design.
func TestSnapshotReplaceOverwritesOnlyWhenAsked(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 3)
	dir := t.TempDir()
	dst := filepath.Join(dir, "modelsdb-backup.db")
	if err := os.WriteFile(dst, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := vacuumSnapshot(dir, dst, true)
	if err != nil || got != dst {
		t.Fatalf("replacing snapshot = %q, %v, want %s", got, err, dst)
	}
	if b, _ := os.ReadFile(dst); string(b) == "old" {
		t.Error("the replacing snapshot left the old content in place")
	}
	if names := dirEntries(t, dir); len(names) != 1 {
		t.Errorf("a replacing snapshot should leave 1 file, found %v", names)
	}
}

// TestTimestampedBackupsInOneSecondAreAllKept takes several backups back to back,
// which land in the same second, and expects one file each.
func TestTimestampedBackupsInOneSecondAreAllKept(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 3)
	dir := t.TempDir()
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		p, err := BackupTo(dir, TimestampedBackup)
		if err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		if seen[p] {
			t.Fatalf("backup %d reported %s, which an earlier backup already wrote", i, p)
		}
		seen[p] = true
	}
	if names := dirEntries(t, dir); len(names) != 5 {
		t.Errorf("5 backups left %d files: %v", len(names), names)
	}
}

// TestSimultaneousClaimsOfOneNameEachGetTheirOwn races the claim itself: the
// hard link is the atomic step, so every caller ends with a distinct file.
func TestSimultaneousClaimsOfOneNameEachGetTheirOwn(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "modelsdb-backup-x.db")
	const callers = 8
	got := make([]string, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		work := filepath.Join(dir, "modelsdb-backup-w"+string(rune('a'+i))+".tmp")
		if err := os.WriteFile(work, []byte{byte('a' + i)}, 0600); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(i int, work string) {
			defer wg.Done()
			p, err := nameWithoutReplacing(work, dst)
			if err != nil {
				t.Errorf("caller %d: %v", i, err)
			}
			got[i] = p
		}(i, work)
	}
	wg.Wait()
	seen := map[string]byte{}
	for i, p := range got {
		b, err := os.ReadFile(p)
		if err != nil || len(b) != 1 || b[0] != byte('a'+i) {
			t.Errorf("caller %d: %s holds %q, %v, want its own byte", i, p, b, err)
		}
		if _, dup := seen[p]; dup {
			t.Errorf("two callers ended on %s", p)
		}
		seen[p] = 1
	}
}

// TestPreMigrationSnapshotKeepsAnExistingFileOfTheSameName covers backupDB: a
// snapshot already sitting at this second's name survives a second one.
func TestPreMigrationSnapshotKeepsAnExistingFileOfTheSameName(t *testing.T) {
	setupTestDB(t)
	fillWAL(t, 3)
	first, err := backupDB(40)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backupDB(40)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("two pre-migration snapshots share the path %s", first)
	}
	for _, p := range []string{first, second} {
		if info, err := os.Stat(p); err != nil || info.Size() == 0 {
			t.Errorf("snapshot %s missing or empty: %v", p, err)
		}
	}
}
