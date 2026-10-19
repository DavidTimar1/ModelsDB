package dbcore

import (
	"fmt"
	"strings"
	"testing"
)

const foldStamp = "2026-10-01T00:00:00Z"

// foldCatalog builds a v51 catalog holding the columns the fold reads and
// writes: Architecture (unsealed, hand), Parameters, and every column the
// migration deletes that a test names. It answers their ids by key.
func foldCatalog(t *testing.T, keys ...string) map[string]int64 {
	t.Helper()
	setupTestDB(t)
	migrateUpTo(t, 51)
	ids := map[string]int64{
		ArchitectureColumnKey: v45Column(t, ArchitectureColumnKey, freshLayout, true),
		"parameters":          v45Column(t, "parameters", freshLayout, false, "huggingface", "research"),
		"price_prompt":        v45Column(t, "price_prompt", freshLayout, false, "openrouter", "research"),
	}
	for _, k := range keys {
		ids[k] = v45Column(t, k, freshLayout, false, "huggingface", "research")
	}
	return ids
}

func putFacts(t *testing.T, ids map[string]int64, model string, facts map[string]string) {
	t.Helper()
	putModel(t, model)
	for k, v := range facts {
		putSourcedCell(t, ids[k], model, "huggingface", v, foldStamp)
	}
}

func architectureOf(t *testing.T, model string) (string, bool) {
	t.Helper()
	var text string
	err := DB.QueryRow(`SELECT value_text FROM cells WHERE column_id=(SELECT id FROM columns WHERE column_key=?) AND model_name=? AND source='hand'`,
		ArchitectureColumnKey, model).Scan(&text)
	if err != nil {
		return "", false
	}
	return text, true
}

func assertFolded(t *testing.T) {
	t.Helper()
	for _, key := range append(append([]string{}, architectureFoldKeys...), architectureDropKeys...) {
		var n int
		if err := DB.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key=?`, key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("column %s still exists after v52", key)
		}
	}
	if v, _ := currentSchemaVersion(); v != 52 {
		t.Errorf("user_version = %d, want 52", v)
	}
	assertSound(t)
}

var allFoldKeys = append(append([]string{}, architectureFoldKeys...), architectureDropKeys...)

func denseModel() map[string]string {
	return map[string]string{
		"hidden_layers": "40", "attention_layers": "40", "kv_heads": "8", "head_dim": "128",
		"hidden_size": "5120", "attention_heads": "32", "kv_bytes_per_token": "163840",
		"architecture_class": "Mistral3ForConditionalGeneration", "weight_dtype": "bfloat16",
		"weight_file_format": "safetensors", "declared_weight_bits": "16", "recurrent_layers": "0",
	}
}

// TestFoldCreatesALineForAModelWithNoEntry: the generated line states the
// measured facts, the column is sealed with hand declared, and the other
// columns are gone.
func TestFoldCreatesALineForAModelWithNoEntry(t *testing.T) {
	ids := foldCatalog(t, allFoldKeys...)
	putFacts(t, ids, "Dense", denseModel())
	putFacts(t, ids, "Water", map[string]string{"weight_bytes": "100", "build_label": "Q4_K_M"})
	before, _ := countCells()

	migrateUpTo(t, 52)

	assertFolded(t)
	want := "40 layers, all hold a KV cache; no recurrent state; head dim 128, 8 KV heads (hidden size 5,120, 32 attention heads); KV cache 163,840 bytes per token. Class Mistral3ForConditionalGeneration; weights bfloat16 in safetensors."
	if got, ok := architectureOf(t, "Dense"); !ok || got != want {
		t.Errorf("architecture = %q (present %v), want %q", got, ok, want)
	}
	if _, ok := architectureOf(t, "Water"); ok {
		t.Error("a model whose only facts were dropped columns got an architecture line")
	}
	// 12 folded cells and 2 dropped cells go, one line comes.
	if after, _ := countCells(); after != before-12-2+1 {
		t.Errorf("cells went from %d to %d, want 13 fewer cells and one new line", before, after)
	}
	var sealed int
	if err := DB.QueryRow(`SELECT sealed FROM columns WHERE column_key=?`, ArchitectureColumnKey).Scan(&sealed); err != nil || sealed != 1 {
		t.Errorf("the architecture column should be sealed (sealed=%d, err=%v)", sealed, err)
	}
	if got := sourcesByRank(t, ids[ArchitectureColumnKey]); fmt.Sprint(got) != "[hand]" {
		t.Errorf("architecture declares %v, want hand alone", got)
	}
}

// TestFoldReplacesAnUnknownAndKeepsRealText: "unknown" in any case is replaced,
// real text keeps every word and gains only what it lacks, and a complete entry
// is not touched.
func TestFoldReplacesAnUnknownAndKeepsRealText(t *testing.T) {
	ids := foldCatalog(t, allFoldKeys...)
	hand := ids[ArchitectureColumnKey]
	facts := func() map[string]string {
		f := denseModel()
		f["kv_bytes_per_token"] = "163840"
		return f
	}
	putFacts(t, ids, "Unknown", map[string]string{"kv_bytes_per_token": "65536", "declared_weight_bits": "10.5"})
	putSourcedCell(t, hand, "Unknown", "hand", " Unknown ", foldStamp)
	putFacts(t, ids, "Real", facts())
	putSourcedCell(t, hand, "Real", "hand", "40 layers, all hold a KV cache; KV cache 163,840 bytes per token. My own remark: parked", foldStamp)
	putFacts(t, ids, "Whole", facts())
	complete := "Hidden size 5120, 32 attention heads; Mistral3ForConditionalGeneration, bfloat16, safetensors."
	putSourcedCell(t, hand, "Whole", "hand", complete, foldStamp)

	migrateUpTo(t, 52)

	assertFolded(t)
	if got, _ := architectureOf(t, "Unknown"); got != "KV cache 65,536 bytes per token. 10.5 bits per weight." {
		t.Errorf("unknown became %q", got)
	}
	got, _ := architectureOf(t, "Real")
	old := "40 layers, all hold a KV cache; KV cache 163,840 bytes per token. My own remark: parked"
	if !strings.HasPrefix(got, old) {
		t.Errorf("the real entry lost its words: %q", got)
	}
	if !strings.HasSuffix(got, "Hidden size 5,120, 32 attention heads; class Mistral3ForConditionalGeneration; weights bfloat16 in safetensors.") {
		t.Errorf("the real entry gained %q", strings.TrimPrefix(got, old))
	}
	if got, _ := architectureOf(t, "Whole"); got != complete {
		t.Errorf("an entry that already holds every fact was changed to %q", got)
	}
}

// TestFoldLeavesOutTheCacheFiguresItCannotTrust: a sliding-window row and a
// Qwen3-Next row get layer counts, hidden size and heads, and no head dim, KV
// heads or cache figure.
func TestFoldLeavesOutTheCacheFiguresItCannotTrust(t *testing.T) {
	ids := foldCatalog(t, allFoldKeys...)
	putFacts(t, ids, "Sliding", map[string]string{
		"hidden_layers": "6", "attention_layers": "6", "kv_heads": "16", "head_dim": "256",
		"hidden_size": "5376", "attention_heads": "32", "kv_bytes_per_token": "983040",
		"layer_types": "sliding_attention, sliding_attention, sliding_attention, sliding_attention, sliding_attention, full_attention",
	})
	putFacts(t, ids, "Next", map[string]string{
		"hidden_layers": "48", "attention_layers": "48", "kv_heads": "2", "head_dim": "256",
		"hidden_size": "2048", "attention_heads": "16", "kv_bytes_per_token": "98304",
		"architecture_class": "Qwen3NextForCausalLM", "recurrent_layers": "36", "state_bytes_per_layer": "1097728",
	})

	migrateUpTo(t, 52)

	assertFolded(t)
	sliding, _ := architectureOf(t, "Sliding")
	if sliding != "6 layers (5 sliding_attention, 1 full_attention); no recurrent state; hidden size 5,376, 32 attention heads." {
		t.Errorf("sliding line = %q", sliding)
	}
	next, _ := architectureOf(t, "Next")
	for _, banned := range []string{"KV cache", "head dim", "KV heads", "hold a"} {
		if strings.Contains(next, banned) || strings.Contains(sliding, banned) {
			t.Errorf("a row with unsound cache figures says %q: %q / %q", banned, sliding, next)
		}
	}
	if !strings.Contains(next, "36 recurrent layers") || !strings.Contains(next, "recurrent state 1,097,728 bytes per recurrent layer") {
		t.Errorf("the Qwen3-Next line lost its recurrent facts: %q", next)
	}
}

// TestFoldMovesTheGGUFTotalIntoParametersInBillions: the count moves only where
// the model has no Parameters cell, divided by 1e9 and rounded to two places.
func TestFoldMovesTheGGUFTotalIntoParametersInBillions(t *testing.T) {
	ids := foldCatalog(t, allFoldKeys...)
	putFacts(t, ids, "Moved", map[string]string{"gguf_total_bytes": "16395544384"})
	putFacts(t, ids, "Small", map[string]string{"gguf_total_bytes": "234000287"})
	putFacts(t, ids, "Has", map[string]string{"gguf_total_bytes": "3256956928"})
	putSourcedCell(t, ids["parameters"], "Has", "research", "3.5", foldStamp)

	migrateUpTo(t, 52)

	assertFolded(t)
	for model, want := range map[string]string{"Moved": "16.4", "Small": "0.23"} {
		var text, source string
		if err := DB.QueryRow(`SELECT value_text, source FROM cells WHERE column_id=? AND model_name=?`, ids["parameters"], model).Scan(&text, &source); err != nil {
			t.Fatalf("%s has no Parameters cell: %v", model, err)
		}
		if text != want || source != "huggingface" {
			t.Errorf("%s parameters = %q from %s, want %q from huggingface", model, text, source, want)
		}
	}
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM cells WHERE column_id=? AND model_name='Has'`, ids["parameters"]).Scan(&n); err != nil || n != 1 {
		t.Errorf("a model that already had Parameters should keep exactly its own cell, found %d (err %v)", n, err)
	}
}

// TestFoldCreatesTheArchitectureColumnWhenTheCatalogHasNone: an install that
// imported models and never made the column gets it, sealed and hand-only.
func TestFoldCreatesTheArchitectureColumnWhenTheCatalogHasNone(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 51)
	ids := map[string]int64{}
	for _, k := range allFoldKeys {
		ids[k] = v45Column(t, k, freshLayout, false, "huggingface", "research")
	}
	putFacts(t, ids, "M", denseModel())

	migrateUpTo(t, 52)

	assertFolded(t)
	var sealed, visible int
	var label string
	if err := DB.QueryRow(`SELECT sealed, visible, label FROM columns WHERE column_key=?`, ArchitectureColumnKey).Scan(&sealed, &visible, &label); err != nil {
		t.Fatalf("the column was not created: %v", err)
	}
	if sealed != 1 || visible != 0 || label != ArchitectureColumnLabel {
		t.Errorf("created column: sealed=%d visible=%d label=%q", sealed, visible, label)
	}
	if text, ok := architectureOf(t, "M"); !ok || text == "" {
		t.Error("no line was written on the created column")
	}
}

// TestFoldIsANoOpWithoutTheColumns: a catalog that never had the columns keeps
// every cell, and an existing Architecture column is only sealed.
func TestFoldIsANoOpWithoutTheColumns(t *testing.T) {
	ids := foldCatalog(t)
	putModel(t, "M")
	putSourcedCell(t, ids[ArchitectureColumnKey], "M", "hand", "mine", foldStamp)
	putSourcedCell(t, ids["price_prompt"], "M", "openrouter", "0.1", foldStamp)
	before, _ := countCells()

	migrateUpTo(t, 52)

	assertFolded(t)
	if after, _ := countCells(); after != before {
		t.Errorf("cells went from %d to %d, want level", before, after)
	}
	if text, _ := architectureOf(t, "M"); text != "mine" {
		t.Errorf("the entry became %q", text)
	}
	var sealed int
	if err := DB.QueryRow(`SELECT sealed FROM columns WHERE column_key=?`, ArchitectureColumnKey).Scan(&sealed); err != nil || sealed != 1 {
		t.Errorf("the existing column should be sealed (sealed=%d, err=%v)", sealed, err)
	}
}

// TestFoldOnAFreshDatabaseDoesNothing: the seed, not the migration, creates the
// column on a database with no models.
func TestFoldOnAFreshDatabaseDoesNothing(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 52)
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key=?`, ArchitectureColumnKey).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("the migration created a column on a fresh database; the seed does that")
	}
	assertSound(t)
}

// TestFoldRefusesAnUnexpectedDrop: a cell lost for a reason the migration does
// not account for aborts it rather than committing the loss.
func TestFoldRefusesAnUnexpectedDrop(t *testing.T) {
	ids := foldCatalog(t, allFoldKeys...)
	putFacts(t, ids, "M", denseModel())
	putSourcedCell(t, ids["price_prompt"], "M", "openrouter", "0.1", foldStamp)
	trigger := fmt.Sprintf(`CREATE TRIGGER surprise_v52 AFTER DELETE ON columns WHEN old.column_key='head_dim'
		BEGIN DELETE FROM cells WHERE column_id=%d; END`, ids["price_prompt"])
	if _, err := DB.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	before, _ := countCells()

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := theArchitectureColumnHoldsWhatTheHuggingFaceColumnsHeld(tx); err == nil {
		t.Fatal("expected the migration to refuse a drop it cannot account for, got no error")
	}
	tx.Rollback()

	if after, _ := countCells(); after != before {
		t.Errorf("cells after the refused, rolled-back migration = %d, want %d", after, before)
	}
}

// TestFoldRefusesANonTextArchitectureColumn: the line is text, so a column under
// that key holding anything else stops the migration before it writes.
func TestFoldRefusesANonTextArchitectureColumn(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 51)
	if _, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at) VALUES (?, 'Architecture', 'number', ?)`, ArchitectureColumnKey, NowStamp()); err != nil {
		t.Fatal(err)
	}
	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := theArchitectureColumnHoldsWhatTheHuggingFaceColumnsHeld(tx); err == nil {
		t.Fatal("expected a refusal for a number column under the architecture key")
	}
}

// TestArchitectureLineWords pins the wording of one line per shape, written
// against values the test declares itself.
func TestArchitectureLineWords(t *testing.T) {
	cases := []struct {
		name  string
		facts archFacts
		want  string
	}{
		{"hybrid", archFacts{
			"hidden_layers": "40", "attention_layers": "10", "kv_heads": "2", "head_dim": "256", "hidden_size": "2048",
			"attention_heads": "16", "recurrent_layers": "30", "kv_bytes_per_token": "20480",
			"layer_types":        strings.Repeat("linear_attention, linear_attention, linear_attention, full_attention, ", 9) + "linear_attention, linear_attention, linear_attention, full_attention",
			"architecture_class": "Qwen3_5MoeForConditionalGeneration", "weight_dtype": "bfloat16", "weight_file_format": "safetensors",
		}, "40 layers (30 linear_attention, 10 full_attention), 10 hold a KV cache; 30 recurrent layers; head dim 256, 2 KV heads (hidden size 2,048, 16 attention heads); KV cache 20,480 bytes per token. Class Qwen3_5MoeForConditionalGeneration; weights bfloat16 in safetensors."},
		{"bits only below 16", archFacts{"declared_weight_bits": "16", "weight_dtype": "bfloat16"}, "Weights bfloat16."},
		{"quantized", archFacts{"quantization_method": "awq", "declared_weight_bits": "4"}, "Quantization awq, 4 bits per weight."},
		{"no layer count", archFacts{"attention_layers": "12", "kv_bytes_per_token": "1000"}, "12 layers hold a KV cache; no recurrent state; KV cache 1,000 bytes per token."},
		{"nothing", archFacts{"state_bytes_per_layer": "5"}, ""},
	}
	for _, c := range cases {
		if got := architectureLine(c.facts); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestHasWordDoesNotFindFloat16InsideBfloat16: the test for a fact an entry
// already states is a whole-word one.
func TestHasWordDoesNotFindFloat16InsideBfloat16(t *testing.T) {
	if hasWord("weights bfloat16 in safetensors", "float16") {
		t.Error("float16 was found inside bfloat16")
	}
	if !hasWord("Weights float16.", "FLOAT16") {
		t.Error("float16 was not found as a whole word, ignoring case")
	}
}
