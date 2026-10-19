package catalog

import (
	"net/http"
	"strings"
	"testing"
)

// A repository text from a request is reduced to owner/name before it is placed
// in an API path, so it cannot reach another path on the host.
func TestARepositoryTextCannotChangeTheAPIPathItIsSplicedInto(t *testing.T) {
	hits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { hits++; http.NotFound(w, r) })
	hfServer(t, mux)

	for _, bad := range []string{"x?y=z", "../admin", "a/..", "owner/na me", "owner/na%2Fme", "https://evil.example/owner/name", ""} {
		if _, err := HFRepoBuilds(bad); err == nil {
			t.Errorf("HFRepoBuilds(%q) read a repository", bad)
		}
		if _, _, err := HFQuantizedRepos(bad); err == nil {
			t.Errorf("HFQuantizedRepos(%q) listed builds", bad)
		}
	}
	if hits != 0 {
		t.Errorf("%d request(s) reached the HuggingFace host for repository text that is not a repository", hits)
	}
	if got, err := NormalizeHFRepo("owner/name/tree/main"); err != nil || got != "owner/name" {
		t.Errorf("NormalizeHFRepo trimmed to %q, %v; want owner/name", got, err)
	}
}

// A Link header may not send the paging loop to another site.
func TestPagingNeverLeavesTheHuggingFaceAPIHost(t *testing.T) {
	foreignHits := 0
	foreign := http.NewServeMux()
	foreign.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { foreignHits++; w.Write([]byte("[]")) })
	other := hfServer(t, foreign)
	otherURL := other.URL

	mux := http.NewServeMux()
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", "<"+otherURL+"/elsewhere>; rel=\"next\"")
		w.Write([]byte(`[{"id":"someone/quant","tags":["gguf"]}]`))
	})
	// hfServer replaces the global base URLs; install the paging server last.
	server := hfServer(t, mux)
	if strings.TrimSuffix(HFAPIBaseURL, "/api") != server.URL {
		t.Fatal("test setup: the API base does not point at the paging server")
	}

	repos, _, err := HFQuantizedRepos("owner/base")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("got %d repositories, want the one on the first page", len(repos))
	}
	if foreignHits != 0 {
		t.Errorf("the paging loop followed a Link header to another host (%d request(s))", foreignHits)
	}
}
