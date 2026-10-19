// Which columns are stored at two decimal places.

package store

import (
	"modelsdb/internal/shared"
	"strings"
)

// twoDecimalStems are the columns whose values are rounded to two decimal places
// before they are stored, named by key - so the rule holds whoever wrote the
// value and whatever the column is currently labelled.
//
// All three are MEASURED figures in a large unit - billions of parameters, and
// gigabytes of weights - where the digits past the second answer nothing a person
// asks. Rounding them on the way in means a value read out of a column and
// written back into it is the value that was already there, rather than a longer
// number that only looks different.
//
// "Build size (bytes)" is deliberately NOT here. It is an exact byte count the
// variation importer measured, and only its GB rendering in the browser is
// rounded; the count itself is kept as it was read.
var twoDecimalStems = map[string]bool{
	"parameters":        true,
	"active_parameters": true,
	"weights_size_gb":   true,
}

// roundedForStorage is the value a column really stores: two decimal places for
// the three measured figures above, and the value untouched for every other
// column. A value that does not read as a number is given back as it arrived.
//
// BOTH WRITE PATHS REACH IT - what a person sends to POST /api/save or
// POST /api/models/add, and what a source states through recordSourceFacts - so
// the rule is applied once rather than being repeated per caller, and a figure
// read out of a column and written back into it is the figure that was there.
func roundedForStorage(columnKey, value string) string {
	if !twoDecimalStems[strings.ToLower(strings.TrimSpace(columnKey))] {
		return value
	}
	return shared.TwoDecimals(value)
}
