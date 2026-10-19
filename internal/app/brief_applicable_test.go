package app

import (
	"fmt"
	"modelsdb/internal/store"
	"net/http"
	"strings"
	"testing"
)

// askedFields is the set of field names a brief answer offers.
func askedFields(t *testing.T, name string) (map[string]map[string]interface{}, map[string]interface{}) {
	t.Helper()
	body := briefFor(t, name)
	asked := map[string]map[string]interface{}{}
	for _, g := range gapsOf(t, body) {
		asked[fmt.Sprint(g["field"])] = g
	}
	return asked, body
}

// TestTheBriefOffersOnlyWhatCanApplyToTheRow: an embedding model that only
// exists as a repository is not asked for tools, reasoning, an OpenRouter price
// list or reference images; a listed chat model is asked for its listing's
// fields; and everything offered has a description.
func TestTheBriefOffersOnlyWhatCanApplyToTheRow(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Repo: Embedder","cells":{"model_type":"embedding","context_length":"8192"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	asked, body := askedFields(t, "Repo: Embedder")
	for _, gone := range []string{"supports_tools", "supports_reasoning", "price_cache_read", "zdr", "tokenizer", "reference_images", "default_parameters_temperature", "build_files", "gguf_architecture", "state_size", "top_provider_context_length"} {
		if asked[gone] != nil {
			t.Errorf("an embedding repository is asked for %s", gone)
		}
	}
	for _, want := range []string{"knowledge_cutoff", "parameters", "active_parameters", "description"} {
		if asked[want] == nil {
			t.Errorf("an embedding repository is not asked for %s", want)
		}
	}
	for field, g := range asked {
		if strings.TrimSpace(fmt.Sprint(g["about"])) == "" {
			t.Errorf("%s is offered with no description", field)
		}
	}
	if !strings.Contains(fmt.Sprint(asked["knowledge_cutoff"]["accepts"]), "YYYY-MM-DD") {
		t.Errorf("knowledge_cutoff accepts %q, want the date form named", asked["knowledge_cutoff"]["accepts"])
	}
	text := fmt.Sprint(body["brief"])
	for _, want := range []string{"WHAT THIS ROW IS. Kind of model: embedding", "Listed on OpenRouter: no", "WHAT THIS ROW HOLDS NOW", "context_length", "base model's card counts as the source", "report it in your answer"} {
		if !strings.Contains(text, want) {
			t.Errorf("the brief never says %q", want)
		}
	}

	if rec := addModel(t, `{"name":"Cat: Listed Chat","cells":{"model_type":"chat","id":"cat/listed-chat"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	listed, _ := askedFields(t, "Cat: Listed Chat")
	for _, want := range []string{"supports_tools", "tokenizer", "price_cache_read", "zdr"} {
		if listed[want] == nil {
			t.Errorf("a listed chat model is not asked for %s", want)
		}
	}
	for _, gone := range []string{"reference_images", "supported_voices", "build_files"} {
		if listed[gone] != nil {
			t.Errorf("a listed chat model is asked for %s", gone)
		}
	}
}

// TestKeptNamesAreColumnKeys: an import's "kept" list names the columns it wrote,
// under the keys a read answers and a save takes, not the fact keys it read them
// under.
func TestKeptNamesAreColumnKeys(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	rec := importModel(t, `{"url":"`+testImportRepo+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}
	kept, _ := importedBody(t, rec)["kept"].([]interface{})
	if len(kept) == 0 {
		t.Fatal("the import kept nothing")
	}
	named := map[string]bool{}
	for _, k := range kept {
		key := fmt.Sprint(k)
		named[key] = true
		if _, err := store.ColumnByKey(key); err != nil {
			t.Errorf("kept names %q, which is not a column key", key)
		}
	}
	for _, want := range []string{"repo_owner", "weights_size_gb"} {
		if !named[want] {
			t.Errorf("kept does not name the column %s", want)
		}
	}
	for _, gone := range []string{"author"} {
		if named[gone] {
			t.Errorf("kept still names the fact key %s", gone)
		}
	}
}
