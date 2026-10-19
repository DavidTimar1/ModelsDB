// The rows GET /api/models answers with.
//
// A ROW IS IDENTITY PLUS CELLS. The model row itself carries only identity - the
// name every value hangs off, the route that created it, and the reader's own
// decisions about their table (hidden, and which row sits under which). Every
// FACT about the model is a cell, and a cell is answered under its column's key
// with the TEXT the database holds, so what a read gives back is what a write
// took, with no second lookup and no translation on the way out.
//
// IT RETURNS ONE ROW PER STORED MODEL, ALWAYS. It applies no rule about which
// models deserve to be listed - no name is excluded here, and no row is skipped
// for what it holds. A row this function withheld would be a row the API cannot
// return, POST /api/save answers 404 for, and the page cannot draw: it would
// exist, and be unreachable by every verb this app has. Narrowing a caller's
// answer belongs to filterAndProject, which reads that request's own parameters.

package app

import (
	"encoding/json"
	"modelsdb/internal/store"
)

func processModels() ([]map[string]interface{}, error) {
	identities, err := store.ListModelIdentities()
	if err != nil {
		return nil, err
	}

	// One read of the whole cell store and one of every source's verbatim record,
	// instead of two queries per row. A model with neither gets an empty map,
	// never a missing key, so nothing reading the answer has to guard against a
	// null.
	cells, err := store.AllCells()
	if err != nil {
		return nil, err
	}
	records, err := store.AllSourceRecords()
	if err != nil {
		return nil, err
	}
	answeredBy, disputed, overtaken, err := store.AllProvenance()
	if err != nil {
		return nil, err
	}

	// EVERY STORED ROW IS EMITTED. Which rows a given CALLER receives is decided
	// afterwards, by filterAndProject, from that request's own parameters.
	merged := make([]map[string]interface{}, 0, len(identities))
	for _, identity := range identities {
		name := identity.Name

		var raw map[string]interface{}
		if rj := store.RecordOf(records[name], identity.Source); rj != "" {
			_ = json.Unmarshal([]byte(rj), &raw)
		}

		merged = append(merged, map[string]interface{}{
			"name":   name,
			"source": identity.Source,

			// Hidden: the row is kept here in full, and left out of what is shown.
			// The flag is emitted on every row; deciding which rows a CALLER
			// receives is filterAndProject's job, because that is where the
			// request's own include_hidden parameter is read.
			"hidden": identity.Hidden,

			// A VARIATION of another model - one quantized build, recorded as a row
			// of its own. variant_of names the model it was built from and is empty
			// on every ordinary row, so it is what tells the two kinds of row apart.
			"variant_of": identity.VariantOf,

			// Every fact, under its column's key, as the text the database holds.
			"cells": cellsOrEmpty(cells[name]),

			// Which source's answer won each column - "hand" in a column of the
			// reader's own, a source's own name otherwise, "merged" for a
			// union-resolve column an automatic source contributed to.
			"answered_by": answeredByOrEmpty(answeredBy[name]),

			// Which columns have OpenRouter and HuggingFace disagreeing on this
			// model's value. Nothing settles it; the column's order decides.
			"disputed": disputedOrEmpty(disputed[name]),

			// Which columns hold a researched value an automatic source has since
			// overtaken: still stored, no longer shown, and the one research cell
			// the page offers to clear from a read-only field.
			"overtaken": disputedOrEmpty(overtaken[name]),

			"_raw": raw,
		})
	}

	return merged, nil
}

// cellsOrEmpty normalizes a model with no stored value at all to an empty (but
// non-nil) map, so every row carries a cells object rather than a null.
func cellsOrEmpty(held map[string]string) map[string]string {
	if held == nil {
		return map[string]string{}
	}
	return held
}

// answeredByOrEmpty applies the same never-null rule as cellsOrEmpty to the
// answered_by map: a model with no cell at all still carries an empty object.
func answeredByOrEmpty(held map[string]string) map[string]string {
	if held == nil {
		return map[string]string{}
	}
	return held
}

// disputedOrEmpty applies the same never-null rule as cellsOrEmpty to the
// disputed map.
func disputedOrEmpty(held map[string]bool) map[string]bool {
	if held == nil {
		return map[string]bool{}
	}
	return held
}
