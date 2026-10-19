package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v48: the per-key pricing.overrides and
// benchmarks.design_arena columns are gone.
//
// Before this migration, catalog.walkFeedRecord walked a non-empty array of
// objects element by element and unioned every element's own keys into
// separate columns - "pricing.overrides[].prompt", "pricing.overrides[].
// utc_start" and the like. Each such column's fill took the FIRST window that
// happened to state that particular key (catalog.FillFeedFieldColumn), so a
// model with several price windows could show a cell pairing one window's
// price with a different window's hours - never a fact any single feed
// record stated.
//
// From this build on, walkFeedValue records such an array as ONE list leaf
// at its own path and never descends into it, so the next refresh fills a
// single pricing_overrides column - and, on a feed that has made one,
// benchmarks_design_arena - one member per window or row, each element's own
// keys joined in the order the feed sent them. This migration retires what
// the old element-by-element walk made, so the composite columns cannot go
// on being read.
//
// WHAT IT TOUCHES.
//   - columns: every column keyed "pricing_overrides_*" (the eleven the old
//     walk made off pricing.overrides[].*) and every column keyed
//     "benchmarks_design_arena_*" if the feed had made one; DELETE FROM
//     columns cascades to their column_options, column_sources and cells.
//   - feed_fields: the "[]"-bearing paths those columns were tied to keep
//     their row (first_seen/last_seen history), but column_id becomes NULL
//     (ON DELETE SET NULL, feedfields.go), because nothing reads them any
//     more and no future refresh writes one of those paths again - walkFeedValue
//     no longer produces a "[]" path for an array of objects at all.
//
// cellsMayFall IS SET: dropping the cells those composite columns held is
// this migration's whole purpose, so the chain's ordinary guard (migrate.go)
// is off for it, and its own exact refusal below stands in its place, on
// v42's and v47's pattern: the cell count must fall by exactly what the
// retired columns held, or the pre-migration snapshot comes back.
func theOverrideWindowColumnsAreRetired(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to retire
	}

	perColumn, err := countRetiredOverrideColumns(tx)
	if err != nil {
		return err
	}
	if len(perColumn) == 0 {
		return nil
	}
	var expectedCells int
	for _, c := range perColumn {
		expectedCells += c.count
	}

	var before int
	if err := tx.QueryRow("SELECT COUNT(*) FROM cells").Scan(&before); err != nil {
		return fmt.Errorf("count the cells before retiring the override-window columns: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM columns WHERE column_key GLOB 'pricing_overrides_*' OR column_key GLOB 'benchmarks_design_arena_*'`); err != nil {
		return fmt.Errorf("delete the retired override-window columns: %w", err)
	}

	var after int
	if err := tx.QueryRow("SELECT COUNT(*) FROM cells").Scan(&after); err != nil {
		return fmt.Errorf("count the cells after retiring the override-window columns: %w", err)
	}
	if drop := before - after; drop != expectedCells {
		return fmt.Errorf("refusing to commit: deleting %d retired override-window column(s) should have dropped %d cell(s), and %d went",
			len(perColumn), expectedCells, drop)
	}

	log.Printf("Migration v48: deleted %d retired override-window column(s) and %d cell(s) they held", len(perColumn), expectedCells)
	for _, c := range perColumn {
		log.Printf("Migration v48: deleted the column %s and its %d cell(s)", c.columnKey, c.count)
	}
	return nil
}

// countRetiredOverrideColumns counts, per column, the cells held by every
// column this migration is about to delete - measured BEFORE anything is
// deleted, which is what the migration's own refusal is checked against
// afterwards. ruleColumnCount is v42's type (ruledeath.go), reused here for
// the same shape: a column key and how many cells it held.
func countRetiredOverrideColumns(tx *sql.Tx) ([]ruleColumnCount, error) {
	rows, err := tx.Query(`SELECT col.column_key, COUNT(c.model_name) FROM columns col
		LEFT JOIN cells c ON c.column_id = col.id
		WHERE col.column_key GLOB 'pricing_overrides_*' OR col.column_key GLOB 'benchmarks_design_arena_*'
		GROUP BY col.id ORDER BY col.column_key`)
	if err != nil {
		return nil, fmt.Errorf("count the cells on the retired override-window columns: %w", err)
	}
	defer rows.Close()
	var out []ruleColumnCount
	for rows.Next() {
		var r ruleColumnCount
		if err := rows.Scan(&r.columnKey, &r.count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
