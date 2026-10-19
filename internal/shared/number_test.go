package shared

import "testing"

func TestParseNumber(t *testing.T) {
	good := map[string]float64{
		"0": 0, "7": 7, "-3": -3, "+2.5": 2.5, "0.000002": 0.000002, ".5": 0.5, "5.": 5, "1e3": 1000, "2.5E-2": 0.025,
	}
	for text, want := range good {
		got, err := ParseNumber(text)
		if err != nil || got != want {
			t.Errorf("ParseNumber(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
	for _, text := range []string{
		"", " ", "NaN", "nan", "Inf", "+Inf", "-inf", "Infinity", "1e999", "-1e999", "0x1p-2", "0x10", "1_000",
		"1,5", " 5", "5 ", "abc", "5kg", "--5", "e5", ".", "+", "١٢",
	} {
		if got, err := ParseNumber(text); err == nil {
			t.Errorf("ParseNumber(%q) = %v, want an error", text, got)
		}
	}
}
