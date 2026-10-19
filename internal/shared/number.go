package shared

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// decimalNumberRe is the whole of what a number cell may look like: plain
// decimal digits with an optional sign, fraction and exponent.
var decimalNumberRe = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// ParseNumber reads text as a finite decimal number. strconv.ParseFloat alone is
// too generous for a stored figure: it accepts NaN, Inf and hexadecimal floats,
// and a NaN binds to SQL as NULL, so a cell would sort as unanswered while the
// page showed a value. Anything that is not plain decimal, or that overflows to
// infinity, is an error.
func ParseNumber(text string) (float64, error) {
	if !decimalNumberRe.MatchString(text) {
		return 0, fmt.Errorf("%q is not a decimal number", text)
	}
	figure, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(figure) || math.IsInf(figure, 0) {
		return 0, fmt.Errorf("%q is not a finite number", text)
	}
	return figure, nil
}
