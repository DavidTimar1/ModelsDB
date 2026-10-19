// Taking a model out of the table, or off the default view.
//
// Two verbs: DeleteModelPermanently removes the row and everything attached to
// it, and SetHidden keeps the row but leaves it off the default list. Neither
// verb keeps a record of what it removed, and a refresh stores every record the
// OpenRouter feed carries (see catalog.FetchModels) - so a deleted OpenRouter
// model would simply be created again. DeleteModelPermanently therefore refuses a
// row whose models.source is openrouter, and the route points the reader at
// hiding instead.

package store

import (
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
)

// ErrListedByOpenRouter is the refusal to delete a row OpenRouter created: the
// next refresh would create it again, so hiding is the way it leaves the table.
var ErrListedByOpenRouter = errors.New("OpenRouter created this row, so a refresh would add it straight back")

// DeleteModelPermanently removes a model row and everything attached to it. A row
// whose source is openrouter is refused with ErrListedByOpenRouter and a missing
// one with ErrNoSuchModel; the test is that one word exactly, because
// models.source is free text with a legal empty state and every other value it
// takes names a row no refresh creates.
//
// THIS DESTROYS PERSONAL DATA THAT EXISTS NOWHERE ELSE: the model's note, its
// favorite flag, and its value in every column of the reader's own, which cascade
// away with the row (cells.model_name REFERENCES models(name) ON DELETE CASCADE).
// The only way back is a copy of modelsdb.db.
func DeleteModelPermanently(name string) error {
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	res, err := dbcore.DB.Exec("DELETE FROM models WHERE name=? AND source<>?", name, string(SourceOpenRouter))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		if _, held, err := IdentityOf(name); err == nil && held {
			return fmt.Errorf("%w: %q", ErrListedByOpenRouter, name)
		}
		return fmt.Errorf("%w: %q", ErrNoSuchModel, name)
	}
	return nil
}

// SetHidden hides a model from the default view, or shows it again.
//
// This is the non-destructive alternative to DeleteModelPermanently: the row and
// everything attached to it - the note, the favorite flag, every value in a
// column of the reader's own - stay exactly where they are. Only who sees it changes. A hidden model
// is left out of GET /api/models unless the caller asks for it with
// include_hidden=1, and the browser table draws it only while the filter bar's
// Hidden dropdown is set to show hidden models - either alongside the rest or on
// their own - where it carries a crossed-out-eye mark that clears the flag again.
func SetHidden(name string, hidden bool) error {
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	flag := 0
	if hidden {
		flag = 1
	}
	res, err := dbcore.DB.Exec("UPDATE models SET hidden=?, updated_at=? WHERE name=?", flag, dbcore.NowStamp(), name)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %q", ErrNoSuchModel, name)
	}
	return nil
}
