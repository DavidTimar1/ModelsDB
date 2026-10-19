package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withServed stubs the addresses the server reports as bound.
func withServed(t *testing.T, urls []string) {
	t.Helper()
	servedMu.Lock()
	orig := servedList
	servedList = urls
	servedMu.Unlock()
	t.Cleanup(func() {
		servedMu.Lock()
		servedList = orig
		servedMu.Unlock()
	})
}

func TestHostAllowed(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		served  []string
		origins []string
		want    bool
	}{
		{"loopback ip", "127.0.0.1:8125", nil, nil, true},
		{"loopback ip without port", "127.0.0.1", nil, nil, true},
		{"localhost", "localhost:8125", nil, nil, true},
		{"localhost upper case", "LocalHost:8125", nil, nil, true},
		{"ipv6 loopback", "[::1]:8125", nil, nil, true},
		{"rebinding name", "evil.example:8125", nil, nil, false},
		{"rebinding name that starts with localhost", "localhost.evil.example:8125", nil, nil, false},
		{"rebinding name that ends with localhost", "evil-localhost:8125", nil, nil, false},
		{"empty host", "", nil, nil, false},
		{"bound lan address", "192.0.2.7:8125", []string{"http://127.0.0.1:8125", "http://192.0.2.7:8125"}, nil, true},
		{"bound lan address on another port", "192.0.2.7:9999", []string{"http://127.0.0.1:8125", "http://192.0.2.7:8125"}, nil, false},
		{"bound lan address sent without a port", "192.0.2.7", []string{"http://192.0.2.7:8125"}, nil, false},
		{"unbound lan address", "192.0.2.8:8125", []string{"http://192.0.2.7:8125"}, nil, false},
		{"proxy name from allowed_origins without port", "models.example.com", nil, []string{"https://models.example.com:443"}, true},
		{"proxy name from allowed_origins with its port", "models.example.com:443", nil, []string{"https://models.example.com:443"}, true},
		{"proxy name with a port the entry does not hold", "models.example.com:8443", nil, []string{"https://models.example.com:443"}, false},
		{"proxy name with upper case", "Models.Example.COM", nil, []string{"https://models.example.com:443"}, true},
		{"suffix of a listed name", "evil-models.example.com", nil, []string{"https://models.example.com:443"}, false},
		{"listed name used as a prefix", "models.example.com.evil.example", nil, []string{"https://models.example.com:443"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withServed(t, c.served)
			withAllowedOrigins(t, c.origins, func() {
				r := httptest.NewRequest(http.MethodGet, "/api/models", nil)
				r.Host = c.host
				if got := hostAllowed(r); got != c.want {
					t.Errorf("hostAllowed(Host %q) = %v, want %v", c.host, got, c.want)
				}
			})
		})
	}
}

// TestGatedHandlerRefusesADnsRebindingRequest sends the request a rebinding page
// makes: its own name in Host and an Origin equal to it, which the old
// Origin-equals-Host comparison alone let through, on a read and on a write.
func TestGatedHandlerRefusesADnsRebindingRequest(t *testing.T) {
	startupComplete.Store(true)
	t.Cleanup(func() { startupComplete.Store(false) })
	withServed(t, []string{"http://127.0.0.1:8125"})
	reached := false
	http.DefaultServeMux.HandleFunc("/api/test-guard-probe", func(w http.ResponseWriter, r *http.Request) { reached = true })

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			reached = false
			r := httptest.NewRequest(method, "/api/test-guard-probe", strings.NewReader("{}"))
			r.Host = "evil.example:8125"
			r.Header.Set("Origin", "http://evil.example:8125")
			w := httptest.NewRecorder()
			gatedHandler().ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", w.Code)
			}
			if reached {
				t.Error("the handler ran for a request naming a foreign host")
			}
			if strings.Contains(w.Body.String(), "evil.example") {
				t.Error("the refusal echoes the attacker's host")
			}
		})
	}

	t.Run("loopback host reaches the handler", func(t *testing.T) {
		reached = false
		r := httptest.NewRequest(http.MethodGet, "/api/test-guard-probe", nil)
		r.Host = "127.0.0.1:8125"
		w := httptest.NewRecorder()
		gatedHandler().ServeHTTP(w, r)
		if !reached {
			t.Errorf("a loopback request did not reach the handler (status %d)", w.Code)
		}
	})

	t.Run("reverse proxy name listed in allowed_origins reaches the handler", func(t *testing.T) {
		withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
			reached = false
			r := httptest.NewRequest(http.MethodPost, "/api/test-guard-probe", strings.NewReader("{}"))
			r.Host = "models.example.com"
			r.Header.Set("Origin", "https://models.example.com")
			w := httptest.NewRecorder()
			gatedHandler().ServeHTTP(w, r)
			if !reached {
				t.Errorf("a listed reverse-proxy request did not reach the handler (status %d)", w.Code)
			}
		})
	})
}

func TestGatedHandlerSendsFramingHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.Host = "127.0.0.1:8125"
	w := httptest.NewRecorder()
	gatedHandler().ServeHTTP(w, r)
	for key, want := range map[string]string{
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
	} {
		if got := w.Header().Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestGatedHandlerCapsRequestBodies reads a body past the cap through the real
// gate and expects the read to fail rather than buffer without limit.
func TestGatedHandlerCapsRequestBodies(t *testing.T) {
	startupComplete.Store(true)
	t.Cleanup(func() { startupComplete.Store(false) })
	var readErr error
	http.DefaultServeMux.HandleFunc("/api/test-body-cap-probe", func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.Copy(io.Discard, r.Body)
	})
	body := strings.NewReader(strings.Repeat("a", maxRequestBody+1))
	r := httptest.NewRequest(http.MethodPost, "/api/test-body-cap-probe", body)
	r.Host = "127.0.0.1:8125"
	gatedHandler().ServeHTTP(httptest.NewRecorder(), r)
	if readErr == nil {
		t.Error("a body past the cap was read in full")
	}
}

func TestServerHasTimeouts(t *testing.T) {
	srv := newHTTPServer(http.NotFoundHandler())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Errorf("a timeout is unset: %+v", srv)
	}
}
