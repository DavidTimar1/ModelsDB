package dbcore

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// This file is migration v47: the collection source is gone.
//
// The two OpenRouter web-page scrapes that filled it no longer exist, so nothing
// writes it, yet on a database that holds one a scraped price or type still
// outranked OpenRouter's current answer. Under the rule v46 landed there are two
// kinds of column, and a source nothing refreshes is neither.
//
// WHAT IT TOUCHES.
//   - cells: a collection cell on a column that survives becomes a research
//     cell (a scraped price on a model the API omits still shows, and is
//     replaced the moment the feed states one) UNLESS that column and model
//     already hold a research cell, which v46 made from what the reader typed
//     over the scrape - what they typed wins, and the scraped cell is dropped
//     and counted - or UNLESS the automatic winner on that column and model
//     holds the same text, in which case the scrape adds nothing to what is
//     shown and is dropped and counted as a duplicate rather than left as a
//     research cell the edit form would offer to clear on hundreds of models.
//     Then every collection cell is deleted.
//   - columns: collections, price_display and pricing_url, the three columns
//     only the scrapes ever filled, are deleted with their cascade
//     (column_options, column_sources, and any cell still on them).
//   - column_sources, sources, source_records: every row naming collection.
//     The verbatim page text the scrapes stored can be read by nothing once
//     the source is gone.
//
// THIS MIGRATION DROPS CELLS ON PURPOSE, so cellsMayFall is set and the chain's
// guard is off for it. In its place is its own exact refusal, on v42's pattern:
// the drop in the cell count must equal the three columns' cells plus the
// collision and duplicate cells counted before anything moved, and no collection
// cell may remain. Any other number aborts the migration, which restores the
// pre-migration snapshot. The log names every count, so what went can be read
// afterwards.
func theCollectionSourceIsGone(tx *sql.Tx) error {
	held, err := tableExists(tx, "sources")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to remove
	}

	before, err := countCollectionState(tx)
	if err != nil {
		return err
	}

	restamped, err := restampSurvivingCollectionCells(tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM cells WHERE source = 'collection'`); err != nil {
		return fmt.Errorf("delete the collection cells: %w", err)
	}
	if err := deleteCollectionOnlyColumns(tx); err != nil {
		return err
	}
	for _, stmt := range []string{
		`DELETE FROM column_sources WHERE source = 'collection'`,
		`DELETE FROM sources WHERE name = 'collection'`,
		`DELETE FROM source_records WHERE source = 'collection'`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("remove the collection source: %s: %w", stmt, err)
		}
	}

	if err := refuseUnlessTheDropIsExact(tx, before, restamped); err != nil {
		return err
	}
	for _, c := range before.perColumn {
		log.Printf("Migration v47: deleted the column %s and its %d cell(s)", c.columnKey, c.count)
	}
	if before.collectionCells > 0 {
		log.Printf("Migration v47: %d collection cell(s) re-stamped as research, %d dropped where the reader had typed over them, %d collection cell(s) dropped as duplicates of the automatic answer, %d collection cell(s) gone in all",
			restamped, before.collisions, before.duplicates, before.collectionCells)
	}
	return nil
}

// collectionOnlyColumns are the columns only the scrapes ever filled. Found by
// key, as v37 found columns, because a column's id is whatever the database
// gave it.
var collectionOnlyColumns = []string{"collections", "price_display", "pricing_url"}

// collectionState is what the migration counts before it moves anything,
// which is what its refusal is measured against afterwards.
type collectionState struct {
	perColumn       []ruleColumnCount // cells on each of the three columns, whoever stated them
	offThree        int               // collection cells on a column that survives
	collisions      int               // of those, the ones a research cell already outranks
	duplicates      int               // of those, the ones holding the automatic winner's own text
	collectionCells int               // every collection cell
	cells           int               // every cell
}

// threeColumnsIn is the placeholder list for collectionOnlyColumns in a query.
func threeColumnsIn() (string, []interface{}) {
	places := make([]string, len(collectionOnlyColumns))
	args := make([]interface{}, len(collectionOnlyColumns))
	for i, key := range collectionOnlyColumns {
		places[i] = "?"
		args[i] = key
	}
	return strings.Join(places, ","), args
}

// collisionSQL selects every collection cell that is NOT on one of the three
// columns and whose column and model already hold a research cell: the scrape
// the reader typed over, which v46 made research.
const collisionSQL = `SELECT c.column_id, c.model_name FROM cells c
	JOIN columns col ON col.id = c.column_id
	WHERE c.source = 'collection' AND col.column_key NOT IN (%s)
	  AND EXISTS (SELECT 1 FROM cells r WHERE r.column_id = c.column_id AND r.model_name = c.model_name AND r.source = 'research')`

// duplicateOfWinnerSQL is the condition on a collection cell c whose text is
// the automatic winner's own on that column and model: the OpenRouter or
// HuggingFace cell with the lowest rank there holds the same value_text. Such
// a cell adds nothing to what is shown and is dropped rather than re-stamped.
const duplicateOfWinnerSQL = `EXISTS (
	SELECT 1 FROM cells a JOIN column_sources cs ON cs.column_id = a.column_id AND cs.source = a.source
	WHERE a.column_id = c.column_id AND a.model_name = c.model_name
	  AND a.source IN ('openrouter','huggingface') AND a.value_text = c.value_text
	  AND NOT EXISTS (
		SELECT 1 FROM cells b JOIN column_sources cb ON cb.column_id = b.column_id AND cb.source = b.source
		WHERE b.column_id = a.column_id AND b.model_name = a.model_name
		  AND b.source IN ('openrouter','huggingface') AND cb.rank < cs.rank))`

// duplicateSQL selects every collection cell on a surviving column that is
// not a collision and is a duplicate of the automatic winner.
const duplicateSQL = `SELECT c.column_id, c.model_name FROM cells c
	JOIN columns col ON col.id = c.column_id
	WHERE c.source = 'collection' AND col.column_key NOT IN (%s)
	  AND NOT EXISTS (SELECT 1 FROM cells r WHERE r.column_id = c.column_id AND r.model_name = c.model_name AND r.source = 'research')
	  AND ` + duplicateOfWinnerSQL

func countCollectionState(tx *sql.Tx) (collectionState, error) {
	var st collectionState
	places, args := threeColumnsIn()

	rows, err := tx.Query(`SELECT col.column_key, COUNT(c.model_name) FROM columns col
		LEFT JOIN cells c ON c.column_id = col.id
		WHERE col.column_key IN (`+places+`) GROUP BY col.id ORDER BY col.column_key`, args...)
	if err != nil {
		return st, fmt.Errorf("count the cells on the three collection-only columns: %w", err)
	}
	for rows.Next() {
		var r ruleColumnCount
		if err := rows.Scan(&r.columnKey, &r.count); err != nil {
			rows.Close()
			return st, err
		}
		st.perColumn = append(st.perColumn, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return st, err
	}

	if err := tx.QueryRow(`SELECT COUNT(*) FROM (`+fmt.Sprintf(collisionSQL, places)+`)`, args...).Scan(&st.collisions); err != nil {
		return st, fmt.Errorf("count the collection cells the reader typed over: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM (`+fmt.Sprintf(duplicateSQL, places)+`)`, args...).Scan(&st.duplicates); err != nil {
		return st, fmt.Errorf("count the collection cells that duplicate the automatic answer: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells c JOIN columns col ON col.id = c.column_id
		WHERE c.source = 'collection' AND col.column_key NOT IN (`+places+`)`, args...).Scan(&st.offThree); err != nil {
		return st, fmt.Errorf("count the collection cells on surviving columns: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE source = 'collection'`).Scan(&st.collectionCells); err != nil {
		return st, fmt.Errorf("count the collection cells: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells`).Scan(&st.cells); err != nil {
		return st, fmt.Errorf("count the cells: %w", err)
	}
	return st, nil
}

// restampSurvivingCollectionCells inserts as research every collection cell
// on a surviving column whose column and model hold no research cell yet and
// whose text is not the automatic winner's own, and answers how many it
// inserted. research is declared on every such column by v46, because
// collection is a source of kind 'fetched'.
func restampSurvivingCollectionCells(tx *sql.Tx) (int64, error) {
	places, args := threeColumnsIn()
	res, err := tx.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
		SELECT c.column_id, c.model_name, 'research', c.value_text, c.value_number, c.written_at
		FROM cells c JOIN columns col ON col.id = c.column_id
		WHERE c.source = 'collection' AND col.column_key NOT IN (`+places+`)
		  AND NOT EXISTS (SELECT 1 FROM cells r WHERE r.column_id = c.column_id AND r.model_name = c.model_name AND r.source = 'research')
		  AND NOT `+duplicateOfWinnerSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("re-stamp the surviving collection cells as research: %w", err)
	}
	return res.RowsAffected()
}

// deleteCollectionOnlyColumns deletes the three columns; foreign keys are on
// for this migration, so their options, declarations and cells cascade.
func deleteCollectionOnlyColumns(tx *sql.Tx) error {
	places, args := threeColumnsIn()
	if _, err := tx.Exec(`DELETE FROM columns WHERE column_key IN (`+places+`)`, args...); err != nil {
		return fmt.Errorf("delete the three collection-only columns: %w", err)
	}
	return nil
}

// refuseUnlessTheDropIsExact is the migration's own guard: every collection
// cell on a surviving column was re-stamped, a counted collision or a counted
// duplicate; the cells that went are exactly the three columns' cells plus
// those collisions and duplicates; and no collection cell is left. Anything
// else means the data did not have the shape this file describes, and the
// snapshot comes back.
func refuseUnlessTheDropIsExact(tx *sql.Tx, before collectionState, restamped int64) error {
	var after, collectionLeft int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells`).Scan(&after); err != nil {
		return fmt.Errorf("count the cells after the removal: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE source = 'collection'`).Scan(&collectionLeft); err != nil {
		return fmt.Errorf("count the collection cells after the removal: %w", err)
	}
	if int(restamped) != before.offThree-before.collisions-before.duplicates {
		return fmt.Errorf("refusing to commit: %d collection cell(s) sat on surviving columns, %d of them typed over and %d duplicates of the automatic answer, so %d should have become research, and %d did",
			before.offThree, before.collisions, before.duplicates, before.offThree-before.collisions-before.duplicates, restamped)
	}
	expected := before.collisions + before.duplicates
	for _, c := range before.perColumn {
		expected += c.count
	}
	if drop := before.cells - after; drop != expected {
		return fmt.Errorf("refusing to commit: the cell count fell by %d, and the three columns' cells plus the typed-over and duplicate collection cells account for %d", drop, expected)
	}
	if collectionLeft > 0 {
		return fmt.Errorf("refusing to commit: %d collection cell(s) remain", collectionLeft)
	}
	return nil
}
