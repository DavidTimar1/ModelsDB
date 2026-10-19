package shared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Every request, a redirected one included, carries the one browser string and
// nothing that names this program.
func TestNewHTTPClientSendsOnlyTheBrowserUserAgent(t *testing.T) {
	var seen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("User-Agent"))
		http.Redirect(w, r, "/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("User-Agent"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := NewHTTPClient(5 * time.Second).Get(srv.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(seen) != 2 {
		t.Fatalf("saw %d requests, want 2", len(seen))
	}
	for _, ua := range seen {
		if ua != BrowserUserAgent {
			t.Errorf("User-Agent = %q, want the browser string", ua)
		}
	}
	for _, word := range []string{"Go-http-client", "modelsdb", "bot", "crawler", "@", "+http"} {
		if strings.Contains(strings.ToLower(BrowserUserAgent), strings.ToLower(word)) {
			t.Errorf("the User-Agent contains %q", word)
		}
	}
}
