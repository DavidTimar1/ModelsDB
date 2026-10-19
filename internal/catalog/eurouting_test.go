package catalog

import (
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// euServer points EURoutingURL at a local server answering with the given
// status and body, and returns the User-Agent the last request carried.
func euServer(t *testing.T, status int, body string) *string {
	t.Helper()
	agent := new(string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*agent = r.UserAgent()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	orig := EURoutingURL
	EURoutingURL = server.URL
	t.Cleanup(func() { EURoutingURL = orig })
	return agent
}

// euRows seeds five OpenRouter rows (a to e) and one row from another route,
// each OpenRouter row already holding "yes", and returns the feed's id set.
func euRows(t *testing.T) map[string]bool {
	t.Helper()
	catalogTestDB(t)
	listed := map[string]bool{}
	for _, id := range []string{"v/a", "v/b", "v/c", "v/d", "v/e"} {
		holdRow(t, "Row "+id, "openrouter", id)
		store.RecordEURouting("Row "+id, true)
		listed[id] = true
	}
	holdRow(t, "Hand row", "collection", "v/a")
	return listed
}

func euValue(t *testing.T, name string) string {
	t.Helper()
	cells, err := store.AllCells()
	if err != nil {
		t.Fatal(err)
	}
	return cells[name]["eu_routing"]
}

// A good answer marks every OpenRouter row of the feed yes or no by its id,
// writes nothing to a row another route made, and leaves a row the feed no
// longer lists with its last value. The request carries the shared plain
// User-Agent.
func TestDeriveEURoutingMarksEveryFeedRowYesOrNo(t *testing.T) {
	listed := euRows(t)
	holdRow(t, "Dropped row", "openrouter", "v/dropped")
	store.RecordEURouting("Dropped row", true)
	agent := euServer(t, 200, `{"data":[{"id":"v/b"},{"id":"v/d"},{"id":""}]}`)

	DeriveEURouting(listed, nil)

	for id, want := range map[string]string{"v/a": "no", "v/b": "yes", "v/c": "no", "v/d": "yes", "v/e": "no"} {
		if got := euValue(t, "Row "+id); got != want {
			t.Errorf("eu_routing of %s = %q, want %q", id, got, want)
		}
	}
	if got := euValue(t, "Hand row"); got != "" {
		t.Errorf("a row another route made got eu_routing %q, want none", got)
	}
	if got := euValue(t, "Dropped row"); got != "yes" {
		t.Errorf("a row the feed dropped holds %q, want its last value yes", got)
	}
	if *agent != shared.BrowserUserAgent {
		t.Errorf("the EU request carried User-Agent %q, want the shared browser string", *agent)
	}
}

// Every way the answer can be wrong writes nothing, clears nothing, and does
// not panic or return an error the refresh could trip on.
func TestDeriveEURoutingWritesNothingOnABadAnswer(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"non-200", 500, `{"data":[{"id":"v/b"}]}`},
		{"not JSON", 200, `<html>`},
		{"no data list", 200, `{"error":{"message":"expected eu or us"}}`},
		{"empty list", 200, `{"data":[]}`},
		{"every id is blank", 200, `{"data":[{"id":""}]}`},
		{"list as large as the feed", 200, `{"data":[{"id":"v/a"},{"id":"v/b"},{"id":"v/c"},{"id":"v/d"},{"id":"v/e"}]}`},
		{"list of 90 percent of the feed", 200, `{"data":[{"id":"v/a"},{"id":"v/b"},{"id":"v/c"},{"id":"v/d"},{"id":"x/1"}]}`},
		{"list sharing no id with the feed", 200, `{"data":[{"id":"x/1"},{"id":"x/2"}]}`},
		{"oversized body", 200, `{"data":[{"id":"` + strings.Repeat("x", euMaxBodyBytes+1) + `"}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listed := euRows(t)
			euServer(t, c.status, c.body)
			DeriveEURouting(listed, nil)
			for id := range listed {
				if got := euValue(t, "Row "+id); got != "yes" {
					t.Errorf("eu_routing of %s = %q after a bad answer, want the old yes untouched", id, got)
				}
			}
		})
	}
}

// A server that cannot be reached writes nothing either.
func TestDeriveEURoutingWritesNothingWhenTheServerIsUnreachable(t *testing.T) {
	listed := euRows(t)
	server := httptest.NewServer(http.NotFoundHandler())
	orig := EURoutingURL
	EURoutingURL = server.URL
	t.Cleanup(func() { EURoutingURL = orig })
	server.Close()

	DeriveEURouting(listed, nil)

	for id := range listed {
		if got := euValue(t, "Row "+id); got != "yes" {
			t.Errorf("eu_routing of %s = %q after an unreachable server, want yes", id, got)
		}
	}
}

// The refresh runs the pass: the rows its feed lists come out marked.
func TestFetchModelsRunsTheEURoutingPass(t *testing.T) {
	catalogTestDB(t)
	renameFeed(t)
	euServer(t, 200, `{"data":[{"id":"vendor/new"}]}`)

	if _, _, _, err := FetchModels(); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{"Vendor: Brand new": "yes", "Vendor: Settled": "no", "Vendor: Renamed": "no"} {
		if got := euValue(t, name); got != want {
			t.Errorf("eu_routing of %s = %q after a refresh, want %q", name, got, want)
		}
	}
}
