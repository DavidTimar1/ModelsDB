// The user-facing database backup: one complete copy of the
// SQLite file, written into a folder the user names.
//
// This is a SECOND, separate mechanism from the pre-migration snapshots in
// migrate.go. The two differ in purpose and in policy, and neither is a
// substitute for the other:
//
//   - The pre-migration snapshot (backupDB) protects one schema migration. It
//     lands in <data>/.backups, the app chooses the moment and the name, and the
//     folder is pruned to paths.MaxBackupsToKeep entries.
//   - This backup protects the USER'S data. It lands wherever the user asked, it
//     is never pruned, and nothing but the single file it was asked to write is
//     created, listed, or removed in that folder.

package dbcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BackupNaming decides what a backup file is called, and therefore whether a run
// adds a file or replaces one.
type BackupNaming int

const (
	// TimestampedBackup names the file after the moment it was taken, so every
	// run leaves one more backup in the folder.
	TimestampedBackup BackupNaming = iota

	// OverwritingBackup gives every backup the same name, so each run replaces
	// the previous file and the folder holds exactly one.
	OverwritingBackup
)

// backupFilePrefix starts every backup filename, so a backup is recognisable in
// a folder holding the user's own files.
const backupFilePrefix = "modelsdb-backup"

// backupWorkingPattern names the file a backup is written to before it is given
// its real name. The prefix marks it as this app's, so a file left behind by a
// machine that lost power mid-backup is identifiable rather than mysterious, and
// the .tmp ending keeps it out of the set of .db files a reader would take for
// finished backups.
const backupWorkingPattern = backupFilePrefix + "-*.tmp"

// BackupTo writes one complete copy of the database into dir and returns the
// path it wrote. naming decides whether that path is a new timestamped file or
// the same file every time. It creates exactly one file and touches nothing else
// in dir: it does not list, rename, prune, or delete anything the user put there.
//
// SQLITE TAKES THE SNAPSHOT, NOT THE FILESYSTEM. The copy is produced by SQLite's
// own VACUUM INTO, which reads the database through SQLite and writes out a fresh
// file holding a consistent picture of it. Two things follow, and both matter.
// The live database is only READ: modelsdb.db and its modelsdb.db-wal sidecar
// come out of a backup byte for byte and timestamp for timestamp as they went in.
// And the newest writes, which in WAL mode sit in that sidecar rather than in
// modelsdb.db, are in the copy, so the result is one self-contained file instead
// of a main file that opens cleanly while silently missing recent work.
//
// THE FILE IS NAMED ONLY ONCE IT IS FINISHED. The snapshot is written under a
// working name in dir and given the requested name afterwards. A failure
// therefore leaves nothing at the requested name, so a previous backup sitting
// there survives untouched; and because that last step is a single one, the name
// never points at a half-written file.
//
// A TIMESTAMPED BACKUP NEVER REPLACES A FILE. The stamp has one-second
// resolution, so a second backup in the same second takes the next free name
// (-2, -3, ...) rather than refusing: a scheduled run still succeeds and both
// restore points survive. Only OverwritingBackup replaces, and only its own one
// file.
//
// It returns errors and never ends the process. A backup that cannot be taken is
// something to report to whoever asked for it; the server keeps serving.
func BackupTo(dir string, naming BackupNaming) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("a backup needs a destination folder")
	}
	if DB == nil {
		return "", fmt.Errorf("the database is not open, so there is nothing to back up")
	}

	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("destination folder %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("destination %s is a file, not a folder", dir)
	}

	dst := filepath.Join(dir, backupFileName(naming))
	return vacuumSnapshot(dir, dst, naming == OverwritingBackup)
}

// backupFileName returns the filename a backup takes under the given naming.
func backupFileName(naming BackupNaming) string {
	if naming == OverwritingBackup {
		return backupFilePrefix + ".db"
	}
	return fmt.Sprintf("%s-%s.db", backupFilePrefix, fsStamp())
}
