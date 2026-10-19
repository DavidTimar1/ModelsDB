// The research brief for one model, written by the SERVER.
//
// GET /api/models/brief?name=<model> -> what that row still has no answer for,
// and the instructions for filling it in, as one block of text to hand to an
// assistant.
//
// IT IS BUILT FROM THE RULES THE WRITE PATH ENFORCES, never from prose kept
// alongside them: store.BriefOf reads the row against the keys a save accepts on
// it, which are the columns research may fill and nothing automatic has yet,
// and leaves out every one that cannot describe this kind of row.
// The browser prints what arrives here and writes none of it, which is what
// stops the page and the server disagreeing about what may be written.

package app

import (
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// handleModelBrief (GET /api/models/brief?name=...) answers with one model's
// gaps and the brief that asks for them.
//
// The optional "origin" parameter is the address the CALLER reached this server
// at, which is written into every instruction in the text. The browser sends the
// address in its own location bar, because that is the one address it can prove
// works; every address this process is really serving on is listed beside it, so
// an assistant on another machine can pick the one that suits it.
func handleModelBrief(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if strings.TrimSpace(name) == "" {
		http.Error(w, "Pass ?name=<model>, exactly as GET /api/models returns it.", http.StatusBadRequest)
		return
	}
	row, err := store.BriefOf(name)
	if errors.Is(err, store.ErrNoSuchModel) {
		http.Error(w, fmt.Sprintf("No model called %q is in this catalog.", name), http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("ERROR [%s %q] work out the gaps on %q: %v", r.Method, r.URL.Path, name, err)
		http.Error(w, fmt.Sprintf("Failed to read what %q is missing: %v", name, err), http.StatusInternalServerError)
		return
	}
	slug, err := store.RepositoryOf(name)
	if err != nil {
		log.Printf("ERROR [%s %q] read the repository of %q: %v", r.Method, r.URL.Path, name, err)
	}

	addresses := briefAddresses(r.URL.Query().Get("origin"))
	example := briefExample(name, row.Gaps)
	writeJSON(w, map[string]interface{}{
		"name":    name,
		"hf_slug": slug,
		"gaps":    row.Gaps,
		"held":    row.Held,
		"example": example,
		"brief":   briefText(name, slug, row, addresses, example),
	})
}

// briefAddresses is every address to offer, the caller's own first. An address
// this process reports is skipped when the caller is already on it, and a caller
// that named none leaves the server's own list to lead.
//
// The last resort is the loopback address of the port this instance was
// configured with. A process that never started a listener - a test, or a data
// subcommand - reports no served address at all, and an instruction with no
// address in it cannot be followed.
func briefAddresses(origin string) []string {
	var out []string
	if u, err := url.Parse(strings.TrimRight(strings.TrimSpace(origin), "/")); err == nil &&
		(u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && servedHost(u.Scheme+"://"+u.Host) {
		out = append(out, u.Scheme+"://"+u.Host)
	}
	for _, a := range servedAddresses() {
		if !contains(out, a) {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		out = append(out, "http://127.0.0.1:"+strconv.Itoa(paths.Port))
	}
	return out
}

// servedHost reports whether origin (scheme + host, e.g. "https://name") is
// one this process really answers on: either a bound listener (servedAddresses)
// or an address the operator vouched for in config.jsonc "allowed_origins"
// (paths.OriginListed) - the same second way of vouching originAllowed accepts
// in server.go, needed here behind a reverse proxy where the page's own origin
// is never a bound address. The address the caller names is written into every
// instruction in the brief, the "curl -X POST .../api/save" line included, and
// whoever pastes the brief into an assistant runs those instructions. An
// address that was never checked would let anyone who can get a link clicked
// choose where that assistant sends the reader's data. The page itself passes
// window.location.origin, which is by definition an address this process
// answers on.
func servedHost(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	for _, a := range servedAddresses() {
		if x, err := url.Parse(a); err == nil && strings.EqualFold(x.Host, u.Host) {
			return true
		}
	}
	return paths.OriginListed(origin)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// addressUse says what one address is FOR, in the words somebody choosing
// between them needs. A loopback address works only from a process on this
// machine; every other one is how another machine reaches this server.
func addressUse(raw string) string {
	host := raw
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	if host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.") {
		return "only from a process on the same machine as ModelsDB"
	}
	return "from another machine that can reach " + host
}

// briefExample is the body of the worked request: this model's name, which is
// the one real value in it, and a placeholder for each of the first few gaps.
// The name is real because it is not a finding - it says which row is being
// written, and a wrong one is a 404 rather than a wrong fact.
//
// Every gap is a CELL, named by the key it is stored and read under, so the
// example is the same shape a read answers with.
func briefExample(name string, gaps []store.Gap) string {
	cells := []string{}
	for _, g := range gaps {
		if g.Measured || len(cells) >= 3 {
			continue
		}
		cells = append(cells, strconv.Quote(g.Field)+":"+g.Placeholder)
	}
	return `[{"name":` + strconv.Quote(name) + `,"cells":{` + strings.Join(cells, ",") + "}}]"
}

// briefText is the whole brief, as the reader sees it.
//
// EVERY FIELD NAME IN IT COMES FROM store.BriefOf, so the text can only ask for
// what POST /api/save takes on this particular row. The prose around those names
// is the part a person wrote, and it says nothing about which fields exist.
func briefText(name, slug string, row store.RowBrief, addresses []string, example string) string {
	gaps := row.Gaps
	origin := addresses[0]
	var b []string
	add := func(format string, args ...interface{}) { b = append(b, fmt.Sprintf(format, args...)) }

	add(`ModelsDB needs research on one model: %q.`, name)
	add("")
	if slug != "" {
		add("Its HuggingFace repository: https://huggingface.co/%s", slug)
		add("")
	}
	if len(addresses) > 1 {
		add("This ModelsDB answers at each of these addresses:")
		for _, a := range addresses {
			add("  %s  - %s", a, addressUse(a))
		}
		add("Every ModelsDB address below is written as %s. Put whichever address", origin)
		add("above suits where you are running in its place - the paths after it are")
		add("identical.")
	} else {
		add("This ModelsDB answers at %s, %s.", origin, addressUse(origin))
		add("Every ModelsDB address below uses it.")
	}
	add("")

	b = append(b, rowSection(row.Profile)...)
	b = append(b, heldSection(row.Held)...)
	b = append(b, gapSection(gaps)...)

	add("WHERE TO LOOK.")
	add("1. Read %s/api/guide/update-database-manually - the procedure this", origin)
	add("   database expects, including what each field means and what counts as a source.")
	add("2. Read %s/api for every field this API writes and the type each takes.", origin)
	add("3. Use the sources that exist. The model's own card on HuggingFace is the one")
	add("   every model has. A paper, and the provider's own documentation, exist for")
	add("   some models and not for others - read them WHERE THERE IS ONE. A model with")
	add("   no paper and no provider page is normal, and a source you could not find is")
	add("   not a failure: say which fields you could not establish and leave them out.")
	add("   Do not guess a number, and do not fill a field from another model's answer.")
	add("   ONE EXCEPTION: where this model keeps the same weights as its base model (a")
	add("   quantized or converted copy, a fine-tune that changes behaviour and not size),")
	add("   the base model's card counts as the source for facts about those weights.")
	add("   Where the sources are silent and the answer is plainly no for this kind of")
	add("   model, write \"no\"; leave anything less obvious empty.")
	nextStep := 4
	if askedForSize(gaps) {
		nextStep = 5
		add("4. A repository that publishes more than one copy, or more than one format, of")
		add("   the same weights is the trap in every size figure. Count ONLY the files")
		add("   model.safetensors.index.json names, taking each file's size from")
		add("   https://huggingface.co/api/models/<repo>?blobs=true; where there is no such")
		add("   index the single model.safetensors is the whole weight set. Everything else")
		add("   in the repository is a second copy or another format and is NOT added to it:")
		add("   a consolidated.safetensors, an original/ or metal/ folder, a GGUF or")
		add("   quantized mirror, and any folder holding the same tensors at another")
		add("   precision. A GGUF repository states its parameter count as gguf.total in the")
		add("   same listing, and its sizes come from its own files. A diffusers repository")
		add("   keeps its weights in one folder per component (transformer/, text_encoder/):")
		add("   count the component that does the generating and say which you counted.")
		add("   If a repository leaves you unable to tell one copy from another, write no")
		add("   size at all and say so.")
	}
	add("%d. Write only the fields listed above. A value you send is kept as research", nextStep)
	add("   and shown until OpenRouter or the model's repository states the fact,")
	add("   after which theirs is shown; a field one of them already states is refused,")
	add("   because nothing typed replaces what an update maintains. If a value a source")
	add("   states looks WRONG, do not try to write over it: report it in your answer to")
	add("   the person, naming the field, the value shown, what the source really says")
	add("   and where. The note, the favourite and every column the reader made are")
	add("   theirs alone: do not write them, and do not create columns. The one")
	add("   exception is the Architecture line, key architecture_basis, which the update")
	add("   procedure (section 6a) has you write after reading the model's config.json.")
	add("")
	add("HOW TO SEND WHAT YOU FOUND.")
	add("  curl -X POST %s/api/save \\", origin)
	add("    -H 'Content-Type: application/json' \\")
	add("    -d '%s'", example)
	add("")
	add("EVERY VALUE IN THAT EXAMPLE IS A PLACEHOLDER, not this model's answer. Replace")
	add("each <...> with what you actually found, and delete the fields you did not")
	add("establish. Sent as it stands the request is refused and nothing is written.")
	add("")
	add(`The name must be exactly %q, character for character. Leave a field out rather`, name)
	add("than guess it. A field sent as the wrong type is refused with 400 and NOTHING")
	add("in that object is written - not that field and not the others beside it.")
	return strings.Join(b, "\n")
}

// rowSection says what kind of row this is, which is why some fields are not
// offered: the reader of the brief should know a missing field was left out on
// purpose.
func rowSection(p store.RowProfile) []string {
	kind := p.ModelType
	if kind == "" {
		kind = "not stated yet"
	}
	listed := "no, so fields that describe an OpenRouter listing are not offered"
	if p.Listed {
		listed = "yes"
	}
	shape := "a model row"
	if p.Build {
		shape = "one quantized build filed under a model"
	}
	lines := []string{
		"WHAT THIS ROW IS. Kind of model: " + kind + ". Listed on OpenRouter: " + listed + ".",
		"It is " + shape + ". Only fields that can describe this kind of row are offered below.",
	}
	if p.IsGGUFRepository() {
		lines = append(lines,
			"It is a GGUF repository: one file per quantization and none of them is the",
			"weights, so no weight size is offered. Its parameter count is the",
			"gguf.total its listing states.")
	}
	return append(lines, "")
}

// heldSection prints what the row already holds, so nothing is researched twice
// and a wrong stored value can be noticed.
func heldSection(held []store.Held) []string {
	if len(held) == 0 {
		return nil
	}
	b := []string{"WHAT THIS ROW HOLDS NOW (do not send these; a source or you already answered):"}
	for _, h := range held {
		b = append(b, fmt.Sprintf("  %s  (%s)  =  %s   [%s]", h.Field, h.Label, h.Value, h.Source))
	}
	return append(b, "")
}

// askedForSize reports whether the brief asks for a parameter count or a size,
// the facts the repository-files warning is about.
func askedForSize(gaps []store.Gap) bool {
	for _, g := range gaps {
		switch g.Field {
		case "parameters", "active_parameters", "weights_size_gb":
			return true
		}
	}
	return false
}

// gapSection lists what the row has no answer for, split by whether it is
// something to research at all or a measurement to be read off the repository.
func gapSection(gaps []store.Gap) []string {
	var fixed, measured []store.Gap
	for _, g := range gaps {
		if g.Measured {
			measured = append(measured, g)
		} else {
			fixed = append(fixed, g)
		}
	}
	var b []string
	add := func(format string, args ...interface{}) { b = append(b, fmt.Sprintf(format, args...)) }
	if len(gaps) == 0 {
		add("EVERY FIELD ON THIS MODEL HOLDS A VALUE. Check the stored values against the")
		add("sources below and correct anything that is wrong.")
		add("")
		return b
	}
	add("WHAT IS UNANSWERED ON THIS MODEL, AND WHAT EACH FIELD TAKES.")
	add("Every name below is exactly the key to send, and a value is sent as TEXT.")
	add("")
	if len(fixed) > 0 {
		add("Fields nothing has stated yet, which research may fill - send each inside")
		add("\"cells\", under the key printed here:")
		b = append(b, gapLines(fixed)...)
		add("")
	}
	if len(measured) > 0 {
		add("MEASURED, NOT RESEARCHED. These columns hold what a repository was READ to")
		add("say, so the honest way to fill them is to read the repository again:")
		add("POST /api/models/import with {\"url\":\"<owner/model>\",\"reimport\":true}.")
		add("Write one by hand only when there is no repository to read, and then only")
		add("under the rule on sizes in point 4 below, where it is given:")
		b = append(b, gapLines(measured)...)
		add("")
	}
	return b
}

// gapLines writes one gap per line, with the name padded so the three answers
// line up under each other.
func gapLines(gaps []store.Gap) []string {
	width := 0
	for _, g := range gaps {
		if len(g.Field) > width {
			width = len(g.Field)
		}
	}
	out := make([]string, 0, len(gaps))
	for _, g := range gaps {
		line := fmt.Sprintf("  %-*s  (%s)  now: %-8s  takes: %s", width, g.Field, g.Label, g.Current, g.Accepts)
		if g.About != "" {
			line += "\n" + strings.Repeat(" ", width+4) + "(" + g.About + ")"
		}
		out = append(out, line)
	}
	return out
}
