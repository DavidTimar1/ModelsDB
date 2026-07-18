package app

import (
	"database/sql"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"path/filepath"
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

func colStr(t *testing.T, name, col string) string {
	t.Helper()
	var s string
	if err := dbcore.DB.QueryRow("SELECT "+col+" FROM models WHERE name=?", name).Scan(&s); err != nil {
		t.Fatalf("read %s.%s: %v", name, col, err)
	}
	return s
}

func colInt(t *testing.T, name, col string) int64 {
	t.Helper()
	var v sql.NullInt64
	if err := dbcore.DB.QueryRow("SELECT "+col+" FROM models WHERE name=?", name).Scan(&v); err != nil {
		t.Fatalf("read %s.%s: %v", name, col, err)
	}
	return v.Int64
}

func TestUpsertAPIModelInsertAndPreserveCurated(t *testing.T) {
	migratedDB(t)
	norm := catalog.NormalizeModel(map[string]interface{}{
		"name": "OpenAI: GPT-5", "id": "openai/gpt-5", "description": "first",
		"architecture":         map[string]interface{}{"output_modalities": []interface{}{"text"}},
		"supported_parameters": []interface{}{"tools"},
		"pricing":              map[string]interface{}{"prompt": "0.000003", "completion": "0.000015"},
	})
	if err := store.UpsertAPIModel(norm); err != nil {
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
	if n, ok := store.FindNameByID("openai/gpt-5"); !ok || n != "OpenAI: GPT-5" {
		t.Errorf("findNameByID = %q,%v", n, ok)
	}

	// Lay down curated values, then re-upsert with changed metadata: the curated
	// columns must survive while metadata refreshes.
	if err := store.UpdateCurated("OpenAI: GPT-5", map[string]interface{}{"rating": float64(5), "notes": "great"}); err != nil {
		t.Fatalf("updateCurated: %v", err)
	}
	norm2 := catalog.NormalizeModel(map[string]interface{}{
		"name": "OpenAI: GPT-5", "id": "openai/gpt-5", "description": "second",
		"architecture": map[string]interface{}{"output_modalities": []interface{}{"text"}},
		"pricing":      map[string]interface{}{"prompt": "0.000003", "completion": "0.000015"},
	})
	if err := store.UpsertAPIModel(norm2); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if got := colStr(t, "OpenAI: GPT-5", "description"); got != "second" {
		t.Errorf("description should refresh on re-upsert, got %q", got)
	}
	if got := colInt(t, "OpenAI: GPT-5", "rating"); got != 5 {
		t.Errorf("curated rating must survive re-upsert, got %d", got)
	}
	if got := colStr(t, "OpenAI: GPT-5", "notes"); got != "great" {
		t.Errorf("curated notes must survive re-upsert, got %q", got)
	}
}

func TestImportFullRecord(t *testing.T) {
	migratedDB(t)
	rec := map[string]interface{}{
		"name": "Qwen: Qwen3 HF", "source": "huggingface", "hf_slug": "Qwen/Qwen3",
		"author": "Qwen", "description": "MoE model", "parameters": float64(235),
		"active_parameters": float64(22), "moe": "yes", "notes": "hf note",
		"price_display": "HF-only", "measurement": "per second",
		"input_modalities": []interface{}{"text"}, "output_modalities": []interface{}{"text"},
	}
	if err := store.ImportFullRecord(rec, "huggingface", "chat"); err != nil {
		t.Fatalf("importFullRecord: %v", err)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "source"); got != "huggingface" {
		t.Errorf("source = %q", got)
	}
	if got := colInt(t, "Qwen: Qwen3 HF", "parameters"); got != 235 {
		t.Errorf("parameters = %d, want 235", got)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "notes"); got != "hf note" {
		t.Errorf("notes = %q", got)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "measurement"); got != "per second" {
		t.Errorf("measurement = %q", got)
	}
}

func TestSetPricingPreservesUnitOnEmpty(t *testing.T) {
	migratedDB(t)
	// Minimal row via updateCurated, then price it.
	if err := store.UpdateCurated("Veo", map[string]interface{}{}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPricing("Veo", "", "0.10", "", "Video: $0.10/second", "https://openrouter.ai/veo", "per second"); err != nil {
		t.Fatalf("setPricing: %v", err)
	}
	if got := colStr(t, "Veo", "price_completion"); got != "0.10" {
		t.Errorf("price_completion = %q", got)
	}
	if got := colStr(t, "Veo", "measurement"); got != "per second" {
		t.Errorf("measurement = %q", got)
	}
	// A re-price with an empty measurement must NOT clear the existing unit.
	if err := store.SetPricing("Veo", "", "0.12", "", "Video: $0.12/second", "https://openrouter.ai/veo", ""); err != nil {
		t.Fatalf("re-setPricing: %v", err)
	}
	if got := colStr(t, "Veo", "measurement"); got != "per second" {
		t.Errorf("empty measurement should preserve prior unit, got %q", got)
	}
}

func TestSaveCuratedPreservesOmittedFields(t *testing.T) {
	migratedDB(t)
	if err := store.UpdateCurated("M", map[string]interface{}{"notes": "keep me", "rating": float64(3)}); err != nil {
		t.Fatal(err)
	}
	// Save only a new rating; notes must be preserved (read-modify-write merge).
	if err := store.SaveCurated("M", map[string]interface{}{"rating": float64(5)}); err != nil {
		t.Fatalf("saveCurated: %v", err)
	}
	if got := colInt(t, "M", "rating"); got != 5 {
		t.Errorf("rating = %d, want 5", got)
	}
	if got := colStr(t, "M", "notes"); got != "keep me" {
		t.Errorf("omitted notes must be preserved, got %q", got)
	}
}
