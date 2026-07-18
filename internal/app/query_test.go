package app

import (
	"math"
	"modelsdb/internal/shared"
	"net/url"
	"sort"
	"testing"
)

func TestToFloat(t *testing.T) {
	cases := []struct {
		in   interface{}
		want float64
		ok   bool
	}{
		{float64(3.5), 3.5, true},
		{int(2), 2, true},
		{int64(4), 4, true},
		{"1.5", 1.5, true},
		{"  2 ", 2, true}, // trimmed
		{"abc", 0, false},
		{true, 0, false},
	}
	for _, c := range cases {
		got, ok := toFloat(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("toFloat(%#v) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPerMillion(t *testing.T) {
	got, ok := perMillion("0.000002")
	if !ok || math.Abs(got-2.0) > 1e-9 {
		t.Errorf("perMillion(0.000002) = (%v,%v), want ~2", got, ok)
	}
	if _, ok := perMillion(""); ok {
		t.Errorf("perMillion('') should be !ok")
	}
}

func TestParseFloatDefault(t *testing.T) {
	if parseFloatDefault("", 7) != 7 {
		t.Errorf("empty should return default")
	}
	if parseFloatDefault("3.5", 7) != 3.5 {
		t.Errorf("valid should parse")
	}
	if parseFloatDefault("x", 7) != 7 {
		t.Errorf("invalid should return default")
	}
}

func sampleRows() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "Claude Opus", "model_type": "chat", "tool": "yes", "supports_reasoning": true,
			"zdr": true, "context_length": float64(200000), "prompt": "0.000003", "completion": "0.000015",
			"input_modalities": "text", "output_modalities": "text"},
		{"name": "Flux Image", "model_type": "image", "tool": "no", "zdr": false,
			"context_length": nil, "prompt": "", "completion": "",
			"input_modalities": "text", "output_modalities": "image"},
		{"name": "Gemini Flash", "model_type": "chat", "tool": "yes", "zdr": true,
			"context_length": float64(1000000), "prompt": "0.0000001", "completion": "0.0000004",
			"input_modalities": "text", "output_modalities": "text"},
	}
}

func names(rows []map[string]interface{}) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, shared.GetStr(r, "name"))
	}
	sort.Strings(out)
	return out
}

func eqStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFilterAndProject(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
		want  []string
	}{
		{"name substring", url.Values{"name": {"claude"}}, []string{"Claude Opus"}},
		{"type exact", url.Values{"type": {"image"}}, []string{"Flux Image"}},
		{"tool=yes", url.Values{"tool": {"yes"}}, []string{"Claude Opus", "Gemini Flash"}},
		{"zdr=true", url.Values{"zdr": {"true"}}, []string{"Claude Opus", "Gemini Flash"}},
		{"zdr=false", url.Values{"zdr": {"false"}}, []string{"Flux Image"}},
		{"min_context", url.Values{"min_context": {"500000"}}, []string{"Gemini Flash"}},
		{"max_in price", url.Values{"max_in": {"1"}}, []string{"Gemini Flash"}}, // Claude 3/1M excluded, Flux unpriced excluded
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(filterAndProject(sampleRows(), tt.query))
			if !eqStr(got, tt.want) {
				t.Errorf("filter %v -> %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestFilterAndProjectFields(t *testing.T) {
	out := filterAndProject(sampleRows(), url.Values{"name": {"claude"}, "fields": {"name,prompt"}})
	if len(out) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out))
	}
	row := out[0]
	if len(row) != 2 || row["name"] != "Claude Opus" || row["prompt"] != "0.000003" {
		t.Errorf("projection wrong: %v", row)
	}
	if _, ok := row["model_type"]; ok {
		t.Errorf("projected row should not carry unrequested fields")
	}
}
