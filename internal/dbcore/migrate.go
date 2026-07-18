// Package dbcore - DB schema versioning with safe, backed-up migrations.
//
// The schema version is tracked in SQLite's built-in PRAGMA user_version. On
// open, migrate() applies every registered migration whose target version is
// above the DB's current version, in order. Because a shipped binary evolves a
// stranger's database, the path is deliberately conservative:
//
//   - Before touching a non-empty DB, a full pre-migration BACKUP is copied into
//     the data dir's .backups folder (WAL checkpointed first so the single .db
//     file is a complete snapshot). Retention is capped at MaxBackupsToKeep.
//   - Each migration runs in a transaction and bumps user_version atomically.
//   - After migrating, PRAGMA integrity_check must pass.
//   - On ANY failure, the pre-migration backup is restored over the live DB and
//     the app exits with a clear, visible error (the backup is kept). We never
//     leave a half-migrated database in place.
//
// A DB whose version is NEWER than this build is left untouched (no downgrade);
// the min_app_version gate and update notifier handle telling the user to update.
package dbcore

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// schemaVersion is the schema this build expects. Bump it by ONE and append a
// matching migration to the registry for every schema change.
const schemaVersion = 3

type migration struct {
	version int // target user_version after this migration
	apply   func(tx *sql.Tx) error
}

// migrations must be ordered by ascending version and contiguous from 1.
var migrations = []migration{
	// v1: baseline. Fresh DBs already have the full schema from schemaSQL; this
	// brings PRE-versioning databases (created before user_version existed) up to
	// the current column set. Idempotent.
	{version: 1, apply: addBaselineColumns},
	// v2: add the `unlisted` flag (exclude a model from the public curated.json
	// export while keeping it in the DB). Idempotent - fresh DBs already have the
	// column from schemaSQL.
	{version: 2, apply: addUnlistedColumn},
	// v3: add the curated `disk_size_gb` column (native-precision on-disk model
	// size in GB). Idempotent - fresh DBs already have the column from schemaSQL.
	{version: 3, apply: addDiskSizeColumn},
}

func currentSchemaVersion() (int, error) {
	var v int
	err := DB.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}

// migrate applies pending migrations with a pre-migration backup and integrity
// check, auto-restoring on failure. See the package doc for the guarantees.
func migrate() error {
	cur, err := currentSchemaVersion()
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if cur == schemaVersion {
		return nil
	}
	if cur > schemaVersion {
		log.Printf("Warning: database schema v%d is newer than this build (v%d); leaving it untouched. Update ModelsDB.", cur, schemaVersion)
		return nil
	}

	// Back up first, but only when there is data worth protecting.
	var backupPath string
	if rows, _ := CountModels(); rows > 0 {
		backupPath, err = backupDB(cur)
		if err != nil {
			return fmt.Errorf("pre-migration backup failed (aborting, DB untouched): %w", err)
		}
		log.Printf("Pre-migration backup written: %s", backupPath)
	}

	for _, m := range migrations {
		if m.version <= cur {
			continue
		}
		if err := applyMigration(m); err != nil {
			failMigration(m.version, err, backupPath)
		}
		log.Printf("Migrated schema v%d -> v%d", cur, m.version)
		cur = m.version
	}

	if err := integrityCheck(); err != nil {
		failMigration(schemaVersion, fmt.Errorf("integrity check: %w", err), backupPath)
	}
	return nil
}

// applyMigration runs one migration and bumps user_version atomically.
func applyMigration(m migration) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	if err := m.apply(tx); err != nil {
		tx.Rollback()
		return err
	}
	// PRAGMA user_version takes no placeholders; the value is our own int constant.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", m.version)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// failMigration restores the pre-migration backup (if any) over the live DB and
// exits with a visible error. It never returns.
func failMigration(version int, cause error, backupPath string) {
	if backupPath == "" {
		shared.Fatalf("schema migration to v%d failed: %v. The database was a fresh/empty DB, so no backup was taken; remove %s and restart to rebuild.", version, cause, paths.DbFile)
		return
	}
	if rerr := restoreFromBackup(backupPath); rerr != nil {
		shared.Fatalf("schema migration to v%d failed: %v. AND restoring the backup failed: %v. Your pre-migration backup is intact at %s - restore it manually over %s.", version, cause, rerr, backupPath, paths.DbFile)
		return
	}
	shared.Fatalf("schema migration to v%d failed: %v. No changes were kept - the pre-migration backup was restored over the database. Backup retained at %s. Please update ModelsDB or report this.", version, cause, backupPath)
}

func integrityCheck() error {
	var res string
	if err := DB.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("%s", res)
	}
	return nil
}

// addBaselineColumns adds any columns missing from a pre-versioning database
// (SQLite has no "ADD COLUMN IF NOT EXISTS", so check PRAGMA table_info first).
func addBaselineColumns(tx *sql.Tx) error {
	rows, err := tx.Query("PRAGMA table_info(models)")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	rows.Close()
	for _, c := range []struct{ name, ddl string }{
		{"price_display", "ALTER TABLE models ADD COLUMN price_display TEXT DEFAULT ''"},
		{"pricing_url", "ALTER TABLE models ADD COLUMN pricing_url TEXT DEFAULT ''"},
		{"zdr", "ALTER TABLE models ADD COLUMN zdr INTEGER DEFAULT 1"},
		{"measurement", "ALTER TABLE models ADD COLUMN measurement TEXT DEFAULT ''"},
		{"pricing_note", "ALTER TABLE models ADD COLUMN pricing_note TEXT DEFAULT ''"},
	} {
		if !have[c.name] {
			if _, err := tx.Exec(c.ddl); err != nil {
				return fmt.Errorf("add column %s: %w", c.name, err)
			}
		}
	}
	return nil
}

// columnExists reports whether the models table already has a column of the
// given name (SQLite has no "ADD COLUMN IF NOT EXISTS").
func columnExists(tx *sql.Tx, col string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(models)")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}

// addUnlistedColumn adds the `unlisted` export-exclusion flag if it is not
// already present (fresh DBs get it from schemaSQL).
func addUnlistedColumn(tx *sql.Tx) error {
	has, err := columnExists(tx, "unlisted")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = tx.Exec("ALTER TABLE models ADD COLUMN unlisted INTEGER DEFAULT 0")
	return err
}

// addDiskSizeColumn adds the curated `disk_size_gb` column (native-precision
// on-disk model size in GB) if it is not already present (fresh DBs get it from
// schemaSQL).
func addDiskSizeColumn(tx *sql.Tx) error {
	has, err := columnExists(tx, "disk_size_gb")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = tx.Exec("ALTER TABLE models ADD COLUMN disk_size_gb REAL")
	return err
}

// ---- backup / restore -------------------------------------------------------

// fsStamp is a filesystem-safe UTC timestamp (no colons) for backup filenames.
func fsStamp() string { return time.Now().UTC().Format("20060102-150405") }

// backupDB checkpoints the WAL and copies the live DB into BackupsDir, returning
// the backup path. fromVer is recorded in the filename.
func backupDB(fromVer int) (string, error) {
	if _, err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("Warning: WAL checkpoint before backup failed: %v", err)
	}
	if err := os.MkdirAll(paths.BackupsDir, 0755); err != nil {
		return "", err
	}
	dst := filepath.Join(paths.BackupsDir, fmt.Sprintf("modelsdb-v%d-%s.db", fromVer, fsStamp()))
	if err := copyFile(paths.DbFile, dst); err != nil {
		return "", err
	}
	pruneBackups()
	return dst, nil
}

// restoreFromBackup closes the DB, replaces the live file with the backup (and
// drops stale WAL/SHM sidecars), then reopens the connection WITHOUT migrating.
func restoreFromBackup(backupPath string) error {
	if DB != nil {
		DB.Close()
	}
	os.Remove(paths.DbFile + "-wal")
	os.Remove(paths.DbFile + "-shm")
	if err := copyFile(backupPath, paths.DbFile); err != nil {
		return err
	}
	return openConn(paths.DbFile)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// dbBackups returns the backup files in BackupsDir, oldest first.
func dbBackups() []os.DirEntry {
	entries, err := os.ReadDir(paths.BackupsDir)
	if err != nil {
		return nil
	}
	var files []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, e)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		fi, _ := files[i].Info()
		fj, _ := files[j].Info()
		return fi.ModTime().Before(fj.ModTime())
	})
	return files
}

func pruneBackups() {
	files := dbBackups()
	if len(files) <= paths.MaxBackupsToKeep {
		return
	}
	for _, e := range files[:len(files)-paths.MaxBackupsToKeep] {
		os.Remove(filepath.Join(paths.BackupsDir, e.Name()))
	}
}

// ---- CLI verbs --------------------------------------------------------------

// ListBackups prints the available pre-migration backups (newest last).
func ListBackups() {
	files := dbBackups()
	if len(files) == 0 {
		fmt.Printf("No backups in %s\n", paths.BackupsDir)
		return
	}
	fmt.Printf("Backups in %s:\n", paths.BackupsDir)
	for _, e := range files {
		info, _ := e.Info()
		var size int64
		var when string
		if info != nil {
			size = info.Size()
			when = info.ModTime().Local().Format("2006-01-02 15:04:05")
		}
		fmt.Printf("  %-36s %10d bytes  %s\n", e.Name(), size, when)
	}
}

// RestoreBackup copies a chosen backup over the live DB after first snapshotting
// the current DB (so a restore is itself reversible). The arg may be a full path
// or a filename within BackupsDir.
func RestoreBackup(arg string) error {
	src := arg
	if !filepath.IsAbs(src) && !paths.FileExists(src) {
		src = filepath.Join(paths.BackupsDir, arg)
	}
	if !paths.FileExists(src) {
		return fmt.Errorf("backup not found: %s", arg)
	}
	if paths.FileExists(paths.DbFile) {
		if _, err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			log.Printf("Warning: WAL checkpoint before restore failed: %v", err)
		}
		os.MkdirAll(paths.BackupsDir, 0755)
		safety := filepath.Join(paths.BackupsDir, fmt.Sprintf("modelsdb-prerestore-%s.db", fsStamp()))
		if err := copyFile(paths.DbFile, safety); err != nil {
			return fmt.Errorf("could not snapshot current DB before restore: %w", err)
		}
		log.Printf("Saved current DB to %s before restoring", safety)
	}
	if DB != nil {
		DB.Close()
	}
	os.Remove(paths.DbFile + "-wal")
	os.Remove(paths.DbFile + "-shm")
	if err := copyFile(src, paths.DbFile); err != nil {
		return err
	}
	log.Printf("Restored %s over %s. Start ModelsDB normally; any pending migrations will run with a fresh backup.", src, paths.DbFile)
	return nil
}
