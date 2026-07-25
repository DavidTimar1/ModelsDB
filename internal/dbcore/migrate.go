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
	"strconv"
	"strings"
	"time"
)

// schemaVersion is the schema this build expects. Bump it by ONE and append a
// matching migration to the registry for every schema change.
const schemaVersion = 5

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
	// v4: add the generic custom-column tables (custom_columns,
	// custom_column_options, custom_values - idempotent, fresh DBs already have
	// them from schemaSQL) and carry every model's existing speed/rating/
	// ocr_quality value into three custom-column definitions ("Speed", "Rating",
	// "OCR"), so no user's live personal data is lost when the app moves from
	// fixed personal columns to user-defined ones. The old models.speed/rating/
	// ocr_quality columns are left in place for this one step (v5, next, drops
	// them) so the migration itself has something to read the legacy values from.
	{version: 4, apply: addCustomColumnsAndMigratePersonalFields},
	// v5: drop the legacy models.speed/rating/ocr_quality columns. Every
	// consumer (SaveCurated, UpdateCurated, ImportFullRecord, the /api/save
	// handler, the UI) now reads and writes Speed/Rating/OCR-style personal
	// values exclusively through the generic custom-column system, so these
	// columns are dead weight - this is the contract step of the v4 migration's
	// expand/migrate/contract sequence.
	{version: 5, apply: dropLegacyPersonalColumns},
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

// addCustomColumnsAndMigratePersonalFields creates the generic custom-column
// tables (idempotent - fresh DBs already have them from schemaSQL) and, on a DB
// upgrading from an older version, carries every model's existing speed/rating/
// ocr_quality value into three new custom-column definitions: "Speed"
// (dropdown_text: fast/avg/slow), "Rating" (dropdown_number: 1-4), and "OCR"
// (dropdown_text: good/bad) - the option sets and unset sentinels (empty string
// for speed/ocr_quality, 0 for rating) match what the UI's inline selects
// already accepted, so every value maps onto the new system with no loss. A
// model with no value for one of these (still at its unset default) simply
// gets no row in custom_values, matching the new system's sparse storage. The
// legacy speed/rating/ocr_quality columns themselves are dropped by the very
// next migration (v5, dropLegacyPersonalColumns) once this one has read them.
func addCustomColumnsAndMigratePersonalFields(tx *sql.Tx) error {
	// The three allowed `type` values (and the "dropdown_text"/"dropdown_number"
	// literals passed to ensureCustomColumn below) mirror store.ColumnTypeText/
	// ColumnTypeDropdownText/ColumnTypeDropdownNumber; change both together.
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS custom_columns (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT    NOT NULL UNIQUE,
			type       TEXT    NOT NULL CHECK (type IN ('text', 'dropdown_text', 'dropdown_number')),
			created_at TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS custom_column_options (
			column_id INTEGER NOT NULL REFERENCES custom_columns(id) ON DELETE CASCADE,
			position  INTEGER NOT NULL,
			value     TEXT    NOT NULL,
			PRIMARY KEY (column_id, position)
		)`,
		`CREATE TABLE IF NOT EXISTS custom_values (
			column_id  INTEGER NOT NULL REFERENCES custom_columns(id) ON DELETE CASCADE,
			model_name TEXT    NOT NULL REFERENCES models(name) ON DELETE CASCADE,
			value      TEXT    NOT NULL DEFAULT '',
			PRIMARY KEY (column_id, model_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_values_model ON custom_values(model_name)`,
	} {
		if _, err := tx.Exec(ddl); err != nil {
			return fmt.Errorf("create custom-column schema: %w", err)
		}
	}

	speedID, err := ensureCustomColumn(tx, "Speed", "dropdown_text", []string{"fast", "avg", "slow"})
	if err != nil {
		return fmt.Errorf("create Speed custom column: %w", err)
	}
	ratingID, err := ensureCustomColumn(tx, "Rating", "dropdown_number", []string{"1", "2", "3", "4"})
	if err != nil {
		return fmt.Errorf("create Rating custom column: %w", err)
	}
	ocrID, err := ensureCustomColumn(tx, "OCR", "dropdown_text", []string{"good", "bad"})
	if err != nil {
		return fmt.Errorf("create OCR custom column: %w", err)
	}

	rows, err := tx.Query("SELECT name, speed, rating, ocr_quality FROM models")
	if err != nil {
		return fmt.Errorf("read legacy personal columns: %w", err)
	}
	type legacyRow struct {
		name, speed, ocr string
		rating           int64
	}
	var legacy []legacyRow
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.name, &r.speed, &r.rating, &r.ocr); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy personal columns: %w", err)
		}
		legacy = append(legacy, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, r := range legacy {
		if v := strings.TrimSpace(r.speed); v != "" {
			if err := setCustomValueInTx(tx, speedID, r.name, v); err != nil {
				return fmt.Errorf("carry speed for %q: %w", r.name, err)
			}
		}
		if r.rating >= 1 && r.rating <= 4 {
			if err := setCustomValueInTx(tx, ratingID, r.name, strconv.FormatInt(r.rating, 10)); err != nil {
				return fmt.Errorf("carry rating for %q: %w", r.name, err)
			}
		}
		if v := strings.TrimSpace(r.ocr); v != "" {
			if err := setCustomValueInTx(tx, ocrID, r.name, v); err != nil {
				return fmt.Errorf("carry ocr_quality for %q: %w", r.name, err)
			}
		}
	}
	return nil
}

// ensureCustomColumn returns the id of the named custom column, creating it
// (with its ordered dropdown options) if it does not already exist. Idempotent,
// like the rest of this migration - safe if ever re-run against a DB that
// already has the column.
func ensureCustomColumn(tx *sql.Tx, name, colType string, options []string) (int64, error) {
	var id int64
	err := tx.QueryRow("SELECT id FROM custom_columns WHERE name=?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := tx.Exec("INSERT INTO custom_columns (name, type, created_at) VALUES (?,?,?)", name, colType, NowStamp())
	if err != nil {
		return 0, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, opt := range options {
		if _, err := tx.Exec("INSERT INTO custom_column_options (column_id, position, value) VALUES (?,?,?)", id, i, opt); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// setCustomValueInTx upserts one model's value for one custom column within a
// migration transaction (the store-package equivalent, store.SetCustomValue,
// runs on the plain *sql.DB and is not usable inside a *sql.Tx).
func setCustomValueInTx(tx *sql.Tx, columnID int64, modelName, value string) error {
	_, err := tx.Exec(`INSERT INTO custom_values (column_id, model_name, value) VALUES (?,?,?)
		ON CONFLICT(column_id, model_name) DO UPDATE SET value=excluded.value`, columnID, modelName, value)
	return err
}

// dropLegacyPersonalColumns removes the legacy models.speed/rating/ocr_quality
// columns now that every value they held has already been carried into the
// custom-column system (by the v4 migration, which runs immediately before this
// one) and every code path reads/writes personal Speed/Rating/OCR-style data
// through that system instead. Idempotent - checks each column exists first
// (via columnExists), so re-running this against a DB that already lacks them
// (or a fresh DB, whose schemaSQL-created columns v4 has already emptied into
// custom_values by the time this runs) is a no-op.
func dropLegacyPersonalColumns(tx *sql.Tx) error {
	for _, col := range []string{"speed", "rating", "ocr_quality"} {
		has, err := columnExists(tx, col)
		if err != nil {
			return err
		}
		if !has {
			continue
		}
		if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE models DROP COLUMN %s", col)); err != nil {
			return fmt.Errorf("drop column %s: %w", col, err)
		}
	}
	return nil
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
