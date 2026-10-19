package app

import (
	"errors"
	"log"
	"modelsdb/internal/paths"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxRequestBody caps every request body the server reads. The largest honest
// body is a bulk save or a settings write, far below this; the cap exists so a
// request cannot make the server buffer without limit.
const maxRequestBody = 16 << 20

// Server timeouts. ReadHeaderTimeout and IdleTimeout stop a client that
// connects and then says nothing from holding a goroutine and a socket for ever.
// WriteTimeout is long on purpose: an import reads several remote pages before it
// answers, and a short limit would cut that answer off.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 2 * time.Minute
	writeTimeout      = 10 * time.Minute
	idleTimeout       = 2 * time.Minute
)

// newHTTPServer wraps handler in a server with the timeouts above.
func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// setSecurityHeaders keeps another site from framing the page, which would let
// it trick a click on a button, and stops a browser guessing a content type.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
}

// decodeFailureStatus is the status for a body that could not be decoded: 413
// when it ran past maxRequestBody, 400 when it was simply not the expected JSON.
func decodeFailureStatus(err error) int {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// hostAllowed reports whether the Host header names this server. A page on a
// hostile site can rebind its own name to this machine, so its requests arrive
// here carrying that name in both Host and Origin and would pass a check that
// compared the two with each other. The name must instead be one this server
// is known by: a loopback name, an address a listener really bound, or the host
// of an entry in allowed_origins (the reverse-proxy case).
func hostAllowed(r *http.Request) bool {
	host, port := splitHostHeader(r.Host)
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, served := range servedAddresses() {
		if hostMatches(served, host, port, true) {
			return true
		}
	}
	for _, listed := range paths.AllowedOrigins {
		if hostMatches(listed, host, port, false) {
			return true
		}
	}
	return false
}

// hostMatches reports whether the base URL names host, and port when the Host
// header carried one. A proxy may forward Host without a port, so an absent port
// matches an allowed_origins entry (portRequired false) and never a bound address.
func hostMatches(baseURL, host, port string, portRequired bool) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Hostname(), host) {
		return false
	}
	if port == "" {
		return !portRequired
	}
	return u.Port() == port
}

// splitHostHeader splits a Host header into a lower-cased host and its port,
// which is empty when the header carries none. A bracketed IPv6 literal loses
// its brackets.
func splitHostHeader(hostHeader string) (host, port string) {
	h := strings.TrimSpace(hostHeader)
	if h == "" {
		return "", ""
	}
	if hh, pp, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(strings.TrimSuffix(hh, ".")), pp
	}
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	return strings.ToLower(strings.TrimSuffix(h, ".")), ""
}

// writeForeignHost refuses a request whose Host header is not a name this
// server answers to. The offending Host is never echoed: it is text whoever
// mounted the attack chose.
func writeForeignHost(w http.ResponseWriter, r *http.Request) {
	log.Printf("REFUSED [%s %q] request names a host this server does not answer to", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("Refused: this request names a host this server does not answer to.\n" +
		"Reach ModelsDB by 127.0.0.1, localhost or an address it is bound to. Behind a reverse proxy, list the public address under \"allowed_origins\" in config.jsonc and restart.\n"))
}
