package shared

import "testing"

// TestTwoDecimalsRoundsWithoutLosingASmallFigure declares every value it checks,
// so it reads nothing out of the database and no legitimate change to stored data
// can break it.
//
// Three rules that matter to a reader of the table are pinned here: a whole
// number reads as a whole number (no trailing ".0"), a trailing zero is dropped
// so 33.10 stores as 33.1, and a figure below one keeps BOTH its decimals - a
// 350-million-parameter model reads 0.35, which is the figure one decimal place
// could not hold.
func TestTwoDecimalsRoundsWithoutLosingASmallFigure(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"32.762123264", "32.76"},
		{"65.52", "65.52"},
		{"33.10", "33.1"},
		{"2.00", "2"},
		{"3.3", "3.3"},
		{"0.35", "0.35"},
		{"0.6", "0.6"},
		{"0.62", "0.62"},
		{"0.004", "0"},
		{"32.0", "32"},
		{"32", "32"},
		{"14.496080928", "14.5"},
		{"17666268832", "17666268832"},
		{" 7.248023552 ", "7.25"},
	} {
		if got := TwoDecimals(c.in); got != c.want {
			t.Errorf("TwoDecimals(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestTwoDecimalsRoundTripsAValueItAlreadyStored is what makes the rule usable
// on a column somebody edits: reading a stored figure and writing it back must
// store the same digits, or every save would look like a change.
func TestTwoDecimalsRoundTripsAValueItAlreadyStored(t *testing.T) {
	for _, stored := range []string{"0.35", "3.3", "32", "32.76", "65.52", "0.01"} {
		if got := TwoDecimals(stored); got != stored {
			t.Errorf("TwoDecimals(%q) = %q, want it unchanged", stored, got)
		}
	}
}

// TestTwoDecimalsLeavesTextItCannotReadAlone is the other half: this rule is
// applied to whatever a column holds, and a column can hold something a person
// typed. Anything that is not a number comes back byte for byte.
func TestTwoDecimalsLeavesTextItCannotReadAlone(t *testing.T) {
	for _, in := range []string{"", " ", "about 30", "n/a", "-", "1,024", "32B"} {
		if got := TwoDecimals(in); got != in {
			t.Errorf("TwoDecimals(%q) = %q, want it untouched", in, got)
		}
	}
}
