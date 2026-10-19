package app

import (
	"strings"
	"testing"
)

// guideRouteHeadings returns the endpoint patterns the served guide gives a
// HEADING of its own, taken from the last word of every markdown heading whose
// last word looks like a path.
//
// It anchors on the heading, never on "does this text appear anywhere", because a
// substring match cannot tell two nested routes apart: the string "/api/models"
// occurs inside "/api/models/hide", so a mention of one would pass for the other
// in both directions. A heading's last word is the whole pattern or it is not.
func guideRouteHeadings() map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(apiGuide, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		words := strings.Fields(strings.TrimLeft(line, "#"))
		if len(words) == 0 {
			continue
		}
		last := words[len(words)-1]
		if strings.HasPrefix(last, "/") {
			out[last] = true
		}
	}
	return out
}

// TestEveryRegisteredRouteIsDocumented is the guard on the served guide: every
// route the server actually registers must have a section of its own in it. The
// guide had fallen to 10 of the routes then registered because features shipped a route and never
// touched the documentation, and nothing noticed. Ranging over the real route
// list is what makes the omission fail here instead of misleading a caller.
//
// The two exempt routes serve the browser app itself ("/" and "/static/"), which
// no API caller calls.
func TestEveryRegisteredRouteIsDocumented(t *testing.T) {
	routes, err := serverRoutes()
	if err != nil {
		t.Fatalf("serverRoutes: %v", err)
	}
	documented := guideRouteHeadings()
	for _, rt := range routes {
		if rt.undocumented {
			if documented[rt.pattern] {
				t.Errorf("route %q is marked undocumented but the guide gives it a heading", rt.pattern)
			}
			continue
		}
		if !documented[rt.pattern] {
			t.Errorf("route %q is registered but has no heading in the API guide (apiGuide in query.go); add a section whose heading ENDS with the pattern, e.g. \"### GET %s\"", rt.pattern, rt.pattern)
		}
	}
}

// TestGuideDocumentsOnlyRealRoutes is the same check from the other side: a
// heading in the guide that names a route the server does not register is a
// promise nothing keeps, and a typo in a pattern would otherwise read as a
// documented endpoint.
func TestGuideDocumentsOnlyRealRoutes(t *testing.T) {
	routes, err := serverRoutes()
	if err != nil {
		t.Fatalf("serverRoutes: %v", err)
	}
	registered := map[string]bool{}
	for _, rt := range routes {
		registered[rt.pattern] = true
	}
	for pattern := range guideRouteHeadings() {
		if !registered[pattern] {
			t.Errorf("the API guide documents %q, which no route registers", pattern)
		}
	}
}

// TestRoutePatternsAreUniqueAndAbsolute pins the route list itself. A duplicate
// pattern makes http.HandleFunc PANIC at startup - the server would die on the
// registration loop, before it ever listened - and a pattern without a leading
// slash can never match a request path.
func TestRoutePatternsAreUniqueAndAbsolute(t *testing.T) {
	routes, err := serverRoutes()
	if err != nil {
		t.Fatalf("serverRoutes: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("serverRoutes returned no routes")
	}
	seen := map[string]bool{}
	for _, rt := range routes {
		if !strings.HasPrefix(rt.pattern, "/") {
			t.Errorf("route pattern %q does not start with a slash", rt.pattern)
		}
		if seen[rt.pattern] {
			t.Errorf("route pattern %q is registered twice, which panics at startup", rt.pattern)
		}
		seen[rt.pattern] = true
		if rt.handler == nil {
			t.Errorf("route %q has no handler", rt.pattern)
		}
	}
}
