package app

import (
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReimportOfABuildTakesItsParentsTypeUnlessItsTaskSaysOtherwise holds J3.2:
// a build whose own repository states no task, or a task that maps to nothing,
// is typed as the model it was made from, a build whose repository states a task
// keeps what that task says, and a row filed under nothing still reads chat.
func TestReimportOfABuildTakesItsParentsTypeUnlessItsTaskSaysOtherwise(t *testing.T) {
	migratedDB(t)
	mux := http.NewServeMux()
	for repo, task := range map[string]string{
		"build/no-task":     ``,
		"build/odd-task":    `"pipeline_tag":"unmapped-task",`,
		"build/stated-task": `"pipeline_tag":"text-to-image",`,
		"build/orphan":      ``,
	} {
		repo, task := repo, task
		mux.HandleFunc("/api/models/"+repo, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"` + repo + `","author":"build","sha":"abc",` + task + `"gated":false,"siblings":[]}`))
		})
	}
	server := httptest.NewServer(mux)
	origAPI, origResolve := catalog.HFAPIBaseURL, catalog.HFResolveBaseURL
	catalog.HFAPIBaseURL = server.URL + "/api"
	catalog.HFResolveBaseURL = server.URL
	t.Cleanup(func() {
		catalog.HFAPIBaseURL = origAPI
		catalog.HFResolveBaseURL = origResolve
		server.Close()
	})

	const parent = "maker: tune"
	if err := store.ImportFullRecord(map[string]interface{}{"name": parent, "source": "huggingface"}, "huggingface", "music"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		repo, want string
		filed      bool
	}{
		{"build/no-task", "music", true},
		{"build/odd-task", "music", true},
		{"build/stated-task", "image", true},
		{"build/orphan", "chat", false},
	} {
		build := "maker: tune (" + c.repo + ")"
		if err := store.ImportFullRecord(map[string]interface{}{"name": build, "source": "huggingface", "hf_slug": c.repo}, "huggingface", "chat"); err != nil {
			t.Fatal(err)
		}
		if c.filed {
			if err := store.FileAsVariation(build, parent); err != nil {
				t.Fatal(err)
			}
		}
		rec := importModel(t, `{"url":"`+c.repo+`","reimport":true}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("re-import of %s answered %d: %s", c.repo, rec.Code, rec.Body.String())
		}
		if got := importedBody(t, rec)["model_type"]; got != c.want {
			t.Errorf("%s: the answer says model_type %v, want %q", c.repo, got, c.want)
		}
		cells, err := store.ModelCells(build)
		if err != nil {
			t.Fatal(err)
		}
		if got := cells["model_type"]; got != c.want {
			t.Errorf("%s: stored model_type = %q, want %q", c.repo, got, c.want)
		}
	}
}
