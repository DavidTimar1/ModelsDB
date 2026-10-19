// The one definition of "two decimal places".
//
// Three measured figures in this app carry far more precision than anybody
// reads: a parameter count in billions, the active count beside it, and the size
// of a model's weights in GB. They are STORED rounded rather than merely shown
// rounded, so a figure cannot re-enter the database at full precision by being
// read out of one column and written back into another.
//
// TWO DECIMALS RATHER THAN ONE, because the second digit is the difference
// between a figure and no figure at all for the small end of the catalog: a
// 350-million-parameter model is 0.35 billion, and at one decimal place it read
// as 0.4 - a number nothing states about it. The first digit alone is enough for
// a 32-billion model and not enough for a fraction of one, so the rule is set by
// the figures that need the most.
//
// The rule lives here, below both the database layer and the store layer, so the
// write path (internal/store/rounding.go) and the migration that rewrites the
// values already stored (internal/dbcore/migrate.go) apply the same definition
// rather than two that can drift apart.

package shared

import (
	"math"
	"strconv"
	"strings"
)

// TwoDecimals rounds a figure written as text to two decimal places and gives it
// back as the shortest digits that say the same number:
//
//	"32.762123264" -> "32.76"
//	"32.004"       -> "32"      (a whole number reads as a whole number)
//	"0.35"         -> "0.35"    (a figure below one keeps its decimals)
//	"33.10"        -> "33.1"    (a trailing zero says nothing)
//	"0.004"        -> "0"
//
// TEXT THAT IS NOT A NUMBER COMES BACK EXACTLY AS IT ARRIVED, and so does an
// empty string. This function is applied to whatever a column holds, and a column
// may hold something a person typed; a value it cannot read is a value it must
// not touch.
func TwoDecimals(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	n, err := ParseNumber(trimmed)
	if err != nil {
		return value
	}
	rounded := math.Round(n*100) / 100
	// A tiny negative figure rounds to negative zero, which prints as "-0".
	if rounded == 0 {
		rounded = 0
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}
