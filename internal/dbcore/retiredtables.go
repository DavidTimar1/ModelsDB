// This file is migration v50: drops the three tables the floor schema creates and
// earlier migrations (v41, v44) drop, in the files where a build that applied the
// floor schema at every open created them again, empty.

package dbcore

import (
	"database/sql"
	"fmt"
)

// theRetiredTablesAreDropped removes ignored_models, derived_inputs and
// derived_columns, with their indexes, where they exist. Nothing reads or writes
// them: v41 and v44 already dropped what they held, so a table found here is one
// an open created empty. A table that does hold rows is refused rather than
// discarded.
func theRetiredTablesAreDropped(tx *sql.Tx) error {
	for _, table := range []string{"derived_inputs", "derived_columns", "ignored_models"} {
		held, err := tableExists(tx, table)
		if err != nil {
			return err
		}
		if !held {
			continue
		}
		var rows int
		if err := tx.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&rows); err != nil {
			return fmt.Errorf("count the rows of %s: %w", table, err)
		}
		if rows > 0 {
			return fmt.Errorf("%s holds %d row(s) and nothing reads it; refusing to drop it", table, rows)
		}
	}
	for _, drop := range []string{
		"DROP INDEX IF EXISTS idx_derived_inputs_source",
		"DROP TABLE IF EXISTS derived_inputs",
		"DROP TABLE IF EXISTS derived_columns",
		"DROP INDEX IF EXISTS idx_ignored_models_name",
		"DROP TABLE IF EXISTS ignored_models",
	} {
		if _, err := tx.Exec(drop); err != nil {
			return err
		}
	}
	return nil
}
