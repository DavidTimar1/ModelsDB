package shared

import "testing"

func TestGetStr(t *testing.T) {
	m := map[string]any{"a": "x", "n": 1}
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
