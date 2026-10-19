package app

import (
	"strings"
	"testing"

	"modelsdb/internal/store"
)

// aCatalogOfLinkedModels stores one model per combination of links: both, only
// an OpenRouter id, only a repository, neither, and one whose repository is
// stated by HuggingFace while a hidden twin carries the same. The hidden row is
// there because the empty count and the links filter both reach hidden rows.
func aCatalogOfLinkedModels(t *testing.T) {
	t.Helper()
	migratedDB(t)
	add := func(name string, src store.Source, values map[string]string) {
		t.Helper()
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil && !strings.Contains(err.Error(), "already") {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
		if len(values) == 0 {
			return
		}
		if err := store.WriteCells(src, name, values); err != nil {
			t.Fatalf("write %s cells on %q: %v", src, name, err)
		}
	}
	add("Both", store.SourceOpenRouter, map[string]string{"id": "a/both", "supports_tools": "yes"})
	add("Both", store.SourceHuggingFace, map[string]string{"repository": "a/both"})
	add("Router Only", store.SourceOpenRouter, map[string]string{"id": "b/router", "supports_tools": "no"})
	add("Repo Only", store.SourceHuggingFace, map[string]string{"repository": "c/repo"})
	add("Neither", store.SourceOpenRouter, map[string]string{"description": "no links at all"})
}

// TestValuesAnswerHowManyModelsHoldNothing pins the empty count: the models the
// filter any: returns, counted over the whole catalog and untouched by contains=.
func TestValuesAnswerHowManyModelsHoldNothing(t *testing.T) {
	aCatalogOfLinkedModels(t)

	if got := servedValues(t, "?key=supports_tools"); got.Empty != 2 {
		t.Errorf("supports_tools empty = %d, want 2 (Repo Only and Neither state no flag)", got.Empty)
	}
	if got := servedValues(t, "?key=supports_tools&contains=yes"); got.Empty != 2 {
		t.Errorf("empty = %d with contains=yes, want 2: a search word never narrows the empty row", got.Empty)
	}
	if got := servedValues(t, "?key=repository"); got.Empty != 2 {
		t.Errorf("repository empty = %d, want 2", got.Empty)
	}
	if got := servedValues(t, "?key=notes"); got.Empty != 4 {
		t.Errorf("notes empty = %d, want 4: no model holds a note", got.Empty)
	}
	if asked := queried(t, "?supports_tools=any:&include_hidden=1"); len(asked) != servedValues(t, "?key=supports_tools").Empty {
		t.Errorf("any: returned %v, which is not as many models as the empty count says", asked)
	}
}

// TestLinksKeepsTheModelsThatHaveOrLackALink pins every shape the Links filter
// sends: one included link, either, one excluded link, and the two ANDed.
func TestLinksKeepsTheModelsThatHaveOrLackALink(t *testing.T) {
	aCatalogOfLinkedModels(t)

	tests := []struct {
		about string
		query string
		want  []string
	}{
		{"HuggingFace included", "?links=any:huggingface", []string{"Both", "Repo Only"}},
		{"OpenRouter included", "?links=any:openrouter", []string{"Both", "Router Only"}},
		{"both included means either", "?links=any:huggingface,openrouter", []string{"Both", "Repo Only", "Router Only"}},
		{"capitals and spaces are ignored", "?links=any:%20HuggingFace%20,OPENROUTER", []string{"Both", "Repo Only", "Router Only"}},
		{"OpenRouter excluded", "?links=not:openrouter", []string{"Neither", "Repo Only"}},
		{"both excluded means neither", "?links=not:huggingface,openrouter", []string{"Neither"}},
		{"HuggingFace in and OpenRouter out", "?links=any:huggingface&links=not:openrouter", []string{"Repo Only"}},
		{"beside a column filter", "?links=any:openrouter&supports_tools=no", []string{"Router Only"}},
	}
	for _, tc := range tests {
		if got := queried(t, tc.query); !eqStr(got, tc.want) {
			t.Errorf("%s: %s served %v, want %v", tc.about, tc.query, got, tc.want)
		}
	}
}

// TestLinksRefusesAWordItDoesNotKnow pins the 400 and what it says it accepts.
func TestLinksRefusesAWordItDoesNotKnow(t *testing.T) {
	aCatalogOfLinkedModels(t)

	for _, query := range []string{
		"?links=huggingface",
		"?links=",
		"?links=any:",
		"?links=any:github",
		"?links=not:huggingface,github",
		"?links=any:huggingface&links=bogus",
	} {
		body := refused(t, query)
		for _, word := range []string{"links", "huggingface", "openrouter", "any:", "not:"} {
			if !strings.Contains(body, word) {
				t.Errorf("%s was refused without naming %q: %s", query, word, body)
			}
		}
	}
}

// TestNoColumnMayBeCalledLinks keeps the word the query reads before any column
// out of the columns a reader can make.
func TestNoColumnMayBeCalledLinks(t *testing.T) {
	if !store.IsReservedColumnKey(paramLinks) {
		t.Errorf("%q is read by the query as the Links filter but a column may be called that", paramLinks)
	}
}
