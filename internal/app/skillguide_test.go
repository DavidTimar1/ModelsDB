package app

import (
	"modelsdb/internal/paths"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// skillFileInRepo is the procedure as an agent working in this repository loads
// it: a Claude Code skill file, read off disk. The copy this package embeds must
// equal it exactly.
const skillFileInRepo = "../../.claude/skills/update-database-manually/SKILL.md"

// TestServedProcedureMatchesTheSkillFile is the anti-drift guard on the two
// copies of the manual-research procedure.
//
// There have to be two: go:embed cannot reach outside its own package
// directory, so the served text must live under internal/app, while the skill an
// agent loads must live under .claude/skills. Nothing in Go keeps two files in
// step, and a rule asking a person to remember has already failed here once -
// the served API guide drifted to documenting 10 of the routes the server
// registered, and what fixed that was a test, not a reminder. This is that test
// for this pair.
func TestServedProcedureMatchesTheSkillFile(t *testing.T) {
	onDisk, err := os.ReadFile(skillFileInRepo)
	if err != nil {
		t.Fatalf("reading %s: %v", skillFileInRepo, err)
	}
	if string(onDisk) == manualUpdateSkill {
		return
	}
	t.Errorf("the procedure served at /api/guide/update-database-manually has drifted from the skill file.\n"+
		"  %s is %d bytes\n"+
		"  internal/app/update-database-manually.md is %d bytes\n"+
		"Edit ONE of them, then copy it onto the other so both are identical:\n"+
		"  cp %s internal/app/update-database-manually.md",
		skillFileInRepo, len(onDisk), len(manualUpdateSkill), skillFileInRepo)
}

// TestTheServedProcedureCarriesThePreambleAndTheProcedure pins what the route
// actually answers with: this app's own preamble first, then the whole
// procedure. The preamble is the part a remote reader cannot do without - it is
// what says which steps need a shell on the server's machine - so serving the
// procedure without it would hand an HTTP-only reader instructions it cannot
// run.
func TestTheServedProcedureCarriesThePreambleAndTheProcedure(t *testing.T) {
	// Serve on a port that is not the one written into the examples, so the
	// rewrite that makes those examples runnable is exercised rather than
	// assumed.
	original := paths.Port
	paths.Port = 8125
	defer func() { paths.Port = original }()

	rec := httptest.NewRecorder()
	handleManualUpdateGuide(rec, httptest.NewRequest(http.MethodGet, "/api/guide/update-database-manually", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/markdown; charset=utf-8", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "# The manual-research procedure") {
		t.Errorf("the body does not start with the preamble; it starts %q", body[:min(80, len(body))])
	}
	// The procedure's own first heading, which must follow the preamble.
	if !strings.Contains(body, "\n# update-database-manually\n") {
		t.Error("the body does not carry the procedure itself")
	}
	if !strings.Contains(body, "## 10. When a row looks like a model already here") {
		t.Error("the body does not carry the procedure's section on duplicate rows")
	}
	// Both halves quote the API's address, and both must name the port this
	// server is answering on rather than the default written into the text.
	if strings.Contains(body, "127.0.0.1:8122") {
		t.Error("the served text still names the default port 8122 instead of the active one")
	}
	if !strings.Contains(body, "curl 'http://127.0.0.1:8125/api/models?include_hidden=0") {
		t.Error("an example inside the procedure was not rewritten to the active port")
	}
}

// TestServedPreambleSaysTheResearchRule guards the sentences ahead of the
// procedure, which the twin-file test above does not compare: an assistant
// reads the preamble first, and it once still said a typed answer outranked
// every source after the rule had changed.
func TestServedPreambleSaysTheResearchRule(t *testing.T) {
	if strings.Contains(manualUpdatePreamble, "outranks") {
		t.Errorf("the served preamble still says a written value outranks a source")
	}
	if !strings.Contains(manualUpdatePreamble, "research") {
		t.Errorf("the served preamble never names the research source a write lands as")
	}
}
