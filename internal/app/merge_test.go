package app

import (
	"errors"
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"strings"
	"testing"
)

// upstreamSays runs one OpenRouter refresh record through the store, as the
// catalog fetch would. Each call is one source stating what it knows about the
// same model.
func upstreamSays(t *testing.T, record map[string]interface{}) {
	t.Helper()
	if _, err := store.UpsertAPIModel(catalog.NormalizeModel(record)); err != nil {
		t.Fatalf("UpsertAPIModel: %v", err)
	}
}

// TestContextLengthIsAnsweredByItsHighestRankedSource is the number half of
// the resolution rule, and it is the half that CHANGED under the namespace
// pivot: two sources describing the same model no longer merge into "the
// larger figure wins". Each source owns its own cell, and context_length's
// declared Sources rank OpenRouter above HuggingFace, so OpenRouter's own
// answer wins the column regardless of which figure is bigger or which
// arrived last.
func TestContextLengthIsAnsweredByItsHighestRankedSource(t *testing.T) {
	migratedDB(t)

	upstreamSays(t, map[string]interface{}{
		"name": "LiquidAI: LFM2-24B-A2B", "id": "liquid/lfm2-24b-a2b",
		"context_length": float64(32768),
	})
	if got := colInt(t, "LiquidAI: LFM2-24B-A2B", "context_length"); got != 32768 {
		t.Fatalf("context_length = %d after the first source, want 32768", got)
	}

	// A second source states a longer window - the model's own config says
	// 128,000 where the served endpoint says 32,768. OpenRouter outranks
	// HuggingFace for this column, so OpenRouter's own answer keeps winning.
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "LiquidAI: LFM2-24B-A2B", "context_length": float64(128000),
	}, "huggingface", "chat"); err != nil {
		t.Fatalf("ImportFullRecord: %v", err)
	}
	if got := colInt(t, "LiquidAI: LFM2-24B-A2B", "context_length"); got != 32768 {
		t.Errorf("context_length = %d, want OpenRouter's 32768 to still win - HuggingFace ranks below it", got)
	}
	// What HuggingFace itself stated is kept beside it, untouched, under its own
	// cell - readable through the same mechanism the answers route uses.
	if got := sourceStates(t, "LiquidAI: LFM2-24B-A2B", "context_length", "huggingface"); got != "128000" {
		t.Errorf("HuggingFace's own answer = %q, want its 128000 kept beside OpenRouter's", got)
	}

	// A model OpenRouter never described falls through to the next-ranked
	// source that has an answer at all.
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Repo: Only", "context_length": float64(65536),
	}, "huggingface", "chat"); err != nil {
		t.Fatalf("ImportFullRecord: %v", err)
	}
	if got := colInt(t, "Repo: Only", "context_length"); got != 65536 {
		t.Errorf("context_length = %d on a model OpenRouter never described, want HuggingFace's 65536", got)
	}
}

// TestACapabilityIsAnsweredByItsHighestRankedSource is the capability half.
// moe's declared Sources rank HuggingFace above OpenRouter, so a repository
// stating a capability wins the column even where OpenRouter's own cell,
// separately, says otherwise - the same per-source-rank rule context_length
// runs on, not a merge that remembers every sighting forever.
func TestACapabilityIsAnsweredByItsHighestRankedSource(t *testing.T) {
	migratedDB(t)

	upstreamSays(t, map[string]interface{}{
		"name": "LiquidAI: LFM2-24B-A2B", "id": "liquid/lfm2-24b-a2b",
		"supported_parameters": []interface{}{"tools", "reasoning"},
		"description":          "a mixture of experts model",
	})
	for _, c := range []struct{ column, want string }{
		{"tool", "yes"},
		{"moe", "yes"},
	} {
		if got := colStr(t, "LiquidAI: LFM2-24B-A2B", c.column); got != c.want {
			t.Fatalf("%s = %q after the source that saw it, want %q", c.column, got, c.want)
		}
	}
	if got := colInt(t, "LiquidAI: LFM2-24B-A2B", "supports_reasoning"); got != 1 {
		t.Fatalf("supports_reasoning = %d, want 1", got)
	}

	// THE SAME SOURCE, READ AGAIN, IS BELIEVED. Each source owns its own cell, so
	// a second reading of OpenRouter REPLACES the first - which is what makes a
	// capability upstream removes disappear instead of being carried for good.
	upstreamSays(t, map[string]interface{}{
		"name": "LiquidAI: LFM2-24B-A2B", "id": "liquid/lfm2-24b-a2b",
		"description": "a language model",
	})
	if got := colStr(t, "LiquidAI: LFM2-24B-A2B", "tool"); got != "no" {
		t.Errorf("tool = %q after the same source read it again and saw none, want no", got)
	}
	if got := colInt(t, "LiquidAI: LFM2-24B-A2B", "supports_reasoning"); got != 0 {
		t.Errorf("supports_reasoning = %d after the same source read it again, want 0", got)
	}

	// ACROSS SOURCES, RANK DECIDES: moe's Sources list HuggingFace before
	// OpenRouter, so the repository's own answer wins the column even though
	// OpenRouter's own cell, above, currently says no.
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "LiquidAI: LFM2-24B-A2B", "hf_slug": "LiquidAI/LFM2-24B-A2B", "moe": "yes",
	}, "huggingface", "chat"); err != nil {
		t.Fatalf("ImportFullRecord: %v", err)
	}
	if got := colStr(t, "LiquidAI: LFM2-24B-A2B", "moe"); got != "yes" {
		t.Errorf("moe = %q with one source describing a mixture of experts, want yes", got)
	}

	// AND NOTHING TYPED STANDS OVER EITHER: a capability OpenRouter states is
	// what an update maintains, so a save onto it is refused naming OpenRouter.
	err := store.SaveCells("LiquidAI: LFM2-24B-A2B", map[string]string{"supports_tools": "yes"})
	if !errors.Is(err, store.ErrNotWritable) || !strings.Contains(err.Error(), "OpenRouter's answer, no") {
		t.Errorf("typing over OpenRouter's capability: err = %v, want a refusal naming OpenRouter's no", err)
	}
	if got := colStr(t, "LiquidAI: LFM2-24B-A2B", "tool"); got != "no" {
		t.Errorf("tool = %q after the refused save, want OpenRouter's no untouched", got)
	}
}

// TestAnEmptyCapabilityStillTakesTheFirstAnswer keeps the rule above from
// meaning "nothing is ever written". "Nobody has said" is weaker than any
// finding, so the first source to state one fills the column.
func TestAnEmptyCapabilityStillTakesTheFirstAnswer(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("Manual: Only Here", map[string]string{}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if got := colStr(t, "Manual: Only Here", "tool"); got != "" {
		t.Fatalf("tool = %q on a row nobody has answered for, want empty", got)
	}
	upstreamSays(t, map[string]interface{}{"name": "Manual: Only Here", "id": "vendor/one"})
	if got := colStr(t, "Manual: Only Here", "tool"); got != "no" {
		t.Errorf("tool = %q after the first source to state one, want no", got)
	}
}

// TestAResearchedValueFillsAGapUntilTheRefreshStatesIt is the rule the two
// kinds of column rest on, seen from a save: a figure typed into a column the
// refresh has not filled shows as research, the refresh stating the fact is
// what shows afterwards, the researched figure stays beneath it unmoved, and a
// figure the refresh already states cannot be typed over at all.
func TestAResearchedValueFillsAGapUntilTheRefreshStatesIt(t *testing.T) {
	migratedDB(t)
	upstreamSays(t, map[string]interface{}{
		"name": "Vendor: Upstream", "id": "vendor/upstream",
		"supported_parameters": []interface{}{"tools"},
	})

	// OpenRouter stated no context length, so a person fills it in.
	if rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"context_length":"8192"}}]`); rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := colInt(t, "Vendor: Upstream", "context_length"); got != 8192 {
		t.Fatalf("context_length = %d after the research, want 8192", got)
	}
	rows := servedModels(t, "?include_hidden=1&name=Vendor")
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	answeredBy, _ := rows[0]["answered_by"].(map[string]interface{})
	if answeredBy["context_length"] != "research" {
		t.Errorf("answered_by.context_length = %v, want research", answeredBy["context_length"])
	}

	// A later edit is another researched answer, and it replaces the first.
	if rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"context_length":"4096"}}]`); rec.Code != http.StatusOK {
		t.Fatalf("second save status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := colInt(t, "Vendor: Upstream", "context_length"); got != 4096 {
		t.Errorf("context_length = %d after a second research write, want 4096", got)
	}

	// The refresh starts stating the fact. What it states is what shows, and
	// the researched figure is still beneath it.
	upstreamSays(t, map[string]interface{}{
		"name": "Vendor: Upstream", "id": "vendor/upstream",
		"context_length":       float64(128000),
		"supported_parameters": []interface{}{"tools"},
	})
	if got := colInt(t, "Vendor: Upstream", "context_length"); got != 128000 {
		t.Errorf("context_length = %d once OpenRouter states it, want 128000", got)
	}
	if got := sourceStates(t, "Vendor: Upstream", "context_length", "research"); got != "4096" {
		t.Errorf("the researched context_length = %q, want 4096 kept beneath OpenRouter's", got)
	}

	// The row says the researched value is overtaken, and the page draws a
	// Clear control from that.
	rows = servedModels(t, "?include_hidden=1&name=Vendor")
	overtaken, _ := rows[0]["overtaken"].(map[string]interface{})
	if overtaken["context_length"] != true {
		t.Errorf("overtaken.context_length = %v, want true once OpenRouter states over the research", overtaken["context_length"])
	}

	// And now it cannot be typed over: the refusal names OpenRouter and its value.
	rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"context_length":"8192"}}]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("typing over OpenRouter's figure answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Context length holds OpenRouter's answer, 128000, which an update maintains; nothing typed replaces it") {
		t.Errorf("the refusal does not say whose answer stands: %s", rec.Body.String())
	}
	if got := colInt(t, "Vendor: Upstream", "context_length"); got != 128000 {
		t.Errorf("context_length = %d after the refused save, want 128000", got)
	}

	// An empty value clears the overtaken research and nothing else.
	if rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"context_length":""}}]`); rec.Code != http.StatusOK {
		t.Fatalf("clearing the overtaken research answered %d: %s", rec.Code, rec.Body.String())
	}
	if got := sourceStates(t, "Vendor: Upstream", "context_length", "research"); got != "" {
		t.Errorf("the researched context_length = %q after the clear, want gone", got)
	}
	if got := colInt(t, "Vendor: Upstream", "context_length"); got != 128000 {
		t.Errorf("context_length = %d after the clear, want OpenRouter's 128000 untouched", got)
	}
}

// TestAPriceIsAssignedAndNeverMerged draws the line the merge rule stops at. A
// price is not a measurement that gets more accurate with a second opinion - it
// is what the model costs today, and the newest reading is the true one. Taking
// the higher of two prices would freeze a stale figure in place and quote a
// price nobody charges.
func TestAPriceIsAssignedAndNeverMerged(t *testing.T) {
	migratedDB(t)
	upstreamSays(t, map[string]interface{}{
		"name": "Vendor: Upstream", "id": "vendor/upstream",
		"pricing": map[string]interface{}{"prompt": "0.000009"},
	})
	upstreamSays(t, map[string]interface{}{
		"name": "Vendor: Upstream", "id": "vendor/upstream",
		"pricing": map[string]interface{}{"prompt": "0.000002"},
	})
	if got := colStr(t, "Vendor: Upstream", "price_prompt"); got != "0.000002" {
		t.Errorf("price_prompt = %q, want the newest figure 0.000002", got)
	}
}
