// The columns the READER owns: the ones they make themselves,
// and what may be done to them.
//
// A COLUMN OF THEIRS IS A COLUMN LIKE ANY OTHER: a row of `columns` this app
// did not ship, with its labels in `column_options` and its values in cells
// stamped SourceHand, with no side table and no second identity - the
// column's KEY is what it is, its label is what it is called, and relabelling
// it changes only the label. UNSEALED IS WHAT MAKES IT THEIRS: every column
// the app ships is sealed, so a column that is not sealed was made by the
// reader, whatever it is called.
//
// WHAT IS THEIRS TO DO. They may create a column, relabel one, and delete one -
// which permanently deletes every model's value for it. A column the app SHIPS
// can be relabelled and hidden and nothing else, because every source that
// states it binds through its key and its kind.
//
// A column of theirs and its values are ALWAYS personal-only: they live in
// modelsdb.db and are never written to a file, so they are never shared across
// installs - the same guarantee the note and the favourite star carry.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"strings"
)

// ownColumnPosition is where a column the reader makes sits in the layout: after
// every column the app ships, in the order they were created.
const ownColumnPosition = 1000

// CreateOwnColumn adds a column of the reader's own and, for a choice column,
// its ordered option list. It is stated by hand and by nothing else.
//
// THE KEY IS WORKED OUT FROM THE LABEL where the request does not give one, and
// a heading the app KNOWS takes the key its fact is stated under: "Parameters
// (B)" is the column "parameters", the same one HuggingFace states - so this
// is refused as already existing, and the reader types their own figure into
// the column that is already there rather than getting a second one beside it.
//
// A LABEL ANY OTHER COLUMN ALREADY CARRIES IS REFUSED, ignoring capitals,
// because a label names a column table-wide (the unique index on
// columns.label enforces this in the database) and a person choosing a column
// by its heading could not otherwise tell two of them apart.
func CreateOwnColumn(spec ColumnSpec) (Column, error) {
	spec.Label = strings.TrimSpace(spec.Label)
	spec.Key = strings.TrimSpace(spec.Key)
	if spec.Label == "" {
		return Column{}, fmt.Errorf("a column needs a label, which is the heading it is shown under")
	}
	if spec.Key == "" {
		spec.Key = columnKeyForColumnName(spec.Label)
	}
	if spec.ValueType == "" {
		spec.ValueType = ValueTypeText
	}
	if held, err := columnExists(spec.Key); err != nil {
		return Column{}, err
	} else if held {
		return Column{}, fmt.Errorf("%s already exists, so it was not created again. PATCH /api/columns relabels it, and a hand write answers it directly", spec.Key)
	}
	if taken, held, err := columnByLabel(spec.Label); err != nil {
		return Column{}, err
	} else if held {
		return Column{}, fmt.Errorf("%q is already the label of %s, and one label cannot name two columns", spec.Label, taken.Key)
	}
	spec.Position, spec.Sealed = ownColumnPosition, false
	return CreateColumn(spec, SourceHand)
}

// RelabelColumn changes what a column is CALLED and nothing else. Every stored
// value follows it, because a value names its column by key and the key does not
// move - which is the whole reason the two are separate.
//
// A label another column already holds is refused, matched without regard to
// capitals, for the same reason a new column's is.
func RelabelColumn(key, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Errorf("a column needs a label, which is the heading it is shown under")
	}
	column, err := ColumnByKey(key)
	if err != nil {
		return err
	}
	if taken, held, err := columnByLabel(label); err != nil {
		return err
	} else if held && taken.ID != column.ID {
		return fmt.Errorf("%q is already the label of %s, and one label cannot name two columns", label, taken.Key)
	}
	_, err = dbcore.DB.Exec("UPDATE columns SET label=? WHERE id=?", label, column.ID)
	return err
}

// columnByLabel answers the column carrying that heading, ignoring capitals:
// "Author" and "author" are one heading, not two, table-wide - the same
// uniqueness columns.label's unique index (COLLATE NOCASE) enforces in the
// database.
func columnByLabel(label string) (Column, bool, error) {
	var key string
	err := dbcore.DB.QueryRow("SELECT column_key FROM columns WHERE label = ? COLLATE NOCASE", label).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return Column{}, false, nil
	}
	if err != nil {
		return Column{}, false, err
	}
	c, err := ColumnByKey(key)
	return c, true, err
}
