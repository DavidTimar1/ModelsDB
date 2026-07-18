package catalog

import "testing"

func TestExtractBalancedObject(t *testing.T) {
	// Braces and an unbalanced brace INSIDE a string must not confuse the scan.
	s := `junk "pricing": {"a":1,"b":{"c":2},"s":"}{"} trailing`
	obj, ok := extractBalancedObject(s, `"pricing":`)
	if !ok {
		t.Fatalf("expected to find the object")
	}
	if obj != `{"a":1,"b":{"c":2},"s":"}{"}` {
		t.Errorf("got %q", obj)
	}

	// An escaped quote inside the string value is honored.
	s2 := `"pricing":{"s":"a\"}b"}`
	obj2, ok2 := extractBalancedObject(s2, `"pricing":`)
	if !ok2 || obj2 != `{"s":"a\"}b"}` {
		t.Errorf("escape handling failed: %q ok=%v", obj2, ok2)
	}

	if _, ok := extractBalancedObject("no key here", `"pricing":`); ok {
		t.Errorf("expected not found")
	}
}

func TestFmtNum(t *testing.T) {
	tests := []struct {
		s    string
		mult float64
		want string
	}{
		{"0.5", 2, "1"},
		{"2", 0, "2"}, // mult 0 is treated as 1
		{"0.07", 1, "0.07"},
		{"notanumber", 1, "notanumber"}, // unparseable -> returned as-is
	}
	for _, tt := range tests {
		if got := fmtNum(tt.s, tt.mult); got != tt.want {
			t.Errorf("fmtNum(%q,%v) = %q, want %q", tt.s, tt.mult, got, tt.want)
		}
	}
}

func TestBuildDisplay(t *testing.T) {
	pp := pagePricing{DisplayPricing: []displayPrice{
		{SkuLabel: "Output Image", Price: "0.07", UnitLabel: "/megapixel", DisplayMultiplier: 1},
		{SkuLabel: "Empty", Price: "", UnitLabel: "/x"}, // skipped (no price)
	}}
	if got := buildDisplay(pp); got != "Output Image: $0.07/megapixel" {
		t.Errorf("buildDisplay = %q", got)
	}
}

func TestDeriveMeasurement(t *testing.T) {
	token := pagePricing{DisplayPricing: []displayPrice{{SkuLabel: "In", Price: "1", UnitLabel: "/1M tokens"}}}
	if got := deriveMeasurement(token); got != "per 1M tokens" {
		t.Errorf("token-priced measurement = %q, want 'per 1M tokens'", got)
	}

	mixed := pagePricing{DisplayPricing: []displayPrice{
		{SkuLabel: "Img", Price: "0.07", UnitLabel: "/megapixel"},
		{SkuLabel: "Vid", Price: "0.10", UnitLabel: "/second"},
		{SkuLabel: "Dup", Price: "0.20", UnitLabel: "/megapixel"}, // duplicate unit dropped
	}}
	if got := deriveMeasurement(mixed); got != "per megapixel; per second" {
		t.Errorf("measurement = %q", got)
	}

	if got := deriveMeasurement(pagePricing{}); got != "" {
		t.Errorf("empty pricing measurement = %q, want ''", got)
	}
}

func TestTokenPriced(t *testing.T) {
	allTokens := pagePricing{DisplayPricing: []displayPrice{
		{Price: "1", UnitLabel: "/1M tokens"},
		{Price: "2", UnitLabel: "/1M tokens"},
	}}
	if !tokenPriced(allTokens) {
		t.Errorf("all-token should be tokenPriced")
	}

	mixed := pagePricing{DisplayPricing: []displayPrice{
		{Price: "1", UnitLabel: "/1M tokens"},
		{Price: "2", UnitLabel: "/second"},
	}}
	if tokenPriced(mixed) {
		t.Errorf("a non-token unit should make tokenPriced false")
	}

	if tokenPriced(pagePricing{}) {
		t.Errorf("nothing priced -> tokenPriced false")
	}
}
