package app

import (
	"encoding/json"
	"modelsdb/internal/paths"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// withServedList runs fn with the served-address list replaced, and restores it
// afterwards, so a test can state exactly which listeners "bound" without
// starting a server.
func withServedList(t *testing.T, list []string, fn func()) {
	t.Helper()
	servedMu.Lock()
	saved := servedList
	servedList = list
	servedMu.Unlock()
	defer func() {
		servedMu.Lock()
		servedList = saved
		servedMu.Unlock()
	}()
	fn()
}

// TestReachableURLsNameOnlyAddressesAClientCanType covers the one translation
// step between a bound TCP address and what a brief may print. A specific
// address is itself; a wildcard bind is not a destination anybody can send to,
// so it becomes loopback plus this machine's own interface addresses.
func TestReachableURLsNameOnlyAddressesAClientCanType(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want []string
	}{
		{"loopback", "127.0.0.1:8122", []string{"http://127.0.0.1:8122"}},
		{"tailscale address", "100.64.0.10:8122", []string{"http://100.64.0.10:8122"}},
		{"ipv6 loopback keeps its brackets", "[::1]:8122", []string{"http://[::1]:8122"}},
		{"a hostname is passed through", "modelsdb.local:8122", []string{"http://modelsdb.local:8122"}},
		{"not an address at all", "nonsense", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reachableURLs(c.addr); !slices.Equal(got, c.want) {
				t.Errorf("reachableURLs(%q) = %v, want %v", c.addr, got, c.want)
			}
		})
	}

	// A wildcard bind answers everywhere, so loopback leads and no unusable
	// address (0.0.0.0 itself, or a link-local address needing a zone) appears.
	got := reachableURLs("0.0.0.0:8122")
	if len(got) == 0 || got[0] != "http://127.0.0.1:8122" {
		t.Fatalf("wildcard bind = %v, want loopback first", got)
	}
	for _, u := range got {
		if strings.Contains(u, "0.0.0.0") || strings.Contains(u, "[fe80:") {
			t.Errorf("wildcard bind offered an address nobody can send to: %q", u)
		}
	}
}

// TestServedAddressesHoldOnlyWhatBound is the promise the brief rests on: an
// address is reported because its listener bound, never because it was tried.
func TestServedAddressesHoldOnlyWhatBound(t *testing.T) {
	withServedList(t, nil, func() {
		if got := servedAddresses(); len(got) != 0 {
			t.Fatalf("a process with no listener reported %v, want nothing", got)
		}

		// One listener binds; a second address is refused by the operating
		// system (a port already held), exactly as a down interface would be.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer ln.Close()
		bound := ln.Addr().String()
		recordServed(bound)

		if _, err := net.Listen("tcp", bound); err == nil {
			t.Fatal("expected the second bind on the same address to fail")
		}

		want := []string{"http://" + bound}
		if got := servedAddresses(); !slices.Equal(got, want) {
			t.Errorf("servedAddresses() = %v, want %v", got, want)
		}

		// Recording the same listener twice does not double it, and the copy
		// handed out cannot be written back into the list.
		recordServed(bound)
		got := servedAddresses()
		if !slices.Equal(got, want) {
			t.Errorf("after a repeat record: %v, want %v", got, want)
		}
		got[0] = "http://tampered"
		if again := servedAddresses(); !slices.Equal(again, want) {
			t.Errorf("the returned slice aliases the list: %v", again)
		}
	})
}

// TestHealthReportsEveryServedAddress is what the page reads: GET /api/health
// carries the full list, in bind order with loopback first, so the brief can
// name every address and say which fits where.
func TestHealthReportsEveryServedAddress(t *testing.T) {
	want := []string{"http://127.0.0.1:8122", "http://100.64.0.10:8122"}
	withServedList(t, want, func() {
		rec := httptest.NewRecorder()
		handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var got struct {
			Addresses []string `json:"addresses"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("body not JSON: %v", err)
		}
		if !slices.Equal(got.Addresses, want) {
			t.Errorf("addresses = %v, want %v", got.Addresses, want)
		}
	})

	// A server that has bound nothing reports an empty list, not a missing
	// field: the page must be able to tell "none reported" from "not answered".
	withServedList(t, nil, func() {
		rec := httptest.NewRecorder()
		handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
		if !strings.Contains(rec.Body.String(), `"addresses":[]`) {
			t.Errorf("empty list not reported as []: %s", rec.Body.String())
		}
	})
}

// TestHealthReportsAllowedOrigins covers the two fields GET /api/health adds
// for config.jsonc "allowed_origins": the normalized list actually in effect,
// and how many entries this startup dropped as malformed - so a bad edit is
// visible without reading the log, and an empty list still reports "[]", not a
// missing field.
func TestHealthReportsAllowedOrigins(t *testing.T) {
	origOrigins, origDropped := paths.AllowedOrigins, paths.AllowedOriginsDropped
	t.Cleanup(func() { paths.AllowedOrigins, paths.AllowedOriginsDropped = origOrigins, origDropped })

	paths.AllowedOrigins = []string{"https://models.example.com:443"}
	paths.AllowedOriginsDropped = 2
	rec := httptest.NewRecorder()
	handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got struct {
		AllowedOrigins        []string `json:"allowed_origins"`
		AllowedOriginsDropped int      `json:"allowed_origins_dropped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if !slices.Equal(got.AllowedOrigins, []string{"https://models.example.com:443"}) {
		t.Errorf("allowed_origins = %v, want the normalized list in effect", got.AllowedOrigins)
	}
	if got.AllowedOriginsDropped != 2 {
		t.Errorf("allowed_origins_dropped = %d, want 2", got.AllowedOriginsDropped)
	}

	paths.AllowedOrigins = []string{}
	paths.AllowedOriginsDropped = 0
	rec = httptest.NewRecorder()
	handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if !strings.Contains(rec.Body.String(), `"allowed_origins":[]`) {
		t.Errorf("empty allowed_origins not reported as [], got: %s", rec.Body.String())
	}
}
