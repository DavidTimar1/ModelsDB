package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v38: derived.moe - the MoE column - reads what
// OpenRouter states about a model too, not only the reader's own answer and
// the repository's.
//
// WHY A MIGRATION IS THE ONLY WAY TO REACH AN EXISTING DATABASE. See
// typereads.go's comment on theTypeColumnReadsEverySourceThatStatesIt: the
// same reasoning applies here. store.SeedShippedColumns never corrects a
// recipe that already exists, so an install carried forward keeps reading two
// of the three columns that state this fact until this migration rewrites the
// stored recipe - and ONLY where it is still the untouched original.
//
// WHAT IT TOUCHES. It reads derived_inputs for the derived.moe column. Where
// that list is EXACTLY [mine.moe, huggingface.moe] in that order, it replaces
// it with [mine.moe, huggingface.moe, openrouter.moe] and recomputes the
// column for every model that has a value in any of those four columns. Any
// other stored list - one input added, removed or reordered - is the
// reader's own choice and is left completely untouched.
func theMoeColumnReadsEverySourceThatStatesIt(tx *sql.Tx) error {
	held, err := tableExists(tx, "derived_columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, so there is no recipe to repair
	}

	derivedID, ok, err := columnIDByKey(tx, "derived.moe")
	if err != nil || !ok {
		return err // no such column: nothing shipped it yet, nothing to repair
	}
	mineID, ok, err := columnIDByKey(tx, "mine.moe")
	if err != nil || !ok {
		return err
	}
	huggingfaceID, ok, err := columnIDByKey(tx, "huggingface.moe")
	if err != nil || !ok {
		return err
	}
	openrouterID, ok, err := columnIDByKey(tx, "openrouter.moe")
	if err != nil || !ok {
		return err
	}

	current, err := orderedDerivedInputs(tx, derivedID)
	if err != nil {
		return err
	}
	if len(current) != 2 || current[0] != mineID || current[1] != huggingfaceID {
		return nil // not the untouched original recipe: it is the reader's now
	}

	if _, err := tx.Exec("DELETE FROM derived_inputs WHERE column_id=?", derivedID); err != nil {
		return fmt.Errorf("clear the old %s inputs: %w", "derived.moe", err)
	}
	newInputs := []int64{mineID, huggingfaceID, openrouterID}
	for position, inputID := range newInputs {
		if _, err := tx.Exec("INSERT INTO derived_inputs (column_id, position, input_column_id) VALUES (?,?,?)",
			derivedID, position, inputID); err != nil {
			return fmt.Errorf("add %s as an input: %w", "derived.moe", err)
		}
	}

	stamp := NowStamp()
	if _, err := tx.Exec("UPDATE derived_columns SET rebuilt_at=? WHERE column_id=?", stamp, derivedID); err != nil {
		return fmt.Errorf("stamp the rebuild: %w", err)
	}

	// recomputeModelType is typereads.go's helper, named for the column it was
	// written for; its logic is the same first_non_empty rule this column's
	// recipe names, so it is reused rather than duplicated.
	n, err := recomputeModelType(tx, derivedID, newInputs, stamp)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("Migration v38: recomputed the MoE column for %d model(s)", n)
	}
	return nil
}
