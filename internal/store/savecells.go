// What a WRITE over the network may put in a cell.
//
// A REQUEST NAMES COLUMNS BY THEIR BARE KEY, exactly as a read answers them, so
// a value read out of GET /api/models can be posted straight back with nothing
// looked up in between. The request does not say who is writing: ONE RULE
// decides per key, from the column and from what is on file for that model
// (sourceForOwnWrite), so a person in the page and an assistant over the API
// are held to the same rule and neither can reach a value an automatic source
// holds.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"strings"
)

// SaveCells writes the cells one request carries onto a model that already
// exists.
//
// IT IS ONE TRANSACTION. Every key is resolved before anything is written, so
// a refused key leaves every stored value exactly as it was - which matters
// because an empty value CLEARS a cell, so a value that fails quietly does not
// fail, it deletes.
//
// IT CREATES NO MODEL. A name this database has never seen answers ErrNoSuchModel
// and writes nothing, so a mistyped name fails instead of minting a row carrying
// whatever was sent. AddModelWithCells is how a model is added.
func SaveCells(name string, cells map[string]string) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoSuchModel
	}
	if _, exists, err := IdentityOf(name); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("%w: %q", ErrNoSuchModel, name)
	}
	return writeOwnCells(name, cells)
}

// writeOwnCells is the shared half of a save and an add: round the three
// measured figures and write the lot, each key as the source the rule gives
// it, in one transaction.
func writeOwnCells(name string, cells map[string]string) error {
	if err := ensureShippedColumns(); err != nil {
		return err
	}
	cells = roundedCells(cells)
	keys := sortedKeys(cells)

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := modelMustExist(tx, name); err != nil {
		return err
	}
	stamp := dbcore.NowStamp()
	for _, key := range keys {
		column, err := columnByKey(tx, key)
		if err != nil {
			return unknownKey(key, err)
		}
		src, err := sourceForOwnWrite(tx, column, name, cells[key])
		if err != nil {
			return err
		}
		if err := writeOneCell(tx, column, src, name, cells[key], stamp); err != nil {
			return err
		}
	}
	// A request carrying no cell at all is still a write, so the row records
	// that it was written - which is what every other path stamps too.
	if err := stampModelWritten(tx, name, stamp); err != nil {
		return err
	}
	return tx.Commit()
}

// sourceForOwnWrite is THE RULE for a write that arrives over the network,
// decided per key: a column of the reader's own takes hand; a column research
// may fill takes research, unless an automatic source already holds the
// answer for this model, in which case the write is refused naming that
// source and its value, because nothing typed replaces what an update
// maintains; a column neither may reach is refused naming the source that
// does state it.
//
// AN EMPTY VALUE IS ALWAYS ACCEPTED where research is declared, automatic
// answer or not: it deletes the research cell and nothing else, so clearing
// a researched value an automatic source has since overtaken destroys nothing
// and tidies what the answers panel shows as no longer shown.
func sourceForOwnWrite(tx *sql.Tx, column Column, modelName, value string) (Source, error) {
	sources, err := declaredSources(tx, column.ID)
	if err != nil {
		return "", err
	}
	if containsSource(sources, SourceHand) {
		return SourceHand, nil
	}
	if !containsSource(sources, SourceResearch) {
		names := make([]string, 0, len(sources))
		for _, s := range sources {
			names = append(names, string(s))
		}
		return "", fmt.Errorf("%w: %s is stated only by %s, and nothing typed reaches it", ErrNotWritable, column.Key, quotedList(names))
	}
	if strings.TrimSpace(value) == "" {
		return SourceResearch, nil
	}
	held, err := automaticAnswer(tx, column.ID, modelName)
	if err != nil {
		return "", err
	}
	if held.source != "" {
		return "", fmt.Errorf("%w: %s holds %s's answer, %s, which an update maintains; nothing typed replaces it",
			ErrNotWritable, column.Label, displayWord(held.source), held.value)
	}
	return SourceResearch, nil
}

// automaticAnswer is the winning automatic cell on file for one column and
// model, or an empty one where no automatic source has stated it. It is read
// inside the write's own transaction, so the refusal and the write see the
// same cells.
func automaticAnswer(tx *sql.Tx, columnID int64, modelName string) (storedCell, error) {
	rows, err := tx.Query(`SELECT c.source, c.value_text FROM cells c
		JOIN column_sources cs ON cs.column_id = c.column_id AND cs.source = c.source
		WHERE c.column_id=? AND c.model_name=? ORDER BY cs.rank ASC`, columnID, modelName)
	if err != nil {
		return storedCell{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var c storedCell
		if err := rows.Scan(&c.source, &c.value); err != nil {
			return storedCell{}, err
		}
		if Source(c.source).Kind() == "fetched" {
			return c, nil
		}
	}
	return storedCell{}, rows.Err()
}

// displayWord is the word a refusal names a source by: what the page calls
// it, not the stored name.
func displayWord(source string) string {
	switch Source(source) {
	case SourceOpenRouter:
		return "OpenRouter"
	case SourceHuggingFace:
		return "HuggingFace"
	case SourceResearch:
		return "Research"
	}
	return source
}

// containsSource reports whether the list names src.
func containsSource(list []Source, src Source) bool {
	for _, s := range list {
		if s == src {
			return true
		}
	}
	return false
}

// unknownKey words the refusal of a key naming no column: making a column is
// the reader's own act, so the refusal says where.
func unknownKey(key string, err error) error {
	if errors.Is(err, ErrNoSuchColumn) {
		return fmt.Errorf("%w: %q. A column of your own is made with POST /api/columns, and nothing makes one on the way past a write", ErrNoSuchColumn, key)
	}
	return err
}

// roundedCells applies the two-decimal rule to the three measured figures, read
// off the key each value is going into. Every other value is stored exactly as
// it was sent.
func roundedCells(cells map[string]string) map[string]string {
	out := make(map[string]string, len(cells))
	for key, value := range cells {
		out[key] = roundedForStorage(key, value)
	}
	return out
}

// everyValueItsColumnWillHold refuses a value the column it is going into cannot
// take, a column nothing typed may reach, and a key naming no column, BEFORE
// anything is inserted. It resolves every key by the same rule a write does,
// against a model that has no cells yet, so on a new row every fetched
// column's value lands as research and every column of the reader's own as
// hand.
//
// It exists for the add: the row and its cells are two statements, so a check
// made only inside the cell writer would answer 400 having already created a row
// under that name.
func everyValueItsColumnWillHold(name string, cells map[string]string) error {
	keys := sortedKeys(cells)
	if len(keys) == 0 {
		return nil
	}
	tx, err := dbcore.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, key := range keys {
		column, err := columnByKey(tx, key)
		if err != nil {
			return unknownKey(key, err)
		}
		if _, err := sourceForOwnWrite(tx, column, name, cells[key]); err != nil {
			return err
		}
		value := strings.TrimSpace(roundedForStorage(key, cells[key]))
		if value == "" {
			continue
		}
		if _, err := sortKeyFor(column, value); err != nil {
			return err
		}
	}
	return nil
}

// isUniqueViolation reports whether err is SQLite refusing a duplicate key.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// AddModelWithCells creates the row for a model somebody is adding by hand and
// writes the cells the same request carries. The row is source "manual", which is
// what says nothing upstream re-creates it, and the name is stored as
// NormalizeModelName gives it.
//
// THE CELLS ARE CHECKED BEFORE THE ROW IS INSERTED. The insert and the write are
// two statements, so a check made only on the way to the cells would answer 400
// having already created an empty row under that name.
//
// A MODEL THIS CATALOG ALREADY HOLDS answers ErrModelExists and changes nothing.
// It is recognised by FindExistingModel rather than by its name alone, so typing
// a new display name over the repository or the upstream id of a model already
// here creates no second row for it. A model deleted earlier is not "already
// here": it has no row, so it is added again like any other name.
func AddModelWithCells(name string, cells map[string]string) error {
	name = NormalizeModelName(name)
	if hasNoName(name) {
		return ErrNoModelName
	}
	if err := ensureShippedColumns(); err != nil {
		return err
	}
	if err := everyValueItsColumnWillHold(name, cells); err != nil {
		return err
	}
	match, held, err := FindExistingModel(name, cells["id"], cells["repository"])
	if err != nil {
		return err
	}
	if held {
		if match.By == MatchByName {
			return fmt.Errorf("%w, as %q", ErrModelExists, match.Name)
		}
		return fmt.Errorf("%w, as %q, which carries %s", ErrModelExists, match.Name, match.Matched())
	}
	if _, err := dbcore.DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES (?,?,?)`,
		name, "manual", dbcore.NowStamp()); err != nil {
		// Two requests adding one name at once both pass the check above; the
		// primary key lets only one through, and the other is told the model exists.
		if isUniqueViolation(err) {
			return fmt.Errorf("%w, as %q", ErrModelExists, name)
		}
		return err
	}
	if err := StampAdded(name); err != nil {
		return err
	}
	return writeOwnCells(name, cells)
}
