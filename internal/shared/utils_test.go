package shared

import "testing"

func TestGetStr(t *testing.T) {
	m := map[string]interface{}{"a": "x", "n": 1}
	if GetStr(m, "a") != "x" {
		t.Errorf("getStr present string")
	}
	if GetStr(m, "n") != "" {
		t.Errorf("getStr non-string should be ''")
	}
	if GetStr(m, "missing") != "" {
		t.Errorf("getStr absent should be ''")
	}
}

func TestGetBool(t *testing.T) {
	cases := []struct {
		v    interface{}
		want bool
	}{
		{true, true},
		{false, false},
		{float64(1), true},
		{float64(0), false},
		{int(2), true},
		{int64(0), false},
		{"true", true},
		{"1", true},
		{"false", false},
		{"no", false},
	}
	for _, c := range cases {
		if got := GetBool(map[string]interface{}{"k": c.v}, "k"); got != c.want {
			t.Errorf("getBool(%#v) = %v, want %v", c.v, got, c.want)
		}
	}
	if GetBool(map[string]interface{}{}, "missing") != false {
		t.Errorf("getBool absent should be false")
	}
}

func TestGetNum(t *testing.T) {
	if GetNum(map[string]interface{}{"k": float64(2.9)}, "k") != 2 {
		t.Errorf("getNum should truncate float to int")
	}
	if GetNum(map[string]interface{}{"k": int64(5)}, "k") != 5 {
		t.Errorf("getNum int64")
	}
	if GetNum(map[string]interface{}{}, "missing") != 0 {
		t.Errorf("getNum absent should be 0")
	}
}

func TestFavToInt(t *testing.T) {
	if favToInt(true) != 1 || favToInt(false) != 0 {
		t.Errorf("favToInt mapping wrong")
	}
}

func TestHasValAndHasValNum(t *testing.T) {
	if !hasVal(map[string]interface{}{"s": "x"}, "s") {
		t.Errorf("non-empty string should hasVal")
	}
	if hasVal(map[string]interface{}{"s": ""}, "s") {
		t.Errorf("empty string should not hasVal")
	}
	if !hasVal(map[string]interface{}{"b": true}, "b") {
		t.Errorf("true bool should hasVal")
	}
	if hasVal(map[string]interface{}{}, "missing") {
		t.Errorf("absent should not hasVal")
	}
	if !hasValNum(map[string]interface{}{"n": float64(3)}, "n") {
		t.Errorf("nonzero num should hasValNum")
	}
	if hasValNum(map[string]interface{}{"n": float64(0)}, "n") {
		t.Errorf("zero num should not hasValNum")
	}
}

func TestUpdateField(t *testing.T) {
	m := map[string]interface{}{"a": "old"}
	u := map[string]interface{}{"a": "new", "b": "ignored-key-not-in-m-is-still-applied"}
	changed := false
	updateField(m, u, "a", &changed)
	if m["a"] != "new" || !changed {
		t.Errorf("updateField should set changed and value, got %v changed=%v", m["a"], changed)
	}

	changed = false
	updateField(m, u, "a", &changed) // same value now -> no change
	if changed {
		t.Errorf("updateField with identical value should not flag changed")
	}

	changed = false
	updateField(m, u, "missing-in-u", &changed) // u lacks the field -> no-op
	if changed {
		t.Errorf("updateField with absent source field should be a no-op")
	}
}

func TestUpdateFieldNum(t *testing.T) {
	m := map[string]interface{}{"n": 1}
	u := map[string]interface{}{"n": float64(7)}
	changed := false
	updateFieldNum(m, u, "n", &changed)
	if m["n"] != 7 || !changed {
		t.Errorf("updateFieldNum should store float as int and flag changed, got %v changed=%v", m["n"], changed)
	}
}
