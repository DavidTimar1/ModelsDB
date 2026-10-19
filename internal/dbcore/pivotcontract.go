package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v41: the contraction that follows v40. v40 widened
// cells' key and collapsed the columns that state one attribute onto one; this
// migration removes what the old, namespace-in-the-key shape needed and no
// longer does, and adds what the new shape guards itself with.
//
// WHAT IT TOUCHES.
//   - columns: rebuilt without namespace and without the two CHECKs that
//     named it, replaced by CHECK (instr(column_key,'.') = 0) and a unique
//     index on label COLLATE NOCASE. The rebuild also strips every column's
//     namespace prefix in one pass - v40 could not de-prefix even its own
//     survivors, because the old CHECK (column_key LIKE namespace || '.%') was
//     still live for the whole of that migration, so every column reaches this
//     one still carrying its prefix.
//   - cells: two triggers. cells_source_is_declared refuses a write whose
//     source is not one column_sources says can fill that column.
//     cells_source_never_moves refuses an UPDATE that would move a stored
//     value onto a different column or source - the only route by which a
//     later write could land on a value it does not own.
//   - answers: a view returning the winning row per column per model, by
//     lowest column_sources.rank. A column with no column_sources row (there
//     should be none left after v40) is treated as lowest priority rather
//     than excluded, so a future gap here fails soft.
//   - derived_columns and derived_inputs: dropped. Every recipe they held is
//     now either a column_sources row (v40) or nothing, because the column it
//     described no longer exists as a column of its own.
//   - source_records.namespace is renamed to source, matching what every other
//     table now calls the same fact.
//
// No cell is dropped by anything here either: cellsMayFall is not set.
func theSchemaContractsForThePivot(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to contract
	}

	if err := rebuildColumnsWithoutNamespace(tx); err != nil {
		return err
	}
	if err := addCellWriteGuardTriggers(tx); err != nil {
		return err
	}
	if err := createAnswersView(tx); err != nil {
		return err
	}
	if err := dropDerivedColumnSubsystem(tx); err != nil {
		return err
	}
	if err := renameSourceRecordsNamespace(tx); err != nil {
		return err
	}
	return nil
}

// rebuildColumnsWithoutNamespace drops the namespace column, the CHECK that
// tied it to column_key, and the CHECK that pinned namespace to five values,
// replacing them with a single dot-free CHECK on column_key and a case-
// insensitive unique index on label. SQLite cannot drop
// a CHECK with ALTER TABLE, so this is create-new/copy/drop-old/rename-new,
// same as v40's cells rebuild, and needs the same disableForeignKeys: every
// other table in the cell store references columns(id).
func rebuildColumnsWithoutNamespace(tx *sql.Tx) error {
	dupes, err := duplicateLabelsCaseInsensitive(tx)
	if err != nil {
		return err
	}
	if len(dupes) > 0 {
		return fmt.Errorf("%d label(s) are shared by more than one column after the collapse, which the unique index refuses: %v", len(dupes), dupes)
	}

	if _, err := tx.Exec(`CREATE TABLE columns_new (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		column_key  TEXT    NOT NULL UNIQUE,
		label       TEXT    NOT NULL,
		value_type  TEXT    NOT NULL CHECK (value_type IN ('text','number','flag','choice','list')),
		unit        TEXT    NOT NULL DEFAULT '',
		about       TEXT    NOT NULL DEFAULT '',
		position    INTEGER NOT NULL DEFAULT 0,
		visible     INTEGER NOT NULL DEFAULT 1,
		sealed      INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT    NOT NULL DEFAULT '',
		color_rules TEXT    NOT NULL DEFAULT '',
		resolve     TEXT    NOT NULL DEFAULT 'first_stated' CHECK (resolve IN ('first_stated','union')),
		CHECK (instr(column_key,'.') = 0)
	)`); err != nil {
		return fmt.Errorf("create the namespace-free columns table: %w", err)
	}
	// substr(key, instr(key,'.')+1) strips a "namespace." prefix when there is
	// one, and is a no-op when there is not (instr returns 0, so the cut point
	// is 1) - every column still carries its prefix reaching this migration, so
	// every one loses it here.
	if _, err := tx.Exec(`INSERT INTO columns_new
		(id, column_key, label, value_type, unit, about, position, visible, sealed, created_at, color_rules, resolve)
		SELECT id, substr(column_key, instr(column_key,'.')+1), label, value_type, unit, about, position, visible, sealed, created_at, color_rules, resolve
		FROM columns`); err != nil {
		return fmt.Errorf("copy columns without their namespace prefix: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE columns`); err != nil {
		return fmt.Errorf("drop the old columns table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE columns_new RENAME TO columns`); err != nil {
		return fmt.Errorf("rename the namespace-free columns table into place: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX idx_columns_label ON columns(label COLLATE NOCASE)`); err != nil {
		return fmt.Errorf("index columns.label: %w", err)
	}
	return nil
}

// duplicateLabelsCaseInsensitive names any label held by more than one
// column. There are none once the collapse has run, so finding one here means
// the data disagreed with that expectation, and the migration fails loudly
// instead of building an index that silently could not enforce what it claims
// to.
func duplicateLabelsCaseInsensitive(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT label FROM columns GROUP BY label COLLATE NOCASE HAVING COUNT(*) > 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

// addCellWriteGuardTriggers adds the database's own layer of the write guard.
// The primary key is the first layer and needs no code; store.WriteCells
// declaring a typed Source is the second; these two triggers are the third.
func addCellWriteGuardTriggers(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TRIGGER cells_source_is_declared
		BEFORE INSERT ON cells
		FOR EACH ROW WHEN NOT EXISTS (
			SELECT 1 FROM column_sources WHERE column_id = NEW.column_id AND source = NEW.source
		)
		BEGIN
			SELECT RAISE(ABORT, 'cells.source is not declared in column_sources for this column');
		END`); err != nil {
		return fmt.Errorf("add the cells_source_is_declared trigger: %w", err)
	}
	if _, err := tx.Exec(`CREATE TRIGGER cells_source_never_moves
		BEFORE UPDATE OF column_id, source ON cells
		FOR EACH ROW WHEN NEW.column_id <> OLD.column_id OR NEW.source <> OLD.source
		BEGIN
			SELECT RAISE(ABORT, 'a cell''s column and source are fixed once written; delete and re-insert instead');
		END`); err != nil {
		return fmt.Errorf("add the cells_source_never_moves trigger: %w", err)
	}
	return nil
}

// createAnswersView adds the one read every attribute now resolves through:
// the cell with the lowest column_sources.rank for its column and model. A
// cell whose source carries no column_sources row (none should remain after
// v40, but a future column added outside that path could still lack one) is
// treated as the lowest priority rather than left out, so a missing rank
// never hides a value that exists nowhere else.
func createAnswersView(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE VIEW answers AS
		SELECT c.column_id, c.model_name, c.source, c.value_text, c.value_number, c.written_at
		FROM cells c
		LEFT JOIN column_sources cs ON cs.column_id = c.column_id AND cs.source = c.source
		WHERE NOT EXISTS (
			SELECT 1 FROM cells c2
			LEFT JOIN column_sources cs2 ON cs2.column_id = c2.column_id AND cs2.source = c2.source
			WHERE c2.column_id = c.column_id AND c2.model_name = c.model_name
			  AND COALESCE(cs2.rank, 2147483647) < COALESCE(cs.rank, 2147483647)
		)`); err != nil {
		return fmt.Errorf("create the answers view: %w", err)
	}
	return nil
}

// dropDerivedColumnSubsystem removes derived_columns and derived_inputs.
// Nothing reads them after this migration: v40 already turned every recipe
// they held into column_sources rows (or, for a merged-away input, deleted
// the row outright alongside the column it pointed at), so what is left here
// is the recipe metadata itself - which column_sources has already replaced.
func dropDerivedColumnSubsystem(tx *sql.Tx) error {
	for _, table := range []string{"derived_inputs", "derived_columns"} {
		held, err := tableExists(tx, table)
		if err != nil {
			return err
		}
		if !held {
			continue
		}
		var n int
		if err := tx.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			return fmt.Errorf("count what %s still holds: %w", table, err)
		}
		if _, err := tx.Exec("DROP TABLE " + table); err != nil {
			return fmt.Errorf("drop %s: %w", table, err)
		}
		if n > 0 {
			log.Printf("Migration v41: dropped %s, discarding %d row(s) already superseded by column_sources", table, n)
		}
	}
	return nil
}

// renameSourceRecordsNamespace renames source_records.namespace to source,
// so the one other table that named "who said this" by the old word now
// names it the same way cells and column_sources do. SQLite's RENAME COLUMN
// updates the table's own primary key definition (model_name, namespace) to
// match, so no rebuild is needed here.
func renameSourceRecordsNamespace(tx *sql.Tx) error {
	held, err := tableExists(tx, "source_records")
	if err != nil {
		return err
	}
	if !held {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE source_records RENAME COLUMN namespace TO source`); err != nil {
		return fmt.Errorf("rename source_records.namespace to source: %w", err)
	}
	return nil
}
