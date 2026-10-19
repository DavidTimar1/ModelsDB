package store

import (
	"errors"
	"testing"
)

// A column cannot carry a word a request already reads as a parameter or as a
// part of the row, because it could never be filtered by.
func TestNoColumnMayBeCalledAWordTheQueryAlreadyUses(t *testing.T) {
	seedInto(t)
	for _, key := range []string{"name", "include_hidden", "hide_free", "links", "fields", "source", "hidden", "variant_of", "_raw"} {
		if !IsReservedColumnKey(key) {
			t.Errorf("%q is not reserved", key)
		}
		_, err := CreateColumn(ColumnSpec{Key: key, Label: "L " + key, ValueType: ValueTypeText}, SourceHand)
		if !errors.Is(err, ErrColumnKey) {
			t.Errorf("CreateColumn(%q) err = %v, want ErrColumnKey", key, err)
		}
	}
	// "Name" as a label works out to the key name, so the reader's own column is
	// refused the same way.
	if _, err := CreateOwnColumn(ColumnSpec{Label: "Name"}); !errors.Is(err, ErrColumnKey) {
		t.Errorf("CreateOwnColumn(Name) err = %v, want ErrColumnKey", err)
	}
	if _, err := CreateOwnColumn(ColumnSpec{Label: "Ordinary column"}); err != nil {
		t.Errorf("an ordinary column was refused: %v", err)
	}
}
