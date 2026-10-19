package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v49: the GGUF total column is relabelled.
//
// The column gguf_total_bytes held gguf.total, which a repository states as a
// count of PARAMETERS, under the label "GGUF total (bytes)" and the unit
// "bytes". Nothing writes it (the Parameters column holds that fact), and the
// shipped description and label say so, but a shipped column's label and unit are
// seeded once and never reapplied, so a database carried forward still shows the
// old wording over cells that hold parameter counts.
//
// WHAT IT TOUCHES. One row of columns: the label and the unit of the column
// keyed gguf_total_bytes, and only while the label is still exactly the one the
// app shipped. A label the reader changed is theirs and is left alone, and the
// unit is cleared with the label or not at all. No cell is read or written.
// The about text is the app's own and is reapplied at every start.

const (
	oldGGUFTotalLabel = "GGUF total (bytes)"
	newGGUFTotalLabel = "GGUF total (legacy)"
)

func theGGUFTotalColumnIsRelabelled(tx *sql.Tx) error {
	res, err := tx.Exec(`UPDATE columns SET label=?, unit='' WHERE column_key='gguf_total_bytes' AND label=?`,
		newGGUFTotalLabel, oldGGUFTotalLabel)
	if err != nil {
		return fmt.Errorf("relabel gguf_total_bytes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	log.Printf("Migration v49: relabelled %d column(s) named gguf_total_bytes", n)
	return nil
}
