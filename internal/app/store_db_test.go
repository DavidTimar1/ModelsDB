package app

import (
	"errors"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// migratedDB gives each test a hermetic, fully-migrated temp SQLite DB via the
// exported dbcore.OpenDB (open + base schema + migrations). Nothing here touches
// the real data dir.
func migratedDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := dbcore.OpenDB(paths.DbFile); err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() {
		if dbcore.DB != nil {
			dbcore.DB.Close()
		}
	})
}

// cellValue answers what one column holds for one model, by the column's own
// key. It is what a test asks when the question is about a particular column
// rather than about what the catalog says.
func cellValue(t *testing.T, model, key string) string {
	t.Helper()
	cells, err := store.ModelCells(model)
	if err != nil {
		t.Fatalf("read the cells of %q: %v", model, err)
	}
	return cells[key]
}

// colStr answers what one model's field reads as, wherever it is stored now.
//
// A model row holds its IDENTITY and nothing else, so what it still carries is
// read off the row and every FACT is read out of the cell store. Three assertions
// still go by the short names they were first written under, which colStrAliases
// maps to the column keys.
func colStr(t *testing.T, name, col string) string {
	t.Helper()
	switch col {
	case "name", "source", "variant_of":
		var s string
		if err := dbcore.DB.QueryRow("SELECT "+col+" FROM models WHERE name=?", name).Scan(&s); err != nil {
			t.Fatalf("read %s.%s: %v", name, col, err)
		}
		return s
	case "hidden":
		var n int
		if err := dbcore.DB.QueryRow("SELECT hidden FROM models WHERE name=?", name).Scan(&n); err != nil {
			t.Fatalf("read %s.hidden: %v", name, err)
		}
		return strconv.Itoa(n)
	}
	if key, aliased := colStrAliases[col]; aliased {
		col = key
	}
	value := cellValue(t, name, col)
	if col == "favorite" || col == "supports_reasoning" {
		// These two assertions read a flag as 1 or 0.
		switch strings.ToLower(value) {
		case "yes", "true", "1":
			return "1"
		}
		return "0"
	}
	return value
}

// colStrAliases maps the short names some assertions use to the column keys.
var colStrAliases = map[string]string{"tool": "supports_tools", "hf_slug": "repository", "measurement": "price_unit"}

func colInt(t *testing.T, name, col string) int64 {
	t.Helper()
	figure, _ := strconv.ParseFloat(strings.TrimSpace(colStr(t, name, col)), 64)
	return int64(figure)
}

func TestUpsertAPIModelInsertAndPreserveCurated(t *testing.T) {
	migratedDB(t)
	norm := catalog.NormalizeModel(map[string]interface{}{
		"name": "OpenAI: GPT-5", "id": "openai/gpt-5", "description": "first",
		"architecture":         map[string]interface{}{"output_modalities": []interface{}{"text"}},
		"supported_parameters": []interface{}{"tools"},
		"pricing":              map[string]interface{}{"prompt": "0.000003", "completion": "0.000015"},
	})
	if _, err := store.UpsertAPIModel(norm); err != nil {
		t.Fatalf("upsertAPIModel: %v", err)
	}

	if !store.ModelExists("OpenAI: GPT-5") {
		t.Fatal("model should exist after upsert")
	}
	if got := colStr(t, "OpenAI: GPT-5", "model_type"); got != "chat" {
		t.Errorf("model_type = %q, want chat (from text output)", got)
	}
	if got := colStr(t, "OpenAI: GPT-5", "price_prompt"); got != "0.000003" {
		t.Errorf("price_prompt = %q", got)
	}
	if got := colStr(t, "OpenAI: GPT-5", "tool"); got != "yes" {
		t.Errorf("tool = %q, want yes (supports_tool_parameters)", got)
	}
	if got := colStr(t, "OpenAI: GPT-5", "measurement"); got != "per 1M tokens" {
		t.Errorf("measurement seed = %q", got)
	}
	if n, ok, err := store.FindNameByCell([]string{"id"}, "openai/gpt-5", false); err != nil || !ok || n != "OpenAI: GPT-5" {
		t.Errorf("FindNameByCell = %q,%v,%v", n, ok, err)
	}

	// Lay down a note and a researched value in a column the feed did not
	// state, then re-upsert with changed metadata: both survive while the
	// metadata refreshes, and a column the feed states cannot be typed over.
	if err := store.SaveCells("OpenAI: GPT-5", map[string]string{"expiration_date": "2027-01-01", "notes": "great"}); err != nil {
		t.Fatalf("SaveCells: %v", err)
	}
	if err := store.SaveCells("OpenAI: GPT-5", map[string]string{"supports_tools": "no"}); !errors.Is(err, store.ErrNotWritable) {
		t.Errorf("typing over the feed's tool answer: err = %v, want ErrNotWritable", err)
	}
	norm2 := catalog.NormalizeModel(map[string]interface{}{
		"name": "OpenAI: GPT-5", "id": "openai/gpt-5", "description": "second",
		"architecture": map[string]interface{}{"output_modalities": []interface{}{"text"}},
		"pricing":      map[string]interface{}{"prompt": "0.000003", "completion": "0.000015"},
	})
	if _, err := store.UpsertAPIModel(norm2); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if got := colStr(t, "OpenAI: GPT-5", "description"); got != "second" {
		t.Errorf("description should refresh on re-upsert, got %q", got)
	}
	if got := colStr(t, "OpenAI: GPT-5", "tool"); got != "no" {
		t.Errorf("tool after a re-upsert that saw no tools = %q, want the feed's no", got)
	}
	if got := colStr(t, "OpenAI: GPT-5", "notes"); got != "great" {
		t.Errorf("the note must survive a re-upsert, got %q", got)
	}
	if got := cellValue(t, "OpenAI: GPT-5", "expiration_date"); got != "2027-01-01" {
		t.Errorf("a researched value in a column the feed never states must survive a re-upsert, got %q", got)
	}
}

func TestImportFullRecord(t *testing.T) {
	migratedDB(t)
	rec := map[string]interface{}{
		"name": "Qwen: Qwen3 HF", "source": "huggingface", "hf_slug": "Qwen/Qwen3",
		"description": "MoE model", "moe": "yes",
		"input_modalities": []interface{}{"text"}, "output_modalities": []interface{}{"text"},
	}
	if err := store.ImportFullRecord(rec, "huggingface", "chat"); err != nil {
		t.Fatalf("importFullRecord: %v", err)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "source"); got != "huggingface" {
		t.Errorf("source = %q", got)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "moe"); got != "yes" {
		t.Errorf("moe = %q, want yes", got)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "hf_slug"); got != "Qwen/Qwen3" {
		t.Errorf("hf_slug = %q", got)
	}
	// A record from a route that reads none is refused, and creates nothing.
	if err := store.ImportFullRecord(map[string]interface{}{"name": "Nobody: Reads This"}, "collection", "chat"); err == nil {
		t.Error("a record stamped with a source that reads no record was accepted")
	}
	if store.ModelExists("Nobody: Reads This") {
		t.Error("the refused record created a row")
	}
}

// TestASavePreservesOmittedFields: a save names only the cells it changes, and
// every other cell on the row stays as it was.
func TestASavePreservesOmittedFields(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("M", map[string]string{"notes": "keep me", "supports_tools": "no"}); err != nil {
		t.Fatal(err)
	}
	// Save only a new tool value (research over research); notes must be preserved.
	if err := store.SaveCells("M", map[string]string{"supports_tools": "yes"}); err != nil {
		t.Fatalf("saveCurated: %v", err)
	}
	if got := colStr(t, "M", "tool"); got != "yes" {
		t.Errorf("tool = %q, want yes", got)
	}
	if got := colStr(t, "M", "notes"); got != "keep me" {
		t.Errorf("omitted notes must be preserved, got %q", got)
	}
}
