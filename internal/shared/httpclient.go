package shared

import (
	"net/http"
	"time"
)

// BrowserUserAgent is the one User-Agent every outbound request carries: a plain
// current desktop Chrome string that names no person, no project and no tool, so
// a remote server's access log learns nothing about who is asking. Go's own
// default would announce the Go HTTP client instead.
//
// The version ages. [DOC] whatismybrowser.com/guides/the-latest-user-agent/chrome
// listed this string for Chrome on Windows 10 on 2026-10-09; read a current one
// from that page when it is changed, and never invent a version.
const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/154.0.0.0 Safari/537.36"

// browserTransport stamps BrowserUserAgent on every request that passes through
// it, redirects included.
type browserTransport struct{ next http.RoundTripper }

// RoundTrip sends a copy of req carrying the shared User-Agent.
func (t browserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	stamped := req.Clone(req.Context())
	stamped.Header.Set("User-Agent", BrowserUserAgent)
	return t.next.RoundTrip(stamped)
}

// NewHTTPClient is the client for every request this program sends to a remote
// server. Using it, and no other, is what keeps the User-Agent one plain string.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: browserTransport{next: http.DefaultTransport}}
}
