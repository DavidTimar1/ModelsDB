package catalog

import (
	"reflect"
	"testing"
)

func TestNormalizeModelFullModel(t *testing.T) {
	in := map[string]interface{}{
		"name":            "OpenAI: GPT-5",
		"description":     "a model",
		"id":              "openai/gpt-5",
		"canonical_slug":  "openai/gpt-5-2026",
		"hugging_face_id": "openai/gpt-5",
		"created":         float64(1700000000),
		"context_length":  float64(200000),
		"architecture": map[string]interface{}{
			"input_modalities":  []interface{}{"text", "image"},
			"output_modalities": []interface{}{"text"},
		},
		"supported_parameters": []interface{}{"tools", "reasoning"},
		"pricing": map[string]interface{}{
			"prompt":     "0.000001",
			"completion": "0.000002",
		},
	}
	out := NormalizeModel(in)

	if got := out["name"]; got != "OpenAI: GPT-5" {
		t.Errorf("name = %v", got)
	}
	if got := out["slug"]; got != "openai/gpt-5-2026" {
		t.Errorf("slug should come from canonical_slug, got %v", got)
	}
	if got := out["hf_slug"]; got != "openai/gpt-5" {
		t.Errorf("hf_slug = %v", got)
	}
	if got := out["created_at"]; got != "2023-11-14T22:13:20Z" {
		t.Errorf("created_at = %v, want 2023-11-14T22:13:20Z", got)
	}
	if got := out["supports_reasoning"]; got != true {
		t.Errorf("supports_reasoning = %v, want true", got)
	}
	if got := out["hidden"]; got != false {
		t.Errorf("hidden = %v, want false", got)
	}
	ep, ok := out["endpoint"].(map[string]interface{})
	if !ok {
		t.Fatalf("endpoint missing or wrong type: %T", out["endpoint"])
	}
	if ep["supports_tool_parameters"] != true {
		t.Errorf("supports_tool_parameters = %v, want true (from 'tools')", ep["supports_tool_parameters"])
	}
	if !reflect.DeepEqual(ep["pricing"], in["pricing"]) {
		t.Errorf("pricing not carried into endpoint: %v", ep["pricing"])
	}
	if !reflect.DeepEqual(out["input_modalities"], in["architecture"].(map[string]interface{})["input_modalities"]) {
		t.Errorf("input_modalities not lifted to top level: %v", out["input_modalities"])
	}
}

func TestNormalizeModelFallbacksAndAbsent(t *testing.T) {
	// No canonical_slug -> slug falls back to id; no hugging_face_id -> hf_slug nil;
	// no supported_parameters -> both capability flags false; no created -> no created_at.
	in := map[string]interface{}{
		"name": "Some Model",
		"id":   "vendor/some-model",
	}
	out := NormalizeModel(in)

	if out["slug"] != "vendor/some-model" {
		t.Errorf("slug should fall back to id, got %v", out["slug"])
	}
	if out["hf_slug"] != nil {
		t.Errorf("hf_slug should be nil when hugging_face_id absent, got %v", out["hf_slug"])
	}
	if _, ok := out["created_at"]; ok {
		t.Errorf("created_at should be absent when 'created' missing")
	}
	if out["supports_reasoning"] != false {
		t.Errorf("supports_reasoning should be false")
	}
	ep := out["endpoint"].(map[string]interface{})
	if ep["supports_tool_parameters"] != false {
		t.Errorf("supports_tool_parameters should be false")
	}
	if _, ok := ep["pricing"]; ok {
		t.Errorf("endpoint.pricing should be absent when no pricing on input")
	}
}

func TestNormalizeModelToolChoiceAndIncludeReasoning(t *testing.T) {
	// The alternate parameter spellings also flip the flags.
	out := NormalizeModel(map[string]interface{}{
		"name":                 "x",
		"id":                   "x",
		"supported_parameters": []interface{}{"tool_choice", "include_reasoning"},
	})
	if out["supports_reasoning"] != true {
		t.Errorf("include_reasoning should set supports_reasoning")
	}
	if out["endpoint"].(map[string]interface{})["supports_tool_parameters"] != true {
		t.Errorf("tool_choice should set supports_tool_parameters")
	}
}
