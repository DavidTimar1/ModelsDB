package app

import (
	"modelsdb/internal/store"
	"strings"
	"testing"
)

// The helpers here are how a test says "a column of the reader's own" now that
// there are no column NUMBERS anywhere: a column is named by its KEY, which is
// what the API answers with, what a filter names, and what a write sends.
//
// A COLUMN OF THE READER'S OWN IS ONE THAT IS NOT SEALED. The app ships every
// column it defines as sealed (CreateColumn's caller decides that; see
// shipped.go), and CreateOwnColumn always creates an unsealed one with hand as
// its one declared source - there is no separate "mine" namespace left to key
// off, because a column's key carries no namespace at all any more.

// ownColumnKey makes sure a column with that heading exists and answers its key.
// A heading that is already there is answered as it is, so a test may call it as
// often as it likes.
func ownColumnKey(t *testing.T, label, valueType string, options []string) (string, error) {
	t.Helper()
	columns, err := store.ListColumns()
	if err != nil {
		return "", err
	}
	for _, c := range columns {
		// Only a column of the READER'S own answers here: a shipped column may
		// carry the same heading, and a write meant for a fresh column of the
		// reader's own must not land on one already declared for other sources.
		if c.Sealed {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(c.Label), strings.TrimSpace(label)) {
			return c.Key, nil
		}
	}
	column, err := store.CreateOwnColumn(store.ColumnSpec{
		Label: label, ValueType: valueType, Options: options,
	})
	if err != nil {
		return "", err
	}
	return column.Key, nil
}

// setOwnValue writes one value into a column of the reader's own, exactly as
// POST /api/save does.
func setOwnValue(model, key, value string) error {
	err := store.SaveCells(model, map[string]string{key: value})
	return err
}

// ownValueOf reads back what one column holds for one model, and whether it
// holds anything at all. The store already resolves which source wins a
// column - a hand answer over every fetched one - so this is nothing more
// than reading the winning cell straight off ModelCells.
func ownValueOf(model, key string) (string, bool, error) {
	cells, err := store.ModelCells(model)
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(cells[key])
	return value, value != "", nil
}

// ownColumnsList is every column the reader owns, in the order they were made.
func ownColumnsList() ([]store.Column, error) {
	columns, err := store.ListColumns()
	if err != nil {
		return nil, err
	}
	var out []store.Column
	for _, c := range columns {
		if !c.Sealed {
			out = append(out, c)
		}
	}
	return out, nil
}

// ownValuesOf is every value one model holds in a column of the reader's own,
// keyed by that column's key.
func ownValuesOf(model string) (map[string]string, error) {
	columns, err := ownColumnsList()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, c := range columns {
		if v, held, err := ownValueOf(model, c.Key); err != nil {
			return nil, err
		} else if held {
			out[c.Key] = v
		}
	}
	return out, nil
}
