package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v39: the schema expansion that lets one column hold a
// cell per source. It is purely additive - two new tables, and two new columns on
// tables that already exist - so nothing that reads the database is affected by
// it.
//
// WHY THE NEW TABLES ARE CREATED HERE RATHER THAN IN db.go. coreSchemaSQL and
// cellStoreSQL (db.go) are the FLOOR schema: exactly what a database at schema
// version 33 holds, and they change only when the floor itself rises (see the
// comment on schemaSQL). Everything a migration after v33 adds is created inside
// that migration's own Go, never by editing the floor declaration.
//
// WHAT IT TOUCHES.
//   - sources: the places an answer can come from, seeded once.
//   - column_sources: empty except where a computed column already had a
//     derived_inputs recipe - see backfillColumnSourcesFromDerivedInputs.
//   - columns.resolve: every column defaults to first_stated, because that is
//     what every column already behaves as.
//   - cells.source: backfilled from the column's own namespace, via
//     sourceForNamespace - so every cell that already exists ends this
//     migration with a source, and nothing about what it holds changes.
func theSchemaExpandsForThePivot(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, so there is nothing to expand
	}

	if _, err := tx.Exec(`ALTER TABLE columns ADD COLUMN resolve TEXT NOT NULL DEFAULT 'first_stated'
		CHECK (resolve IN ('first_stated','union'))`); err != nil {
		return fmt.Errorf("add columns.resolve: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE cells ADD COLUMN source TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add cells.source: %w", err)
	}

	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS sources (
		name     TEXT    PRIMARY KEY NOT NULL,
		label    TEXT    NOT NULL,
		kind     TEXT    NOT NULL CHECK (kind IN ('fetched','hand','static')),
		position INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create sources: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS column_sources (
		column_id INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
		source    TEXT    NOT NULL REFERENCES sources(name),
		rank      INTEGER NOT NULL,
		PRIMARY KEY (column_id, source)
	)`); err != nil {
		return fmt.Errorf("create column_sources: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_column_sources_rank ON column_sources(column_id, rank)`); err != nil {
		return fmt.Errorf("index column_sources.rank: %w", err)
	}

	if err := seedPivotSources(tx); err != nil {
		return err
	}

	stamped, err := backfillCellSources(tx)
	if err != nil {
		return err
	}
	if stamped > 0 {
		log.Printf("Migration v39: stamped %d existing cell(s) with the source that filled them", stamped)
	}

	recorded, err := backfillColumnSourcesFromDerivedInputs(tx)
	if err != nil {
		return err
	}
	if recorded > 0 {
		log.Printf("Migration v39: recorded the source order for %d computed column(s) from their stored recipe", recorded)
	}
	return nil
}

// pivotSource is one row of the seeded sources table.
type pivotSource struct {
	name, label, kind string
	position          int
}

// pivotSources are the places an answer can come from, in display order. 'rule'
// is deliberately absent: it is what a derived column's own cells are stamped
// with by backfillCellSources, and the derived-column subsystem is deleted by a
// later migration, so it is never a source a column can be told to read FROM -
// only a label a cell can carry until then.
var pivotSources = []pivotSource{
	{"openrouter", "OpenRouter", "fetched", 0},
	{"huggingface", "HuggingFace", "fetched", 1},
	{"collection", "Collection page", "fetched", 2},
	{"hand", "Typed by hand", "hand", 3},
	{"app", "The app itself", "static", 4},
}

func seedPivotSources(tx *sql.Tx) error {
	for _, s := range pivotSources {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO sources (name, label, kind, position) VALUES (?,?,?,?)`,
			s.name, s.label, s.kind, s.position); err != nil {
			return fmt.Errorf("seed source %q: %w", s.name, err)
		}
	}
	return nil
}

// sourceForNamespace is the mapping backfillCellSources and
// backfillColumnSourcesFromDerivedInputs both use: a cell in a mine column was
// typed by hand, a cell in a derived column was worked out by a rule, and the
// other three namespaces already name the source that stated them.
func sourceForNamespace(namespace string) string {
	switch namespace {
	case "mine":
		return "hand"
	case "derived":
		return "rule"
	default:
		return namespace
	}
}

// backfillCellSources stamps every existing cell with the source implied by
// its column's namespace. It runs in three passes rather than one correlated
// UPDATE so each pass reads as one sentence: first copy the namespace across
// verbatim, then correct the two namespaces whose source is spelled
// differently.
func backfillCellSources(tx *sql.Tx) (int64, error) {
	res, err := tx.Exec(`UPDATE cells SET source = (
		SELECT namespace FROM columns WHERE columns.id = cells.column_id
	) WHERE source = ''`)
	if err != nil {
		return 0, fmt.Errorf("copy each cell's column namespace into source: %w", err)
	}
	stamped, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE cells SET source = 'hand' WHERE source = 'mine'`); err != nil {
		return 0, fmt.Errorf("rename the mine source to hand: %w", err)
	}
	if _, err := tx.Exec(`UPDATE cells SET source = 'rule' WHERE source = 'derived'`); err != nil {
		return 0, fmt.Errorf("rename the derived source to rule: %w", err)
	}
	return stamped, nil
}

// backfillColumnSourcesFromDerivedInputs records, for every computed column
// that already has a derived_inputs recipe, the source order that recipe
// implies: one column_sources row per distinct source, ranked by the
// position of the first input in the recipe that maps to it. It is a
// snapshot of a recipe the derived-column subsystem still owns in this
// stage - the stage that collapses columns onto one per attribute is what
// reads it.
func backfillColumnSourcesFromDerivedInputs(tx *sql.Tx) (int, error) {
	// v41 drops derived_columns and derived_inputs once this migration's own
	// backfill (and v40's collapse) have both read everything out of them, so
	// this call must stay safe to make against a post-v41 database too - the
	// existing tableExists guard below is what does that: a database that has
	// already moved past this recipe answers "nothing to record" rather than
	// failing on a table that is gone by design, not by accident.
	held, err := tableExists(tx, "derived_columns")
	if err != nil {
		return 0, err
	}
	if !held {
		return 0, nil
	}

	rows, err := tx.Query("SELECT column_id FROM derived_columns")
	if err != nil {
		return 0, err
	}
	var derivedIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		derivedIDs = append(derivedIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	recorded := 0
	for _, derivedID := range derivedIDs {
		inputs, err := orderedDerivedInputs(tx, derivedID)
		if err != nil {
			return 0, err
		}
		rank := 0
		for _, inputID := range inputs {
			var namespace string
			if err := tx.QueryRow("SELECT namespace FROM columns WHERE id=?", inputID).Scan(&namespace); err != nil {
				return 0, fmt.Errorf("read the namespace of input column %d: %w", inputID, err)
			}
			res, err := tx.Exec(`INSERT OR IGNORE INTO column_sources (column_id, source, rank) VALUES (?,?,?)`,
				derivedID, sourceForNamespace(namespace), rank)
			if err != nil {
				return 0, fmt.Errorf("record the source order for column %d: %w", derivedID, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				rank++
			}
		}
		if rank > 0 {
			recorded++
		}
	}
	return recorded, nil
}
