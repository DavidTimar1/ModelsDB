package dbcore

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"modelsdb/internal/shared"
)

// This file is migration v52: the HuggingFace architecture and weight-format
// columns are folded into the Architecture column, and twenty-six columns go.
//
// WHY. One line of facts per model reads better than twenty-six mostly empty
// columns, and the import no longer records these facts: a person or assistant
// writes the line (the update-database-manually procedure, section 6a). What the
// import already measured is written into the line first, so nothing a model
// had is lost, and only then are the columns deleted.
//
// WHAT IT TOUCHES.
//   - the Architecture column (key ArchitectureColumnKey): sealed, with hand
//     declared; created here when the catalog has none.
//   - cells on Architecture, source hand: a model with no entry gets the
//     generated line; an entry that is exactly "unknown" is replaced by it; any
//     other entry keeps every word and gains only the weights facts whose exact
//     word it lacks (see architectureLine and architectureAddition).
//   - cells on gguf_total_bytes: a count of parameters stored under a name that
//     says bytes. Where the model has no Parameters cell from any source, the
//     count moves to Parameters in billions, as a huggingface cell.
//   - columns: the twenty-six rows are deleted; DELETE FROM columns cascades to
//     their column_sources, column_options and cells.
//
// NO WRONG NUMBER IS WRITTEN. The importer counted a sliding-window layer as
// one that holds a full-length cache, and read the local head figures instead
// of the global ones, so its cache figures over-state those models; a
// Qwen3-Next style model has the same trouble. For a row whose layer types
// include sliding_attention, or whose class starts with Qwen3Next, the line
// gives the layer counts, hidden size and attention heads and says nothing about
// head dim, KV heads or the cache.
//
// A stored view that still names a removed key in its column order, hidden
// list or filters needs no rewrite: the page and the query route already pass
// over a key that matches no column.
//
// cellsMayFall IS SET, because deleting those cells is the whole purpose. The
// migration's own exact refusal stands in for the chain's guard: the cell count
// must fall by exactly the cells on the deleted columns less the Architecture
// and Parameters cells created, and every entry that was real text must still
// be the start of its new text, or the pre-migration snapshot comes back.

// The Architecture column, mirroring the shipped definition in
// store.shippedColumns - store sits above dbcore in the import DAG, so it cannot
// be read from here. Change both together; store's
// TestTheArchitectureColumnShipsInStepWithItsMigration holds them in step.
const (
	ArchitectureColumnKey   = "architecture_basis"
	ArchitectureColumnLabel = "Architecture"
	ArchitectureColumnAbout = "How the model is built, as one line of facts: layers, what holds a cache, heads, weights. Yours to edit; an assistant fills it from the repository's config.json."
)

// architectureFoldKeys are the columns whose facts become part of the line.
var architectureFoldKeys = []string{
	"attention_layers", "kv_heads", "head_dim", "hidden_size", "hidden_layers",
	"attention_heads", "layer_types", "recurrent_layers", "state_bytes_per_layer",
	"kv_bytes_per_token", "architecture_class", "weight_dtype", "declared_weight_bits",
	"quantization_method", "weight_file_format",
}

// architectureDropKeys are the columns that lose nothing: five facts already
// held elsewhere or empty, and gguf_total_bytes, which moves to Parameters.
var architectureDropKeys = []string{
	"state_size", "state_heads", "state_head_dim", "state_groups", "state_expand",
	"state_conv_kernel", "vision_projector", "gguf_total_bytes", "weight_bytes",
	"build_repository", "build_label",
}

// theArchitectureColumnHoldsWhatTheHuggingFaceColumnsHeld is migration v52.
func theArchitectureColumnHoldsWhatTheHuggingFaceColumnsHeld(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to fold
	}

	doomed, err := foldColumnIDs(tx)
	if err != nil {
		return err
	}
	archID, haveArch, err := architectureColumn(tx)
	if err != nil {
		return err
	}
	if len(doomed) == 0 {
		// Nothing to fold. A column that exists is still sealed, which is what a
		// shipped column is; one that does not exist is the seed's to create.
		if haveArch {
			return sealArchitectureColumn(tx, archID)
		}
		return nil
	}

	var cellsBefore, doomedCells, modelsBefore int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells`).Scan(&cellsBefore); err != nil {
		return fmt.Errorf("count the cells before folding the architecture columns: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM models`).Scan(&modelsBefore); err != nil {
		return fmt.Errorf("count the models before folding the architecture columns: %w", err)
	}
	for _, id := range doomed {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=?`, id).Scan(&n); err != nil {
			return fmt.Errorf("count the cells on column %d: %w", id, err)
		}
		doomedCells += n
	}
	logColorRules(tx, doomed)

	facts, err := foldedFacts(tx)
	if err != nil {
		return err
	}

	if !haveArch {
		archID, err = createArchitectureColumn(tx)
		if err != nil {
			return err
		}
	} else if err := sealArchitectureColumn(tx, archID); err != nil {
		return err
	}
	var archBefore int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=?`, archID).Scan(&archBefore); err != nil {
		return fmt.Errorf("count the architecture cells before folding: %w", err)
	}
	existing, err := architectureEntries(tx, archID)
	if err != nil {
		return err
	}

	var outcome foldOutcome
	written := map[string]string{} // model -> the text now in its cell
	for _, model := range sortedModels(facts) {
		entry, exists := existing[model]
		text, how, err := writeArchitecture(tx, archID, model, facts[model], entry, exists)
		if err != nil {
			return err
		}
		outcome.count(how)
		if how != foldNothing {
			written[model] = text
		}
	}

	movedParameters, err := moveGGUFTotalToParameters(tx)
	if err != nil {
		return err
	}

	for _, id := range doomed {
		if _, err := tx.Exec(`DELETE FROM columns WHERE id=?`, id); err != nil {
			return fmt.Errorf("delete column %d: %w", id, err)
		}
	}

	if err := checkArchitectureFold(tx, archID, archBefore, cellsBefore, modelsBefore, doomedCells, outcome, movedParameters, existing, written); err != nil {
		return err
	}
	log.Printf("Migration v52: folded the architecture columns into %q and deleted %d column(s) holding %d cell(s); lines: %d new, %d replaced an unknown, %d extended, %d already complete, %d with nothing to say; %d parameter count(s) moved from the GGUF total",
		ArchitectureColumnLabel, len(doomed), doomedCells, outcome.created, outcome.replaced, outcome.extended, outcome.unchanged, outcome.nothing, movedParameters)
	return nil
}

// foldColumnIDs answers the id of each of the twenty-six columns that exists.
func foldColumnIDs(tx *sql.Tx) ([]int64, error) {
	var ids []int64
	for _, key := range append(append([]string{}, architectureFoldKeys...), architectureDropKeys...) {
		var id int64
		err := tx.QueryRow(`SELECT id FROM columns WHERE column_key=?`, key).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue // this catalog never had the column
		}
		if err != nil {
			return nil, fmt.Errorf("find the %s column: %w", key, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// logColorRules names each column about to go that carried colour rules. The
// rules are the reader's setting and go with the column, so the log is the
// record of them.
func logColorRules(tx *sql.Tx, ids []int64) {
	for _, id := range ids {
		var key, rules string
		if err := tx.QueryRow(`SELECT column_key, color_rules FROM columns WHERE id=?`, id).Scan(&key, &rules); err == nil && rules != "" {
			log.Printf("Migration v52: column %s is deleted with its colour rules %s", key, rules)
		}
	}
}

// architectureColumn finds the Architecture column. A column under that key
// that is not text cannot hold the line, so the migration stops rather than
// writing text into a number.
func architectureColumn(tx *sql.Tx) (id int64, found bool, err error) {
	var valueType string
	err = tx.QueryRow(`SELECT id, value_type FROM columns WHERE column_key=?`, ArchitectureColumnKey).Scan(&id, &valueType)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find the %s column: %w", ArchitectureColumnKey, err)
	}
	if valueType != "text" {
		return 0, false, fmt.Errorf("the %s column holds %s, and the architecture line is text: rename that column before upgrading", ArchitectureColumnKey, valueType)
	}
	return id, true, nil
}

// sealArchitectureColumn makes an existing Architecture column a shipped one:
// sealed, with hand declared and a description. Its label, position and
// visibility are the reader's and stay.
func sealArchitectureColumn(tx *sql.Tx, id int64) error {
	if _, err := tx.Exec(`UPDATE columns SET sealed=1, about=CASE WHEN about='' THEN ? ELSE about END WHERE id=?`,
		ArchitectureColumnAbout, id); err != nil {
		return fmt.Errorf("seal the architecture column: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO column_sources (column_id, source, rank)
		SELECT ?, 'hand', COALESCE((SELECT MIN(rank) FROM column_sources WHERE column_id=?), 1) - 1
		WHERE NOT EXISTS (SELECT 1 FROM column_sources WHERE column_id=? AND source='hand')`, id, id, id); err != nil {
		return fmt.Errorf("declare hand on the architecture column: %w", err)
	}
	return nil
}

// createArchitectureColumn adds the column, sealed and hidden, one position past
// the highest shipped one, the way migration v34 placed Added.
func createArchitectureColumn(tx *sql.Tx) (int64, error) {
	var last int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) FROM columns WHERE sealed=1`).Scan(&last); err != nil {
		return 0, fmt.Errorf("read where the shipped columns end: %w", err)
	}
	res, err := tx.Exec(`INSERT INTO columns (column_key, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES (?, ?, 'text', '', ?, ?, 0, 1, ?, '')`,
		ArchitectureColumnKey, ArchitectureColumnLabel, ArchitectureColumnAbout, last+1, NowStamp())
	if err != nil {
		return 0, fmt.Errorf("create the %s column: %w", ArchitectureColumnKey, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?, 'hand', 0)`, id); err != nil {
		return 0, fmt.Errorf("declare hand on the architecture column: %w", err)
	}
	return id, nil
}

// foldedFacts reads, for each model, the winning cell of each folded column,
// keyed by column key. The winner is the declared source that ranks first, the
// order a read uses.
func foldedFacts(tx *sql.Tx) (map[string]map[string]string, error) {
	marks := strings.TrimSuffix(strings.Repeat("?,", len(architectureFoldKeys)), ",")
	args := make([]interface{}, len(architectureFoldKeys))
	for i, k := range architectureFoldKeys {
		args[i] = k
	}
	rows, err := tx.Query(`SELECT c.model_name, k.column_key, c.value_text
		FROM cells c
		JOIN columns k ON k.id = c.column_id
		LEFT JOIN column_sources cs ON cs.column_id = c.column_id AND cs.source = c.source
		WHERE k.column_key IN (`+marks+`)
		ORDER BY c.model_name, k.column_key, COALESCE(cs.rank, 1000000), c.source`, args...)
	if err != nil {
		return nil, fmt.Errorf("read the architecture facts: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var model, key, value string
		if err := rows.Scan(&model, &key, &value); err != nil {
			return nil, err
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		if out[model] == nil {
			out[model] = map[string]string{}
		}
		if _, have := out[model][key]; !have {
			out[model][key] = strings.TrimSpace(value)
		}
	}
	return out, rows.Err()
}

// architectureEntries reads every hand entry on the Architecture column.
func architectureEntries(tx *sql.Tx, archID int64) (map[string]string, error) {
	rows, err := tx.Query(`SELECT model_name, value_text FROM cells WHERE column_id=? AND source='hand'`, archID)
	if err != nil {
		return nil, fmt.Errorf("read the architecture entries: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var model, text string
		if err := rows.Scan(&model, &text); err != nil {
			return nil, err
		}
		out[model] = text
	}
	return out, rows.Err()
}

func sortedModels(facts map[string]map[string]string) []string {
	out := make([]string, 0, len(facts))
	for m := range facts {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// foldHow says what the migration did to one model's Architecture cell.
type foldHow int

const (
	foldNothing foldHow = iota // no line could be built from the facts
	foldCreated
	foldReplaced
	foldExtended
	foldUnchanged
)

type foldOutcome struct{ created, replaced, extended, unchanged, nothing int }

func (o *foldOutcome) count(h foldHow) {
	switch h {
	case foldCreated:
		o.created++
	case foldReplaced:
		o.replaced++
	case foldExtended:
		o.extended++
	case foldUnchanged:
		o.unchanged++
	default:
		o.nothing++
	}
}

// writeArchitecture applies the rule to one model: no entry gets the generated
// line, an entry that is exactly "unknown" is replaced by it, and any other
// entry keeps every word and gains only what it lacks. It answers the text the
// cell holds afterwards.
func writeArchitecture(tx *sql.Tx, archID int64, model string, facts map[string]string, entry string, exists bool) (string, foldHow, error) {
	trimmed := strings.TrimSpace(entry)
	switch {
	case trimmed == "" || strings.EqualFold(trimmed, "unknown"):
		line := architectureLine(facts)
		if line == "" {
			return entry, foldNothing, nil
		}
		if !exists {
			if _, err := tx.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
				VALUES (?,?,'hand',?,NULL,?)`, archID, model, line, NowStamp()); err != nil {
				return "", 0, fmt.Errorf("write the architecture line of %q: %w", model, err)
			}
			return line, foldCreated, nil
		}
		if _, err := tx.Exec(`UPDATE cells SET value_text=?, value_number=NULL, written_at=?
			WHERE column_id=? AND model_name=? AND source='hand'`, line, NowStamp(), archID, model); err != nil {
			return "", 0, fmt.Errorf("replace the architecture entry of %q: %w", model, err)
		}
		return line, foldReplaced, nil
	}
	add := architectureAddition(facts, entry)
	if add == "" {
		return entry, foldUnchanged, nil
	}
	text := strings.TrimRight(entry, " \t\r\n")
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	text += " " + add
	if _, err := tx.Exec(`UPDATE cells SET value_text=?, value_number=NULL, written_at=?
		WHERE column_id=? AND model_name=? AND source='hand'`, text, NowStamp(), archID, model); err != nil {
		return "", 0, fmt.Errorf("extend the architecture entry of %q: %w", model, err)
	}
	return text, foldExtended, nil
}

// moveGGUFTotalToParameters carries each parameter count stored under
// gguf_total_bytes into Parameters, in billions, where the model has no
// Parameters cell from any source. It answers how many it wrote.
func moveGGUFTotalToParameters(tx *sql.Tx) (int, error) {
	var totalID int64
	err := tx.QueryRow(`SELECT id FROM columns WHERE column_key='gguf_total_bytes'`).Scan(&totalID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find the gguf_total_bytes column: %w", err)
	}
	rows, err := tx.Query(`SELECT c.model_name, c.value_text FROM cells c
		LEFT JOIN column_sources cs ON cs.column_id = c.column_id AND cs.source = c.source
		WHERE c.column_id=? ORDER BY c.model_name, COALESCE(cs.rank, 1000000), c.source`, totalID)
	if err != nil {
		return 0, fmt.Errorf("read the GGUF totals: %w", err)
	}
	type total struct {
		model string
		count float64
	}
	var totals []total
	seen := map[string]bool{}
	for rows.Next() {
		var model, text string
		if err := rows.Scan(&model, &text); err != nil {
			rows.Close()
			return 0, err
		}
		if seen[model] {
			continue
		}
		seen[model] = true
		n, perr := shared.ParseNumber(strings.TrimSpace(text))
		if perr != nil {
			continue // a value that is not a number has no count to move
		}
		totals = append(totals, total{model, n})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if len(totals) == 0 {
		return 0, nil
	}

	var paramsID int64
	err = tx.QueryRow(`SELECT k.id FROM columns k JOIN column_sources cs ON cs.column_id=k.id AND cs.source='huggingface'
		WHERE k.column_key='parameters'`).Scan(&paramsID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%d GGUF total(s) hold a parameter count, and there is no Parameters column that takes a huggingface value to move them into", len(totals))
	}
	if err != nil {
		return 0, fmt.Errorf("find the Parameters column: %w", err)
	}
	moved := 0
	for _, t := range totals {
		var has int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=? AND model_name=?`, paramsID, t.model).Scan(&has); err != nil {
			return 0, fmt.Errorf("check the parameters of %q: %w", t.model, err)
		}
		if has > 0 {
			continue
		}
		text := shared.TwoDecimals(strconv.FormatFloat(t.count/1e9, 'f', -1, 64))
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, fmt.Errorf("read the moved parameter count %q of %q: %w", text, t.model, err)
		}
		if _, err := tx.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
			VALUES (?,?,'huggingface',?,?,?)`, paramsID, t.model, text, number, NowStamp()); err != nil {
			return 0, fmt.Errorf("move the parameter count of %q: %w", t.model, err)
		}
		moved++
	}
	return moved, nil
}

// checkArchitectureFold is the migration's own refusal: every count it expects
// is proved against the database before the transaction may commit.
func checkArchitectureFold(tx *sql.Tx, archID int64, archBefore, cellsBefore, modelsBefore, doomedCells int, o foldOutcome, moved int, before, written map[string]string) error {
	var cellsAfter, archAfter, modelsAfter int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells`).Scan(&cellsAfter); err != nil {
		return fmt.Errorf("count the cells after folding: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=?`, archID).Scan(&archAfter); err != nil {
		return fmt.Errorf("count the architecture cells after folding: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM models`).Scan(&modelsAfter); err != nil {
		return fmt.Errorf("count the models after folding: %w", err)
	}
	if want, got := doomedCells-o.created-moved, cellsBefore-cellsAfter; want != got {
		return fmt.Errorf("refusing to commit: folding should have dropped %d cell(s) (%d on the deleted columns less %d architecture and %d parameter cell(s) created), and %d went",
			want, doomedCells, o.created, moved, got)
	}
	if rise := archAfter - archBefore; rise != o.created {
		return fmt.Errorf("refusing to commit: the architecture cells should have risen by %d, and rose by %d", o.created, rise)
	}
	if modelsAfter != modelsBefore {
		return fmt.Errorf("refusing to commit: the model count went from %d to %d", modelsBefore, modelsAfter)
	}
	after, err := architectureEntries(tx, archID)
	if err != nil {
		return err
	}
	for model, old := range before {
		if strings.EqualFold(strings.TrimSpace(old), "unknown") || strings.TrimSpace(old) == "" {
			continue
		}
		if !strings.HasPrefix(after[model], old) {
			return fmt.Errorf("refusing to commit: the entry of %q no longer begins with the words that were there", model)
		}
	}
	for model, text := range written {
		if after[model] != text {
			return fmt.Errorf("refusing to commit: the architecture cell of %q reads back different from what was written", model)
		}
	}
	return nil
}
