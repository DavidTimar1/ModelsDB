// DB schema versioning with safe, backed-up migrations.
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
//   - The number of stored CELLS is counted before and after the whole run; a
//     DECREASE aborts, because cells rows cascade away when a models row is
//     deleted (cells.model_name REFERENCES models(name) ON DELETE CASCADE, with
//     PRAGMA foreign_keys=ON), and every fact this app holds - the reader's notes
//     and their own columns included - is one of those rows and exists nowhere
//     but this file.
//   - After migrating, PRAGMA integrity_check must pass.
//   - On ANY failure, the pre-migration backup is restored over the live DB and
//     the app exits with a clear, visible error (the backup is kept). We never
//     leave a half-migrated database in place.
//
// A DB whose version is NEWER than this build is left untouched (no downgrade);
// the update notifier is what tells the user to update.
//
// These snapshots protect a migration, not the user's data. The user-facing
// backup is a separate mechanism with its own folder and no retention cap - see
// BackupTo in backup.go.

package dbcore

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
const schemaVersion = 52

type migration struct {
	version int // target user_version after this migration
	apply   func(tx *sql.Tx) error
	// disableForeignKeys runs this migration with PRAGMA foreign_keys=OFF for
	// its own transaction, and checks PRAGMA foreign_key_check once it has
	// committed. A rebuild that recreates a table (CREATE new, copy, DROP old,
	// rename) cannot run under an enforced foreign key that references the
	// table being dropped, and setting the pragma INSIDE the transaction is a
	// no-op in SQLite - it must be set on the connection before Begin and
	// restored after Commit or Rollback. integrity_check does not catch an
	// orphaned row left behind while the constraint was off, which is why
	// foreign_key_check runs afterwards regardless of whether the migration
	// looks like it succeeded.
	disableForeignKeys bool
	// cellsMayFall permits this one migration to reduce the stored cell count.
	// Only a migration that deletes on purpose sets it, and such a
	// migration carries an exact refusal of its own in place of the guard, so
	// its legitimate drop cannot mask a different migration's accidental one:
	// the guard runs per migration, not once around the whole chain.
	cellsMayFall bool
}

// migrations must be ordered by ascending version and contiguous from the
// floor. No database below schema 33 is ever opened (see the floor check in
// OpenDB), so there is no migration up to 33; docs/migration.md says how a
// database that old is carried forward. Each entry says what the migration does,
// and the file it lives in says how it stays safe.
var migrations = []migration{
	// v34: every model carries the date it entered this catalog, in a sealed
	// column. No source states it and nothing can work it out afterwards, so every
	// row already here is recorded as having arrived on the day the column
	// shipped, and every row created later is stamped when it is created. Purely
	// additive. See addedcolumn.go.
	{version: 34, apply: theAddedColumnRecordsWhenAModelArrived},
	// v35: the reader's view - filters, column order, what is hidden, page size -
	// and the app's own preferences live in app_settings, so a restored backup
	// brings back a view that matches its models. It runs on a fresh file too:
	// the base schema creates the table and an earlier migration drops it, so the
	// CREATE here is not optional.
	{version: 35, apply: theReadersViewLivesInTheDatabase},
	// v36: drops the three empty tables the base schema used to recreate on every
	// open (custom_columns, custom_column_options, custom_values). It refuses
	// itself rather than lose anything: a row in any of them was written by a
	// route nobody knows about, so the migration fails and the pre-migration
	// backup is restored instead of the table being dropped.
	{version: 36, apply: theCustomColumnTablesAreGone},
	// v37: the Type column reads what OpenRouter and a repository state, not only
	// the reader's own answer. See typereads.go.
	{version: 37, apply: theTypeColumnReadsEverySourceThatStatesIt},
	// v38: the MoE column reads what OpenRouter states too. See moereads.go.
	{version: 38, apply: theMoeColumnReadsEverySourceThatStatesIt},
	// v39: adds the sources and column_sources tables and cells.source and
	// columns.resolve, the last two backfilled from what each cell's column
	// namespace already says. Purely additive. See pivotexpand.go.
	{version: 39, apply: theSchemaExpandsForThePivot},
	// v40: widens the cells primary key to (column_id, model_name, source) so one
	// column holds a row per source, then collapses the columns that state one
	// attribute onto a single column, moving their cells with them. See
	// pivotcollapse.go.
	{version: 40, apply: theColumnsCollapseOntoOnePerAttribute, disableForeignKeys: true},
	// v41: rebuilds columns without a namespace, adds the two write-guard triggers
	// and the answers view, and drops derived_columns and derived_inputs, which
	// column_sources replaces. See pivotcontract.go.
	{version: 41, apply: theSchemaContractsForThePivot, disableForeignKeys: true},
	// v42: deletes every cell stamped source='rule', which the derived-column
	// subsystem left behind. 'rule' carries no column_sources row, so the answers
	// view can only pick one as the winner when it is the sole cell on record for
	// its column and model; the migration is refused before anything is deleted
	// otherwise. The only migration besides v47 and v48 allowed to reduce the
	// stored cell count. See ruledeath.go.
	{version: 42, apply: theDeadRuleCellsAreDeleted, cellsMayFall: true},
	// v43: rewrites the reader's saved view settings (col_hidden, col_known,
	// col_order, order in app_settings), which still name columns by their old
	// dotted key. See staleviewsettings.go.
	{version: 43, apply: theStaleViewSettingsAreRewritten},
	// v44: drops ignored_models and its index; nothing holds a model's key back
	// from coming again. See ignoredropped.go.
	{version: 44, apply: theIgnoredModelsTableIsDropped},
	// v45: adds feed_fields, the record of every leaf path the feed has carried.
	// See feedfields.go.
	{version: 45, apply: theFeedFieldsTableIsAdded},
	// v46: the research source replaces hand on every column an automatic source
	// can state, and the sources table is rebuilt to admit it. Every hand cell on
	// such a column becomes a research cell, so the count stays level and the
	// chain's guard proves it. See research.go.
	{version: 46, apply: theResearchSourceReplacesHandOnFetchedColumns, disableForeignKeys: true},
	// v47: removes the collection source: its cells on surviving columns become
	// research, the three columns only it filled are deleted with their cells, and
	// its rows in column_sources, sources and source_records go. Guarded by its
	// own exact count of the cells it may drop. See collectiondeath.go.
	{version: 47, apply: theCollectionSourceIsGone, cellsMayFall: true},
	// v48: deletes the per-key columns made off pricing.overrides[].* and
	// benchmarks.design_arena[].*. A list of price windows or leaderboard rows is
	// one list column, one member per element, so a cell can no longer pair one
	// window's price with a different window's hours. See overridewindows.go.
	{version: 48, apply: theOverrideWindowColumnsAreRetired, cellsMayFall: true},
	// v49: the GGUF total column, which holds a parameter count, loses its "bytes"
	// label and unit while the label is still the shipped one. No cell is
	// touched. See gguftotallabel.go.
	{version: 49, apply: theGGUFTotalColumnIsRelabelled},
	// v50: drops ignored_models, derived_columns and derived_inputs again from a
	// file in which an earlier build re-created them empty. See retiredtables.go.
	{version: 50, apply: theRetiredTablesAreDropped},
	// v51: deletes the Pricing note column after carrying every pricing note
	// with text in it into that model's Note, guarded by its own exact count.
	// See pricingnote.go.
	{version: 51, apply: thePricingNoteColumnIsRetired, cellsMayFall: true},
	// v52: writes what the HuggingFace architecture and weight-format columns
	// measured into one Architecture line per model, then deletes those columns
	// and eleven that lose nothing, guarded by its own exact counts. See
	// architecturefold.go.
	{version: 52, apply: theArchitectureColumnHoldsWhatTheHuggingFaceColumnsHeld, cellsMayFall: true},
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
	rows, err := CountModels()
	if err != nil {
		return fmt.Errorf("count the models before migrating (aborting, DB untouched): %w", err)
	}
	if rows > 0 {
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
		// The cell-count guard runs around EACH migration, not once around the
		// whole chain: a migration further down that legitimately drops rows
		// (cellsMayFall) must never mask an accidental drop by a different
		// migration earlier in the same run.
		var before int
		if !m.cellsMayFall {
			before, err = countCells()
			if err != nil {
				failMigration(m.version, fmt.Errorf("count the stored cells before migrating: %w", err), backupPath)
			}
		}
		if err := applyMigration(m); err != nil {
			failMigration(m.version, err, backupPath)
		}
		if !m.cellsMayFall {
			after, err := countCells()
			if err != nil {
				failMigration(m.version, fmt.Errorf("count the stored cells after migrating: %w", err), backupPath)
			}
			if after < before {
				failMigration(m.version, fmt.Errorf("stored values dropped from %d to %d cells", before, after), backupPath)
			}
		}
		log.Printf("Migrated schema v%d -> v%d", cur, m.version)
		cur = m.version
	}

	if err := integrityCheck(); err != nil {
		failMigration(schemaVersion, fmt.Errorf("integrity check: %w", err), backupPath)
	}
	return nil
}

// countCells returns the number of stored values. It is the measure migrate()
// guards: every fact this app holds is one of these rows, they hold personal
// data that exists nowhere but this file, and they cascade away silently if a
// migration ever deletes and re-inserts a models row (see the package doc).
//
// A DATABASE WITH NO CELLS TABLE COUNTS ZERO rather than failing. One carried
// forward from before the cell store existed reaches the run without the table,
// and it is the run that creates it - so "none yet" is the honest answer, and any number the run
// ends with is above it.
func countCells() (int, error) {
	var held int
	if err := DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='cells'").Scan(&held); err != nil {
		return 0, err
	}
	if held == 0 {
		return 0, nil
	}
	var n int
	err := DB.QueryRow("SELECT COUNT(*) FROM cells").Scan(&n)
	return n, err
}

// applyMigration runs one migration and bumps user_version atomically. When
// the migration asks for it (disableForeignKeys), foreign key enforcement is
// off for the whole of this migration's own connection-level state - OUTSIDE
// the transaction, because setting the pragma inside one is a no-op - and
// PRAGMA foreign_key_check runs once the transaction has committed, because
// PRAGMA integrity_check does not see a row left orphaned while the
// constraint was off.
func applyMigration(m migration) error {
	if m.disableForeignKeys {
		if _, err := DB.Exec("PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("disable foreign keys for migration v%d: %w", m.version, err)
		}
		defer func() {
			if _, err := DB.Exec("PRAGMA foreign_keys=ON"); err != nil {
				log.Printf("Warning: could not re-enable foreign keys after migration v%d: %v", m.version, err)
			}
		}()
	}

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
	if err := tx.Commit(); err != nil {
		return err
	}

	if m.disableForeignKeys {
		if err := foreignKeyCheck(); err != nil {
			return fmt.Errorf("migration v%d left an orphaned row: %w", m.version, err)
		}
	}
	return nil
}

// foreignKeyCheck runs PRAGMA foreign_key_check and reports every violation it
// finds. Unlike PRAGMA integrity_check, this is the one check that catches a
// row orphaned while foreign_keys was OFF.
func foreignKeyCheck() error {
	rows, err := DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	var violations []string
	for rows.Next() {
		var table string
		var rowid sql.NullInt64
		var parent string
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		violations = append(violations, fmt.Sprintf("%s row %v references missing %s", table, rowid.Int64, parent))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(violations) > 0 {
		return fmt.Errorf("%d row(s): %s", len(violations), strings.Join(violations, "; "))
	}
	return nil
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

// tableExists reports whether this database holds a table of that name.
func tableExists(tx *sql.Tx, name string) (bool, error) {
	var held int
	err := tx.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&held)
	return held > 0, err
}

// ShippedPriceColorRules is the rule the two price columns ship with, as the
// JSON a column's color_rules field stores. It mirrors store.priceColors, which
// is what a fresh install seeds - store sits above dbcore in the import DAG
// (paths -> shared -> dbcore -> store), so it cannot be read from here; change
// both together, and store's TestThePriceColumnsShipTheColoursTheMigrationWrites
// holds them in step by reading this value, which is why it is exported.
const ShippedPriceColorRules = `{"thresholds":[{"below":1,"color":"#22c55e"},{"below":2,"color":"#eab308"}],"otherwise":"#ef4444"}`

// settingsResetNoticeKey is the row the page reads to know that the reader's view
// did not come across from the settings file. store.SettingsResetNotice is the same
// string on the other side of the import DAG (paths -> shared -> dbcore ->
// store), which is why it is written out twice; change both together.
const settingsResetNoticeKey = "server.notice.settings_reset"

// theReadersViewLivesInTheDatabase recreates app_settings and, on a database
// that already held models, records that the view and the backup schedule did
// not come across.
//
// NOTHING IS IMPORTED. The old file is not read, parsed or deleted: a converter
// would have to guess at values whose meaning has moved on, and the reader
// rebuilds a view in a minute. The one thing they cannot see is a daily backup
// that has silently stopped, and the notice row is there to tell them.
//
// A DATABASE WITH NO MODELS IS A NEW INSTALL and gets no notice. Every migration
// runs on a fresh file - schemaSQL builds the table, an earlier migration drops
// it, and this brings it back - so the run itself cannot tell the two apart.
func theReadersViewLivesInTheDatabase(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS app_settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("create app_settings: %w", err)
	}
	var models int
	if err := tx.QueryRow("SELECT COUNT(*) FROM models").Scan(&models); err != nil {
		return fmt.Errorf("count the models: %w", err)
	}
	if models == 0 {
		return nil
	}
	if _, err := tx.Exec("INSERT OR REPLACE INTO app_settings (key, value) VALUES (?, ?)", settingsResetNoticeKey, "true"); err != nil {
		return fmt.Errorf("record the settings-reset notice: %w", err)
	}
	log.Printf("The reader's view now lives in the database; the old settings file was not carried across")
	return nil
}

// theCustomColumnTablesAreGone is migration v36.
//
// A ROW IN ANY OF THEM ENDS THE MIGRATION. Nothing in this app has written these
// tables, so an empty one is a leftover of a schema block that recreated them on
// every open, and a full one holds a value that reached the
// database by a route nobody knows about - which exists in no other file.
// Dropping that would be the only copy going, so the migration fails and the
// pre-migration backup is restored over the database instead.
func theCustomColumnTablesAreGone(tx *sql.Tx) error {
	// Children first: custom_values and custom_column_options both reference
	// custom_columns(id) ON DELETE CASCADE.
	for _, table := range []string{"custom_values", "custom_column_options", "custom_columns"} {
		held, err := tableExists(tx, table)
		if err != nil {
			return err
		}
		if !held {
			continue
		}
		// The table names here are this file's own constants, never anything a
		// request carries.
		var stored int
		if err := tx.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&stored); err != nil {
			return fmt.Errorf("count what %s still holds: %w", table, err)
		}
		if stored > 0 {
			return fmt.Errorf("%s still holds %d row(s), which the move onto the cell store should have carried across, so nothing was dropped", table, stored)
		}
		// The index over custom_values(model_name) goes with the table.
		if _, err := tx.Exec("DROP TABLE " + table); err != nil {
			return fmt.Errorf("drop %s: %w", table, err)
		}
		log.Printf("Dropped the empty %s table, which the cell store replaced", table)
	}
	return nil
}

// ---- backup / restore -------------------------------------------------------

// fsStamp is a filesystem-safe UTC timestamp (no colons) for backup filenames.
func fsStamp() string { return time.Now().UTC().Format("20060102-150405") }

// backupDB snapshots the live DB into BackupsDir through SQLite's own VACUUM INTO
// and returns the backup path. fromVer is recorded in the filename. Going through
// SQLite is what puts writes still sitting in the WAL into the one file; a plain
// copy of modelsdb.db could be days behind them and would then be restored over
// the live database.
func backupDB(fromVer int) (string, error) {
	if err := os.MkdirAll(paths.BackupsDir, 0700); err != nil {
		return "", err
	}
	dst := filepath.Join(paths.BackupsDir, fmt.Sprintf("modelsdb-v%d-%s.db", fromVer, fsStamp()))
	dst, err := vacuumSnapshot(paths.BackupsDir, dst, false)
	if err != nil {
		return "", err
	}
	pruneBackups()
	return dst, nil
}

// SnapshotBeforeRewrite snapshots the open database into BackupsDir the way a
// migration does, for a change that rewrites rows without bumping the schema
// (the refresh folding renamed OpenRouter models). It returns the path written.
// The file is named for the schema version the database is at, and pruning
// applies as it does to every file there.
func SnapshotBeforeRewrite() (string, error) {
	cur, err := currentSchemaVersion()
	if err != nil {
		return "", fmt.Errorf("read schema version: %w", err)
	}
	return backupDB(cur)
}

// vacuumSnapshot writes one consistent copy of the open database into dir and
// returns the path it ended at. The copy is made under a working name and given
// its real name when it is finished, so that name never holds a half-written
// file, and it is readable only by its owner because it carries every note the
// reader wrote.
//
// replace says what happens when dst already exists. True replaces it, which is
// the one-file-forever backup. False never touches it: the copy takes the next
// free name beside it (dst with -2, -3 and so on before the extension) and that
// name is returned. Every file in a backup folder is a restore point, and two
// backups in the same second would otherwise share one timestamped name.
func vacuumSnapshot(dir, dst string, replace bool) (string, error) {
	work, err := os.CreateTemp(dir, backupWorkingPattern)
	if err != nil {
		return "", fmt.Errorf("create a working file in %s: %w", dir, err)
	}
	workPath := work.Name()
	if err := work.Close(); err != nil {
		os.Remove(workPath)
		return "", fmt.Errorf("create a working file in %s: %w", dir, err)
	}
	// Run on the pool, not on a connection holding a transaction: SQLite refuses
	// a VACUUM from inside one.
	if _, err := DB.Exec("VACUUM INTO ?", workPath); err != nil {
		os.Remove(workPath)
		return "", fmt.Errorf("copy the database into %s (no backup written): %w", dir, err)
	}
	final := dst
	if replace {
		err = os.Rename(workPath, dst)
	} else {
		final, err = nameWithoutReplacing(workPath, dst)
	}
	if err != nil {
		os.Remove(workPath)
		return "", fmt.Errorf("give the finished backup its name %s (no backup written): %w", dst, err)
	}
	return final, nil
}

// maxNameClashes bounds how many numbered names nameWithoutReplacing tries.
const maxNameClashes = 1000

// nameWithoutReplacing gives the finished file at workPath the name dst, or the
// first free numbered variant of it, and returns the name it took. The working
// file is gone afterwards on success.
//
// THE CHECK AND THE CLAIM ARE ONE STEP. A hard link fails when its target
// exists, and nothing else can create that name between the test and the claim,
// so a file already there is never replaced. A folder that cannot hold hard
// links (some removable and network volumes) falls back to claiming the name
// with O_EXCL and renaming the finished file over that claim, which is atomic
// for the claim and only leaves an empty file at the name if the process dies
// in between.
func nameWithoutReplacing(workPath, dst string) (string, error) {
	ext := filepath.Ext(dst)
	stem := strings.TrimSuffix(dst, ext)
	for n := 1; n <= maxNameClashes; n++ {
		candidate := dst
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d%s", stem, n, ext)
		}
		err := os.Link(workPath, candidate)
		if err == nil {
			os.Remove(workPath)
			return candidate, nil
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		claim, cerr := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(cerr, fs.ErrExist) {
			continue
		}
		if cerr != nil {
			return "", err
		}
		claim.Close()
		if rerr := os.Rename(workPath, candidate); rerr != nil {
			os.Remove(candidate)
			return "", rerr
		}
		return candidate, nil
	}
	return "", fmt.Errorf("%d backups already share the name %s", maxNameClashes, filepath.Base(dst))
}

// freeName returns path, or the first numbered variant of it that does not exist.
// It is a check without a claim, for a caller that copies the file in itself and
// runs while nothing else writes to the folder.
func freeName(path string) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	candidate := path
	for n := 2; ; n++ {
		if _, err := os.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, n, ext)
	}
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

// copyFile copies src to dst so that dst is either the old file or the complete
// new one: the bytes are written and synced under a working name beside dst and
// renamed onto it. A new dst is readable only by its owner, since it may hold
// the reader's notes; an existing dst keeps the permissions it already had.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	mode := os.FileMode(0600)
	if info, err := os.Stat(dst); err == nil {
		mode = info.Mode().Perm()
	}
	work, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	workPath := work.Name()
	fail := func(err error) error {
		work.Close()
		os.Remove(workPath)
		return err
	}
	if _, err := io.Copy(work, in); err != nil {
		return fail(err)
	}
	if err := work.Sync(); err != nil {
		return fail(err)
	}
	if err := work.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := work.Close(); err != nil {
		os.Remove(workPath)
		return err
	}
	if err := os.Rename(workPath, dst); err != nil {
		os.Remove(workPath)
		return err
	}
	return nil
}

// dbBackups returns the backup files in BackupsDir, oldest first.
func dbBackups() []os.DirEntry {
	entries, err := os.ReadDir(paths.BackupsDir)
	if err != nil {
		return nil
	}
	type stamped struct {
		entry os.DirEntry
		mod   time.Time
	}
	var found []stamped
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		// A file removed between the listing and this call is simply not a backup
		// any more.
		info, err := e.Info()
		if err != nil {
			continue
		}
		found = append(found, stamped{entry: e, mod: info.ModTime()})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].mod.Before(found[j].mod) })
	files := make([]os.DirEntry, 0, len(found))
	for _, f := range found {
		files = append(files, f.entry)
	}
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
// or a filename within BackupsDir. The caller must have checked that no server
// holds the database: nothing here can tell.
func RestoreBackup(arg string) error {
	src := arg
	if !filepath.IsAbs(src) && !paths.FileExists(src) {
		src = filepath.Join(paths.BackupsDir, arg)
	}
	if !paths.FileExists(src) {
		return fmt.Errorf("backup not found: %s", arg)
	}

	// Refuse a below-floor candidate BEFORE copying anything: this build cannot
	// migrate it forward, so putting it in place would leave the install unable
	// to start on the next run, with the copy itself having reported success.
	if v, err := readVersionReadOnly(src); err == nil && v < schemaFloor {
		return fmt.Errorf("%s is schema version %d, below this build's floor of %d - restoring it would leave the install unable to start; see docs/migration.md", src, v, schemaFloor)
	}

	if paths.FileExists(paths.DbFile) {
		os.MkdirAll(paths.BackupsDir, 0700)
		safety := filepath.Join(paths.BackupsDir, fmt.Sprintf("modelsdb-prerestore-%s.db", fsStamp()))
		if DB != nil {
			// A live connection: SQLite writes the snapshot, so what still sits in
			// the WAL is in it and one file is the whole story.
			taken, err := vacuumSnapshot(paths.BackupsDir, safety, false)
			if err != nil {
				return fmt.Errorf("could not snapshot current DB before restore: %w", err)
			}
			safety = taken
		} else {
			// restore/backups run with no connection open (see cli.go), so there is
			// nothing to checkpoint - without one the .db file alone can be behind
			// the data, so its WAL and SHM sidecars come with it.
			safety = freeName(safety)
			if err := copyFile(paths.DbFile, safety); err != nil {
				return fmt.Errorf("could not snapshot current DB before restore: %w", err)
			}
			for _, ext := range []string{"-wal", "-shm"} {
				if !paths.FileExists(paths.DbFile + ext) {
					continue
				}
				if err := copyFile(paths.DbFile+ext, safety+ext); err != nil {
					return fmt.Errorf("could not snapshot current DB's %s sidecar before restore: %w", ext, err)
				}
			}
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
