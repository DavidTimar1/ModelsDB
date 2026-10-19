package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v42: deletes every cell the derived-column subsystem
// left stamped source='rule'. 'rule' is deliberately not a row of
// sources and holds no column_sources row (pivotexpand.go's pivotSources), so
// the answers view (pivotcontract.go's createAnswersView) coalesces a missing
// rank to the largest possible integer - a rule cell can only ever win there
// when it is the ONLY cell on record for its column and model. Every other
// case already has a real source outranking it, so deleting these cells never
// changes what the app has been serving.
//
// On a catalog measured before this migration was written, nearly every rule
// cell already agreed with what the view computes from the surviving cells, every
// disagreement was already outranked by a live openrouter cell, and none was the
// sole cell for its column and model. That is one catalog, not a proof, which is
// exactly what the guard below exists for.
//
// WHAT IT TOUCHES.
//   - cells: every row with source='rule' is removed, and nothing else.
//
// THIS IS THE ONE MIGRATION ALLOWED TO DROP CELLS. cellsMayFall is set on its
// registry entry precisely so migrate()'s per-migration guard does not treat
// this legitimate drop as the failure it exists to catch - see the comment on
// the cellsMayFall field (migrate.go) for why that guard runs around each
// migration rather than once around the whole chain.
func theDeadRuleCellsAreDeleted(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to delete
	}

	if err := refuseIfARuleCellIsTheOnlyAnswer(tx); err != nil {
		return err
	}

	perColumn, err := countRuleCellsPerColumn(tx)
	if err != nil {
		return err
	}
	if len(perColumn) == 0 {
		return nil
	}

	if _, err := tx.Exec(`DELETE FROM cells WHERE source = 'rule'`); err != nil {
		return fmt.Errorf("delete the rule cells: %w", err)
	}
	for _, row := range perColumn {
		log.Printf("Migration v42: deleted %d rule cell(s) for column %s", row.count, row.columnKey)
	}
	return nil
}

// ruleColumnCount is one column's share of the rule cells this migration
// counts, either to refuse the run or to log what it deleted.
type ruleColumnCount struct {
	columnKey string
	count     int
}

// refuseIfARuleCellIsTheOnlyAnswer aborts the migration - which restores the
// pre-migration backup, see migrate.go's failMigration - the moment a rule
// cell exists with no non-rule sibling for the same column and model. Such a
// cell IS the served answer today (the answers view has nothing else to
// return), so deleting it would silently take that answer away rather than
// merely stop computing on read what the view already computed. A database this
// migration meets might hold one.
func refuseIfARuleCellIsTheOnlyAnswer(tx *sql.Tx) error {
	rows, err := tx.Query(`
		SELECT col.column_key, COUNT(*)
		FROM cells c
		JOIN columns col ON col.id = c.column_id
		WHERE c.source = 'rule'
		  AND NOT EXISTS (
			SELECT 1 FROM cells sibling
			WHERE sibling.column_id = c.column_id
			  AND sibling.model_name = c.model_name
			  AND sibling.source <> 'rule'
		  )
		GROUP BY col.column_key
		ORDER BY col.column_key`)
	if err != nil {
		return fmt.Errorf("count rule cells with no other source on record: %w", err)
	}
	defer rows.Close()
	var orphaned []ruleColumnCount
	for rows.Next() {
		var r ruleColumnCount
		if err := rows.Scan(&r.columnKey, &r.count); err != nil {
			return err
		}
		orphaned = append(orphaned, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(orphaned) == 0 {
		return nil
	}
	msg := ""
	for i, r := range orphaned {
		if i > 0 {
			msg += ", "
		}
		msg += fmt.Sprintf("%s (%d)", r.columnKey, r.count)
	}
	return fmt.Errorf("refusing to delete rule cells: %d column(s) have a rule cell with no other source on record for the same model, which deleting would leave with no answer at all: %s",
		len(orphaned), msg)
}

// countRuleCellsPerColumn is what theDeadRuleCellsAreDeleted logs after the
// delete, read before it so the count is not thrown off by the DELETE itself.
func countRuleCellsPerColumn(tx *sql.Tx) ([]ruleColumnCount, error) {
	rows, err := tx.Query(`
		SELECT col.column_key, COUNT(*)
		FROM cells c
		JOIN columns col ON col.id = c.column_id
		WHERE c.source = 'rule'
		GROUP BY col.column_key
		ORDER BY col.column_key`)
	if err != nil {
		return nil, fmt.Errorf("count rule cells per column: %w", err)
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
