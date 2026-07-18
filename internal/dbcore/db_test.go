package dbcore

import (
	"reflect"
	"testing"
)

func TestSplitList(t *testing.T) {
	if got := SplitList("a, b ,,c "); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("splitList trims and drops empties, got %v", got)
	}
	if got := SplitList(""); len(got) != 0 {
		t.Errorf("splitList('') should be empty, got %v", got)
	}
}

func TestToJSONArr(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, ""},
		{"", ""},
		{`["a"]`, `["a"]`},    // already JSON -> passthrough
		{"a, b", `["a","b"]`}, // comma list -> JSON array
		{[]string{"x", "y"}, `["x","y"]`},
		{[]interface{}{"a"}, `["a"]`},
	}
	for _, c := range cases {
		if got := ToJSONArr(c.in); got != c.want {
			t.Errorf("toJSONArr(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseArr(t *testing.T) {
	if got := ParseArr(`["a","b"]`); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("parseArr(json) = %v", got)
	}
	if got := ParseArr("a, b"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("parseArr(csv fallback) = %v", got)
	}
	if got := ParseArr(""); got != nil {
		t.Errorf("parseArr('') = %v, want nil", got)
	}
}

func TestMergeArr(t *testing.T) {
	if got := MergeArr([]string{"text"}, []string{"image", "text"}); !reflect.DeepEqual(got, []string{"text", "image"}) {
		t.Errorf("mergeArr union/order = %v", got)
	}
	if got := MergeArr(nil, []string{"a", "a", " b "}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("mergeArr dedupe/trim = %v", got)
	}
}

func TestNullInt(t *testing.T) {
	if got := NullInt(float64(3)); got != int64(3) {
		t.Errorf("nullInt(float64) = %#v", got)
	}
	if got := NullInt(5); got != int64(5) {
		t.Errorf("nullInt(int) = %#v", got)
	}
	if got := NullInt(nil); got != nil {
		t.Errorf("nullInt(nil) = %#v, want nil", got)
	}
	if got := NullInt("x"); got != nil {
		t.Errorf("nullInt(string) = %#v, want nil", got)
	}
}
