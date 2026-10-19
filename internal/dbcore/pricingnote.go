package dbcore

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
)

// This file is migration v51: the Pricing note column (pricing_note) is gone.
// The reader's Priced per column says what a price is measured in, and a
// caveat about a price is a line in that model's Note.
//
// A shipped column is sealed, so the page cannot delete it; only a migration
// can. What a reader typed into it is theirs and cannot be recreated, so the
// column is not simply dropped: every non-blank pricing note is first carried
// into that model's hand Note cell, and only then is the column deleted.
//
// WHAT IT TOUCHES.
//   - cells on the notes column (source hand): a model with no Note gets a new
//     cell holding the pricing note; a model whose Note is blank has it
//     replaced by the pricing note; a model whose Note already reads the same
//     text is left alone; any other Note keeps its text and gains the pricing
//     note after a newline.
//   - columns: the pricing_note row is deleted; DELETE FROM columns cascades
//     to its column_sources, column_options and cells.
//
// A stored view that still names pricing_note in its column order, hidden list
// or filters needs no rewrite: the page and the query route already pass over
// a key that matches no column.
//
// cellsMayFall IS SET, because deleting the pricing_note cells is the whole
// purpose. The migration's own exact refusal stands in for the chain's guard:
// the cell count must fall by exactly the pricing_note cells less the Note
// cells created, and the Note cell count must rise by exactly the number
// created, or the pre-migration snapshot comes back.
func thePricingNoteColumnIsRetired(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to retire
	}

	var pricingID int64
	err = tx.QueryRow(`SELECT id FROM columns WHERE column_key='pricing_note'`).Scan(&pricingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // this catalog never had the column
	}
	if err != nil {
		return fmt.Errorf("find the pricing_note column: %w", err)
	}

	notes, err := pricingNoteCarriers(tx, pricingID)
	if err != nil {
		return err
	}

	var pricingCells, cellsBefore int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=?`, pricingID).Scan(&pricingCells); err != nil {
		return fmt.Errorf("count the pricing_note cells: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells`).Scan(&cellsBefore); err != nil {
		return fmt.Errorf("count the cells before retiring pricing_note: %w", err)
	}

	var notesID int64
	var notesBefore int
	if len(notes) > 0 {
		if err := tx.QueryRow(`SELECT id FROM columns WHERE column_key='notes'`).Scan(&notesID); err != nil {
			return fmt.Errorf("find the notes column to carry %d pricing note(s) into: %w", len(notes), err)
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=?`, notesID).Scan(&notesBefore); err != nil {
			return fmt.Errorf("count the note cells before carrying pricing notes: %w", err)
		}
	}

	created, appended, unchanged := 0, 0, 0
	for _, n := range notes {
		outcome, err := carryPricingNoteIntoNote(tx, notesID, n)
		if err != nil {
			return err
		}
		switch outcome {
		case noteCreated:
			created++
		case noteAppended:
			appended++
		case noteUnchanged:
			unchanged++
		}
	}

	if _, err := tx.Exec(`DELETE FROM columns WHERE id=?`, pricingID); err != nil {
		return fmt.Errorf("delete the pricing_note column: %w", err)
	}

	var cellsAfter, notesAfter int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells`).Scan(&cellsAfter); err != nil {
		return fmt.Errorf("count the cells after retiring pricing_note: %w", err)
	}
	if drop, want := cellsBefore-cellsAfter, pricingCells-created; drop != want {
		return fmt.Errorf("refusing to commit: retiring pricing_note should have dropped %d cell(s) (%d pricing note cell(s) less %d note cell(s) created), and %d went",
			want, pricingCells, created, drop)
	}
	if len(notes) > 0 {
		if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=?`, notesID).Scan(&notesAfter); err != nil {
			return fmt.Errorf("count the note cells after carrying pricing notes: %w", err)
		}
		if rise := notesAfter - notesBefore; rise != created {
			return fmt.Errorf("refusing to commit: the note cells should have risen by %d, and rose by %d", created, rise)
		}
	}

	log.Printf("Migration v51: deleted the pricing_note column and its %d cell(s); %d pricing note(s) carried into Note (%d new, %d appended, %d already there)",
		pricingCells, len(notes), created, appended, unchanged)
	return nil
}

// pricingNote is one model's non-blank pricing note.
type pricingNote struct {
	model string
	text  string
}

// pricingNoteCarriers reads every pricing note with text in it, in model
// order. Surrounding whitespace is not part of the text that is carried.
func pricingNoteCarriers(tx *sql.Tx, pricingID int64) ([]pricingNote, error) {
	rows, err := tx.Query(`SELECT model_name, TRIM(value_text) FROM cells
		WHERE column_id=? AND TRIM(COALESCE(value_text, '')) <> '' ORDER BY model_name`, pricingID)
	if err != nil {
		return nil, fmt.Errorf("read the pricing notes: %w", err)
	}
	defer rows.Close()
	var out []pricingNote
	for rows.Next() {
		var n pricingNote
		if err := rows.Scan(&n.model, &n.text); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

type noteOutcome int

const (
	noteCreated noteOutcome = iota
	noteAppended
	noteUnchanged
)

// carryPricingNoteIntoNote puts one pricing note into its model's hand Note
// cell and answers what it did to that cell.
func carryPricingNoteIntoNote(tx *sql.Tx, notesID int64, n pricingNote) (noteOutcome, error) {
	var existing sql.NullString
	err := tx.QueryRow(`SELECT value_text FROM cells WHERE column_id=? AND model_name=? AND source='hand'`,
		notesID, n.model).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
			VALUES (?,?,'hand',?,NULL,?)`, notesID, n.model, n.text, NowStamp()); err != nil {
			return 0, fmt.Errorf("carry the pricing note of %q into a new note: %w", n.model, err)
		}
		return noteCreated, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the note of %q: %w", n.model, err)
	}

	have := strings.TrimSpace(existing.String)
	if have == n.text {
		return noteUnchanged, nil
	}
	joined := n.text
	if have != "" {
		joined = existing.String + "\n" + n.text
	}
	if _, err := tx.Exec(`UPDATE cells SET value_text=?, value_number=NULL, written_at=?
		WHERE column_id=? AND model_name=? AND source='hand'`, joined, NowStamp(), notesID, n.model); err != nil {
		return 0, fmt.Errorf("carry the pricing note of %q into its note: %w", n.model, err)
	}
	return noteAppended, nil
}
