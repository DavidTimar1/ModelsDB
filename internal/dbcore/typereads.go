package dbcore

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
)

// This file is migration v37: derived.model_type - the Type column - reads
// what OpenRouter and a repository state about a model, not only the reader's
// own answer and what a collection page says.
//
// WHY A MIGRATION IS THE ONLY WAY TO REACH AN EXISTING DATABASE.
// store.SeedShippedColumns is additive: a recipe already stored is a recipe the
// reader may have changed on purpose, so seeding never corrects one. Changing
// store.shippedColumns therefore reaches a fresh install only. An install
// carried forward keeps reading two of the four columns that state this fact
// until something rewrites the stored recipe - which is what this migration
// does, and ONLY where the stored recipe is still the untouched original.
//
// WHAT IT TOUCHES. It reads derived_inputs for the derived.model_type column.
// Where that list is EXACTLY [mine.model_type, collection.model_type] in that
// order, it replaces it with [mine.model_type, collection.model_type,
// openrouter.model_type, huggingface.model_type] and recomputes the column for
// every model that has a value in any of those five columns. Any other stored
// list - one input added, removed or reordered - is the reader's own choice
// and is left completely untouched.
func theTypeColumnReadsEverySourceThatStatesIt(tx *sql.Tx) error {
	held, err := tableExists(tx, "derived_columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, so there is no recipe to repair
	}

	derivedID, ok, err := columnIDByKey(tx, "derived.model_type")
	if err != nil || !ok {
		return err // no such column: nothing shipped it yet, nothing to repair
	}
	mineID, ok, err := columnIDByKey(tx, "mine.model_type")
	if err != nil || !ok {
		return err
	}
	collectionID, ok, err := columnIDByKey(tx, "collection.model_type")
	if err != nil || !ok {
		return err
	}
	openrouterID, ok, err := columnIDByKey(tx, "openrouter.model_type")
	if err != nil || !ok {
		return err
	}
	huggingfaceID, ok, err := columnIDByKey(tx, "huggingface.model_type")
	if err != nil || !ok {
		return err
	}

	current, err := orderedDerivedInputs(tx, derivedID)
	if err != nil {
		return err
	}
	if len(current) != 2 || current[0] != mineID || current[1] != collectionID {
		return nil // not the untouched original recipe: it is the reader's now
	}

	if _, err := tx.Exec("DELETE FROM derived_inputs WHERE column_id=?", derivedID); err != nil {
		return fmt.Errorf("clear the old %s inputs: %w", "derived.model_type", err)
	}
	newInputs := []int64{mineID, collectionID, openrouterID, huggingfaceID}
	for position, inputID := range newInputs {
		if _, err := tx.Exec("INSERT INTO derived_inputs (column_id, position, input_column_id) VALUES (?,?,?)",
			derivedID, position, inputID); err != nil {
			return fmt.Errorf("add %s as an input: %w", "derived.model_type", err)
		}
	}

	stamp := NowStamp()
	if _, err := tx.Exec("UPDATE derived_columns SET rebuilt_at=? WHERE column_id=?", stamp, derivedID); err != nil {
		return fmt.Errorf("stamp the rebuild: %w", err)
	}

	n, err := recomputeModelType(tx, derivedID, newInputs, stamp)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("Migration v37: recomputed the Type column for %d model(s)", n)
	}
	return nil
}

// columnIDByKey answers one column's id, and whether it exists at all - a
// column this migration expects but does not find is not an error, because a
// stranger's database could in principle be missing one; the migration then
// simply has nothing to repair.
func columnIDByKey(tx *sql.Tx, key string) (int64, bool, error) {
	var id int64
	err := tx.QueryRow("SELECT id FROM columns WHERE column_key=?", key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// orderedDerivedInputs answers one computed column's inputs, in the order they
// are read.
func orderedDerivedInputs(tx *sql.Tx, derivedID int64) ([]int64, error) {
	rows, err := tx.Query("SELECT input_column_id FROM derived_inputs WHERE column_id=? ORDER BY position ASC", derivedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// recomputeModelType works the derived.model_type value out again for every
// model that holds a cell in it or in any of its inputs, taking the first
// input in order that has a value - the same rule as the first_non_empty
// transform this column's recipe names, said in SQL because dbcore sits below
// store and cannot call it.
func recomputeModelType(tx *sql.Tx, derivedID int64, inputIDs []int64, stamp string) (int, error) {
	names, err := modelsHoldingAnyOf(tx, append([]int64{derivedID}, inputIDs...))
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, name := range names {
		value, err := firstNonEmptyCell(tx, name, inputIDs)
		if err != nil {
			return 0, err
		}
		was, err := oneCellText(tx, derivedID, name)
		if err != nil {
			return 0, err
		}
		if was == value {
			continue
		}
		if err := putOrClearTextCell(tx, derivedID, name, value, stamp); err != nil {
			return 0, err
		}
		changed++
	}
	return changed, nil
}

// modelsHoldingAnyOf names every model with a value in any of those columns.
func modelsHoldingAnyOf(tx *sql.Tx, columnIDs []int64) ([]string, error) {
	places := make([]string, len(columnIDs))
	args := make([]interface{}, len(columnIDs))
	for i, id := range columnIDs {
		places[i], args[i] = "?", id
	}
	rows, err := tx.Query("SELECT DISTINCT model_name FROM cells WHERE column_id IN ("+
		joinPlaces(places)+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func joinPlaces(places []string) string {
	out := ""
	for i, p := range places {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// oneCellText reads one model's stored text for one column, answering empty
// when there is no cell - the same answer as "nobody has said".
func oneCellText(tx *sql.Tx, columnID int64, modelName string) (string, error) {
	var value string
	err := tx.QueryRow("SELECT value_text FROM cells WHERE column_id=? AND model_name=?", columnID, modelName).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// firstNonEmptyCell answers the first of those columns, in order, that this
// model holds a value in.
func firstNonEmptyCell(tx *sql.Tx, modelName string, columnIDs []int64) (string, error) {
	for _, id := range columnIDs {
		value, err := oneCellText(tx, id, modelName)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return "", nil
}

// putOrClearTextCell writes one model's value for a TEXT column, or clears the
// cell when the value is empty. derived.model_type is text, so it carries no
// sort key: text sorts by its own letters rather than by a stored number.
func putOrClearTextCell(tx *sql.Tx, columnID int64, modelName, value, stamp string) error {
	if value == "" {
		_, err := tx.Exec("DELETE FROM cells WHERE column_id=? AND model_name=?", columnID, modelName)
		return err
	}
	_, err := tx.Exec(`INSERT INTO cells (column_id, model_name, value_text, value_number, written_at)
		VALUES (?,?,?,NULL,?)
		ON CONFLICT(column_id, model_name) DO UPDATE SET
		  value_text=excluded.value_text, value_number=NULL, written_at=excluded.written_at`,
		columnID, modelName, value, stamp)
	return err
}
