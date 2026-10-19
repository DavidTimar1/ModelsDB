package app

import (
	"encoding/json"
	"modelsdb/internal/paths"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withAllowedOrigins stubs paths.AllowedOrigins for the duration of fn. The
// list is passed already normalized (lower-cased scheme+host, explicit port),
// the shape paths.resolveAllowedOrigins produces at startup - that resolution
// step itself, including malformed-entry dropping, is covered directly in
// internal/paths/paths_test.go.
func withAllowedOrigins(t *testing.T, normalized []string, fn func()) {
	t.Helper()
	orig := paths.AllowedOrigins
	t.Cleanup(func() { paths.AllowedOrigins = orig })
	paths.AllowedOrigins = normalized
	fn()
}

// TestOriginAllowed covers the full matching rule originAllowed enforces,
// including the attacks security-review.md names: a suffix that must not
// match, a scheme mismatch, and a port mismatch. Malformed-entry dropping
// happens at startup, in paths.resolveAllowedOrigins, and is covered directly
// by TestResolveAllowedOrigins in internal/paths/paths_test.go.
func TestOriginAllowed(t *testing.T) {
	t.Run("GET is never checked", func(t *testing.T) {
		withAllowedOrigins(t, nil, func() {
			r := httptest.NewRequest(http.MethodGet, "http://host/api/models", nil)
			r.Header.Set("Origin", "https://evil.example.com")
			if !originAllowed(r) {
				t.Error("GET should never be refused regardless of Origin")
			}
		})
	})
	t.Run("no Origin header is allowed", func(t *testing.T) {
		withAllowedOrigins(t, nil, func() {
			r := httptest.NewRequest(http.MethodPost, "http://host/api/models", nil)
			if !originAllowed(r) {
				t.Error("a request with no Origin header should be allowed (curl, scripts, assistants)")
			}
		})
	})
	t.Run("Origin equal to Host is allowed, today's behavior", func(t *testing.T) {
		withAllowedOrigins(t, nil, func() {
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8125/api/models", nil)
			r.Host = "127.0.0.1:8125"
			r.Header.Set("Origin", "http://127.0.0.1:8125")
			if !originAllowed(r) {
				t.Error("Origin matching Host should be allowed")
			}
		})
	})
	t.Run("foreign Origin refused with an empty list, today's behavior unchanged", func(t *testing.T) {
		withAllowedOrigins(t, nil, func() {
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8125/api/models", nil)
			r.Host = "127.0.0.1:8125"
			r.Header.Set("Origin", "https://evil.example.com")
			if originAllowed(r) {
				t.Error("a foreign Origin must still be refused when allowed_origins is empty")
			}
		})
	})
	t.Run("listed Origin is allowed, the fix", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name" // simulates a reverse proxy stripping the port
			r.Header.Set("Origin", "https://models.example.com")
			if !originAllowed(r) {
				t.Error("an Origin listed in allowed_origins should be allowed")
			}
		})
	})
	t.Run("listed Origin case-insensitive host still allowed", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Set("Origin", "https://Models.Example.COM")
			if !originAllowed(r) {
				t.Error("host case in the request's Origin should not matter")
			}
		})
	})
	t.Run("scheme mismatch is refused even though host is listed", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Set("Origin", "http://models.example.com")
			if originAllowed(r) {
				t.Error("http Origin must not match an https-only listed entry")
			}
		})
	})
	t.Run("port mismatch is refused even though host is listed", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:8443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Set("Origin", "https://models.example.com")
			if originAllowed(r) {
				t.Error("a port not in the listed entry must not match")
			}
		})
	})
	t.Run("suffix is refused even though host is listed", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Set("Origin", "https://evil-models.example.com")
			if originAllowed(r) {
				t.Error("a suffix match must be refused, only an exact origin matches")
			}
			r.Header.Set("Origin", "https://models.example.com.evil.com")
			if originAllowed(r) {
				t.Error("a listed host used as a suffix of a longer host must be refused")
			}
		})
	})
	t.Run("unlisted origin is refused even when a list exists", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Set("Origin", "https://other.example.com")
			if originAllowed(r) {
				t.Error("an origin absent from the list must be refused")
			}
		})
	})
	t.Run("host match still works when a list also exists", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8125/api/models", nil)
			r.Host = "127.0.0.1:8125"
			r.Header.Set("Origin", "http://127.0.0.1:8125")
			if !originAllowed(r) {
				t.Error("Host-equal-Origin must keep working once a list is configured")
			}
		})
	})
	t.Run("two Origin headers refused even with a matching list entry", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Add("Origin", "https://models.example.com")
			r.Header.Add("Origin", "https://models.example.com")
			if originAllowed(r) {
				t.Error("more than one Origin header must be refused regardless of the list")
			}
		})
	})
	t.Run("literal null Origin refused", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			r := httptest.NewRequest(http.MethodPost, "http://name/api/models", nil)
			r.Host = "name"
			r.Header.Set("Origin", "null")
			if originAllowed(r) {
				t.Error(`the literal "null" Origin must be refused`)
			}
		})
	})
}

// TestGatedHandlerBeforeStartupComplete drives gatedHandler() while
// startupComplete sits at its zero value (false), which is what every request
// sees before StartServer flips it - a state no other test in this package
// sets or resets. It pins that "/" gets the human-readable starting page while
// every other path, /api/health included, keeps the JSON writeStarting reply
// that stop/status/InspectPort/reconnectAfterRestart parse.
func TestGatedHandlerBeforeStartupComplete(t *testing.T) {
	handler := gatedHandler()

	t.Run("GET / gets the starting HTML page", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("Cache-Control = %q, want no-cache", cc)
		}
		if body := w.Body.String(); !strings.Contains(body, "ModelsDB") {
			t.Errorf("body does not contain %q: %s", "ModelsDB", body)
		}
	})

	t.Run("HEAD / gets the starting HTML page", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodHead, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
		}
	})

	t.Run("GET /api/health keeps the JSON starting reply", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("body did not decode as JSON: %v", err)
		}
		if got["status"] != "starting" {
			t.Errorf(`status field = %v, want "starting"`, got["status"])
		}
	})

	t.Run("POST /api/models also keeps the JSON starting reply", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/api/models", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
		}
	})
}
