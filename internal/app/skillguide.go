// The manual-research procedure, served over HTTP.
//
// GET /api/guide/update-database-manually -> the preamble below, then the
// procedure itself, as text/markdown.
//
// The procedure is a file in this repository that a person with a shell here
// reads from disk. Serving it means an agent on another machine can read the
// steps it is being asked to follow, so the flow that starts at GET /api can
// finish without anyone pasting instructions into it by hand.

package app

import (
	_ "embed"
	"fmt"
	"modelsdb/internal/paths"
	"net/http"
	"strings"
)

// manualUpdateSkill is the procedure's text, embedded from a copy that lives in
// this package because go:embed cannot reach outside its own directory - a
// pattern containing ".." fails the build outright.
//
// TestServedProcedureMatchesTheSkillFile holds this copy byte-identical to
// .claude/skills/update-database-manually/SKILL.md, which is the one that a
// Claude Code agent working in this repository actually loads. That test is the
// whole anti-drift mechanism: two copies of a long document cannot be kept
// together by asking anybody to remember, so the build goes red instead.
//
//go:embed update-database-manually.md
var manualUpdateSkill string

// manualUpdatePreamble is what this app says ahead of the procedure: which
// document is authoritative where the two differ, and which two steps of a
// research pass are not HTTP at all.
//
// It carries no backticks, matching apiGuide: a Go raw string cannot hold one,
// and the markdown reads fine without them.
const manualUpdatePreamble = `# The manual-research procedure (update-database-manually)

This is how the data the OpenRouter API never returns - parameter counts, the
prices of the models it does not price, the models that exist only on
HuggingFace - is researched and written into this catalog by hand. It is served
here so an agent on another machine can read the steps it is being asked to
follow.

Everything below the horizontal rule is the procedure itself, byte for byte as
it exists in this project's repository. Its first block is the procedure's own
YAML front matter, which is how an agent working inside that repository finds
the file on disk; ignore it when you are reading over HTTP. Every address in the
text names the port THIS server is answering on, rewritten from the default as
the text is sent, so the curl commands run exactly as printed.

Read GET /api alongside this. That guide is the authoritative reference for the
HTTP surface: every route, every writable field, the type each field takes, and
what a source can and cannot reach. Where the two disagree, GET /api is right,
because a test fails when a route is missing from it.

## The whole of it runs over HTTP

Every step of the procedure is a request to this server or to huggingface.co.
Nothing in it needs a shell on this machine, the database file, or a checkout of
the source. A column key is the plain name of the fact it answers and carries no
prefix; who stated a value is recorded on the cell instead. What you write lands
as the source "research", into a cell nothing automatic has stated yet, on ANY
model whatever created the row: it is shown until OpenRouter or the model's
HuggingFace repository states the fact, and then theirs is shown and yours sits
beneath it, dated. Where OpenRouter or the repository has already stated the
value, the write is refused, naming the source and its value. Your own
columns - the note, the favourite, every column you made -
are not yours to write.

The five requests the pass is built from:

| What you want | The request |
|---|---|
| Start the programmatic refresh | POST /api/update, then GET /api/update/status |
| Find what is unanswered | GET /api/models with a filter naming a column key, or GET /api/models/brief?name=<model> for one row |
| Learn which columns this install holds | GET /api/columns |
| Write what you researched | POST /api/save, a JSON array of {"name":"...","cells":{"context_length":"262144"}} |
| Add a model this catalog does not hold | POST /api/models/import for a HuggingFace repository, POST /api/models/add otherwise |

## The two things HTTP cannot do

1. RUN PART OF A REFRESH ON ITS OWN. POST /api/update runs the whole refresh -
   the catalog fetch, the column-making for new feed fields, the ZDR pass and the EU pass -
   and there is no way to ask for one part alone.

2. TAKE A BACKUP TO YOUR OWN DISK. POST /api/backup/run takes a complete backup,
   but the folder it names is a folder on THIS machine. Nothing streams a copy of
   the database over HTTP.

---

`

// handleManualUpdateGuide serves the preamble and the procedure as markdown. It
// does not check the method, matching handleAPIGuide: it is a document, and any
// verb that asks for it gets it.
func handleManualUpdateGuide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	text := manualUpdatePreamble + manualUpdateSkill
	// The text's examples carry the default port; reflect the active one, so a
	// reader on another machine can run them as printed.
	w.Write([]byte(strings.ReplaceAll(text, "127.0.0.1:8122", fmt.Sprintf("127.0.0.1:%d", paths.Port))))
}
