package catalog

import (
	"modelsdb/internal/dbcore"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestApiUrlReadsEveryOutputModality proves the feed URL is not the bare
// /models route, which OpenRouter filters to output_modalities=text by
// default and so would hide every embeddings, image, video, transcription,
// speech, rerank and decisions model from the refresh.
func TestApiUrlReadsEveryOutputModality(t *testing.T) {
	if !strings.Contains(ApiUrl, "output_modalities=all") {
		t.Fatalf("ApiUrl = %q, want it to carry output_modalities=all", ApiUrl)
	}
}

// TestFetchFeedRefusesAnOversizedBody proves the feed fetch errors rather than
// silently truncating and decoding a feed larger than feedMaxBodyBytes.
func TestFetchFeedRefusesAnOversizedBody(t *testing.T) {
	oversized := `{"data":[{"name":"` + strings.Repeat("x", feedMaxBodyBytes+1) + `"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(oversized))
	}))
	t.Cleanup(server.Close)

	origAPI := ApiUrl
	ApiUrl = server.URL
	t.Cleanup(func() { ApiUrl = origAPI })

	_, _, err := fetchFeedFrom(ApiUrl, nil)
	if err == nil {
		t.Fatal("expected an error for an oversized feed, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected the error to say the feed was too large, got: %v", err)
	}
}

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

// TestOpenRouterFactsReadWhatNoColumnHolds pins the facts the catalog fetch
// records as custom columns: the author out of the id, the two architecture
// texts, the parameter list, the provider's limits, the dates, the reasoning
// answer, the alias and the prices no column of models holds.
func TestOpenRouterFactsReadWhatNoColumnHolds(t *testing.T) {
	facts := OpenRouterFacts(map[string]interface{}{
		"name":             "OpenAI: GPT-5",
		"id":               "openai/gpt-5",
		"knowledge_cutoff": "2026-02-16",
		"expiration_date":  "2098-12-31",
		"architecture": map[string]interface{}{
			"tokenizer":     "GPT",
			"instruct_type": "chatml",
		},
		"supported_parameters": []interface{}{"tools", "temperature"},
		"top_provider": map[string]interface{}{
			"max_completion_tokens": float64(128000),
			"is_moderated":          true,
		},
		"reasoning":    map[string]interface{}{"mandatory": false},
		"alias_target": map[string]interface{}{"name": "OpenAI: GPT-5 0731", "slug": "openai/gpt-5-0731"},
		"pricing": map[string]interface{}{
			"prompt":            "0.000001",
			"input_cache_read":  "0.0000001",
			"input_cache_write": "0.0000002",
			"web_search":        "0.01",
			"image_output":      "0",
		},
	})

	want := map[string]string{
		"author":                "openai",
		"tokenizer":             "GPT",
		"instruct_type":         "chatml",
		"supported_parameters":  "tools, temperature",
		"max_completion_tokens": "128000",
		"moderated":             "yes",
		"knowledge_cutoff":      "2026-02-16",
		"expiration_date":       "2098-12-31",
		"reasoning_mandatory":   "no",
		"alias_target":          "openai/gpt-5-0731",
		"price_cache_read":      "0.0000001",
		"price_cache_write":     "0.0000002",
		"price_web_search":      "0.01",
	}
	for key, expected := range want {
		if facts[key] != expected {
			t.Errorf("%s = %q, want %q", key, facts[key], expected)
		}
	}
	// A price quoted as zero is a price nobody is charged, and a column of zeros
	// says nothing, so it is left out rather than recorded.
	if _, there := facts["price_image_output"]; there {
		t.Errorf("a zero price was recorded: %q", facts["price_image_output"])
	}
	// The five prices models holds as columns of its own are NOT facts, or the
	// two copies would disagree the first time one was corrected.
	for _, key := range []string{"price_prompt", "prompt", "price_completion"} {
		if _, there := facts[key]; there {
			t.Errorf("%s was recorded as a fact, and it is a column of models", key)
		}
	}
}

// TestOpenRouterFactsLeaveOutWhatUpstreamDoesNotState keeps an empty column from
// being created: a record that states nothing must offer nothing, rather than
// offering blanks.
func TestOpenRouterFactsLeaveOutWhatUpstreamDoesNotState(t *testing.T) {
	facts := OpenRouterFacts(map[string]interface{}{
		"name": "Some Model",
		"id":   "vendor/some-model",
		"architecture": map[string]interface{}{
			"tokenizer":     "",
			"instruct_type": nil,
		},
		"top_provider": map[string]interface{}{"max_completion_tokens": nil, "is_moderated": nil},
		"pricing":      map[string]interface{}{"prompt": "0.000001"},
	})
	if facts["author"] != "vendor" {
		t.Errorf("author = %q, want \"vendor\"", facts["author"])
	}
	delete(facts, "author")
	if len(facts) != 0 {
		t.Errorf("a record stating nothing produced %v", facts)
	}
}

// TestOpenRouterFactsWithoutAnAuthorInTheID covers the record whose id carries
// no owner at all, which must produce no author rather than an empty one.
func TestOpenRouterFactsWithoutAnAuthorInTheID(t *testing.T) {
	facts := OpenRouterFacts(map[string]interface{}{"name": "x", "id": "bare-id"})
	if _, there := facts["author"]; there {
		t.Errorf("an id with no owner produced author=%q", facts["author"])
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

// TestFetchModelsFailsWhenTheHeldNamesCannotBeRead proves the refresh stops
// with an error when it cannot read the names the database already holds. An
// empty answer there would make every feed record look new and report the
// whole feed as added, so the failure must reach the caller instead.
func TestFetchModelsFailsWhenTheHeldNamesCannotBeRead(t *testing.T) {
	catalogTestDB(t)
	dedicatedListingServer(t,
		`{"data":[{"name":"Acme: One","canonical_slug":"acme/one","id":"acme/one"}]}`,
		`{"data":[]}`,
		`{"data":[]}`,
	)
	if _, err := dbcore.DB.Exec("DROP TABLE models"); err != nil {
		t.Fatalf("drop the models table to make reading the held names fail: %v", err)
	}

	created, _, _, err := FetchModels()
	if err == nil {
		t.Fatalf("FetchModels returned no error, created = %d; want the held-names failure", created)
	}
	if !strings.Contains(err.Error(), "read the names already held") {
		t.Errorf("error = %q, want it to name the held-names read", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 when the refresh could not start", created)
	}
}
