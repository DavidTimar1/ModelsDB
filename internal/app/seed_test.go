package app

import (
	"database/sql"
	"encoding/json"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// diskSize reads the REAL disk_size_gb column for a model (0 when NULL).
func diskSize(t *testing.T, name string) float64 {
	t.Helper()
	var v sql.NullFloat64
	if err := dbcore.DB.QueryRow("SELECT disk_size_gb FROM models WHERE name=?", name).Scan(&v); err != nil {
		t.Fatalf("read disk_size_gb of %q: %v", name, err)
	}
	return v.Float64
}

// durablePaths points paths.CuratedJsonFile at the test's temp data dir so the
// seed/export code reads and writes there, never the real data dir. migratedDB has
// already set paths.DataDir to a t.TempDir().
func durablePaths(t *testing.T) {
	t.Helper()
	paths.CuratedJsonFile = filepath.Join(paths.DataDir, "curated.json")
}

// TestPersonalDataStaysInDBNeverExported pins the model: personal data lives only
// in the database (the source of truth) and is NEVER written to a file. So an
// export writes curated.json but no personal.json, and curated.json carries no
// personal fields - the data survives in the DB regardless of any file.
func TestPersonalDataStaysInDBNeverExported(t *testing.T) {
	migratedDB(t)
	durablePaths(t)
	orig := shared.Version
	shared.Version = "999.0.0"
	defer func() { shared.Version = orig }()

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "M", "source": "huggingface", "hf_slug": "x/m",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	// Set personal data the way the UI save path does (writes to the DB only).
	if err := store.SaveCurated("M", map[string]interface{}{
		"notes": "my note", "rating": float64(4), "favorite": float64(1), "speed": "fast",
	}); err != nil {
		t.Fatal(err)
	}
	if got := colStr(t, "M", "notes"); got != "my note" {
		t.Fatalf("note not stored in DB: %q", got)
	}

	if err := store.ExportData(); err != nil {
		t.Fatalf("export: %v", err)
	}
	// No personal.json is ever written - personal data is not exported.
	if paths.FileExists(filepath.Join(paths.DataDir, "personal.json")) {
		t.Error("personal.json must never be written; personal data lives only in the DB")
	}
	// curated.json exists but must NOT contain the personal note.
	cur, err := os.ReadFile(paths.CuratedJsonFile)
	if err != nil {
		t.Fatalf("curated.json should exist: %v", err)
	}
	if strings.Contains(string(cur), "my note") {
		t.Error("curated.json must not contain personal data")
	}
}

// TestPopulatedDBKeepsDBAuthoritative pins the chosen policy: on a populated DB
// the DB is the source of truth - a DIFFERING curated.json restored into the data
// dir is NOT imported on startup, and ExportMissing never overwrites it.
func TestPopulatedDBKeepsDBAuthoritative(t *testing.T) {
	migratedDB(t)
	durablePaths(t)
	orig := shared.Version
	shared.Version = "999.0.0"
	defer func() { shared.Version = orig }()

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "A", "source": "huggingface", "hf_slug": "x/a",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	if err := store.ExportData(); err != nil {
		t.Fatal(err)
	}

	// Restore a DIFFERING curated.json that adds model B.
	doc := store.CuratedDoc{
		SchemaVersion: shared.CuratedSchemaVersion,
		Models: []map[string]interface{}{
			{"name": "B", "source": "huggingface", "hf_slug": "x/b", "parameters": float64(42)},
		},
	}
	rb, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(paths.CuratedJsonFile, rb, 0644); err != nil {
		t.Fatal(err)
	}

	// The startup path (create-if-missing) must NOT import the restored curated.json
	// onto the populated DB, nor overwrite the user's file.
	store.ExportMissing()
	if store.ModelExists("B") {
		t.Fatal("a curated.json restored onto a populated DB must NOT be imported (DB wins)")
	}
	cur, _ := os.ReadFile(paths.CuratedJsonFile)
	if string(cur) != string(rb) {
		t.Errorf("ExportMissing must not overwrite an existing curated.json")
	}
}

// TestExportMissingCreatesCuratedOnly checks fresh-install materialization: the
// missing curated.json is created, and no personal.json is ever produced.
func TestExportMissingCreatesCuratedOnly(t *testing.T) {
	migratedDB(t)
	durablePaths(t)
	orig := shared.Version
	shared.Version = "999.0.0"
	defer func() { shared.Version = orig }()

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "A", "source": "huggingface", "hf_slug": "x/a",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	if paths.FileExists(paths.CuratedJsonFile) {
		t.Fatal("curated.json should not exist before ExportMissing")
	}
	store.ExportMissing()
	if !paths.FileExists(paths.CuratedJsonFile) {
		t.Fatal("ExportMissing should create curated.json when absent")
	}
	if paths.FileExists(filepath.Join(paths.DataDir, "personal.json")) {
		t.Fatal("ExportMissing must never create personal.json")
	}
}

func writeCurated(t *testing.T, doc store.CuratedDoc) string {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "curated.json")
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportCuratedJSON(t *testing.T) {
	migratedDB(t)
	doc := store.CuratedDoc{
		SchemaVersion: shared.CuratedSchemaVersion,
		MinAppVersion: "", // empty -> always importable regardless of build version
		Models: []map[string]interface{}{
			{"name": "Qwen HF", "source": "huggingface", "model_type": "chat", "hf_slug": "Qwen/Q",
				"parameters": float64(27), "notes": "hfnote", "zdr": false,
				"input_modalities": []interface{}{"text"}, "output_modalities": []interface{}{"text"}},
			{"name": "OR Model", "source": "openrouter", "tool": "yes", "moe": "no", "parameters": float64(70)},
		},
	}
	if err := store.ImportCuratedJSON(writeCurated(t, doc)); err != nil {
		t.Fatalf("importCuratedJSON: %v", err)
	}

	// Non-API model is recreated in full, with curated fields and round-tripped ZDR.
	if !store.ModelExists("Qwen HF") {
		t.Fatal("HF model should be imported")
	}
	if got := colInt(t, "Qwen HF", "parameters"); got != 27 {
		t.Errorf("HF parameters = %d, want 27", got)
	}
	if got := colStr(t, "Qwen HF", "notes"); got != "hfnote" {
		t.Errorf("HF notes = %q", got)
	}
	if got := colInt(t, "Qwen HF", "zdr"); got != 0 {
		t.Errorf("explicit zdr:false should round-trip to 0, got %d", got)
	}

	// OpenRouter model gets a minimal row carrying its curated fields (metadata is
	// filled later by the live fetch).
	if !store.ModelExists("OR Model") {
		t.Fatal("OpenRouter model should get a minimal curated row")
	}
	if got := colStr(t, "OR Model", "tool"); got != "yes" {
		t.Errorf("OR tool = %q, want yes", got)
	}
	if got := colInt(t, "OR Model", "parameters"); got != 70 {
		t.Errorf("OR parameters = %d, want 70", got)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	migratedDB(t)
	// A HF model with objective curated data we expect to survive a full round-trip.
	rec := map[string]interface{}{
		"name": "Flux HF", "source": "huggingface", "hf_slug": "bfl/flux",
		"parameters": float64(13), "price_display": "$1/second", "measurement": "per second",
		"output_modalities": []interface{}{"image"},
	}
	if err := store.ImportFullRecord(rec, "huggingface", "image"); err != nil {
		t.Fatal(err)
	}

	// exportCuratedJSON stamps the header with CuratedMinAppVersion; force a high
	// build version so the re-import is never gated by the compatibility check.
	orig := shared.Version
	shared.Version = "999.0.0"
	defer func() { shared.Version = orig }()

	exportPath := filepath.Join(t.TempDir(), "exported.json")
	if err := store.ExportCuratedJSON(exportPath); err != nil {
		t.Fatalf("exportCuratedJSON: %v", err)
	}

	// Fresh DB, then rebuild from the exported file. Close the first connection
	// first so its temp file can be removed on Windows (setupTestDB's cleanup only
	// closes the latest global db handle, which would otherwise leak this one).
	if dbcore.DB != nil {
		dbcore.DB.Close()
	}
	migratedDB(t)
	if store.ModelExists("Flux HF") {
		t.Fatal("fresh DB should not have the model yet")
	}
	if err := store.ImportCuratedJSON(exportPath); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if !store.ModelExists("Flux HF") {
		t.Fatal("model should be restored from the exported catalog")
	}
	if got := colInt(t, "Flux HF", "parameters"); got != 13 {
		t.Errorf("parameters lost in round-trip: %d", got)
	}
	if got := colStr(t, "Flux HF", "measurement"); got != "per second" {
		t.Errorf("measurement lost in round-trip: %q", got)
	}
	if got := colStr(t, "Flux HF", "source"); got != "huggingface" {
		t.Errorf("source = %q", got)
	}
}

// TestDiskSizeRoundTripsAndSurvivesRefresh pins the curated disk_size_gb column:
// it survives an export -> fresh-DB import round-trip, and the OpenRouter refresh
// upsert never clobbers it (it is absent from that upsert's ON CONFLICT SET).
func TestDiskSizeRoundTripsAndSurvivesRefresh(t *testing.T) {
	migratedDB(t)
	durablePaths(t)
	orig := shared.Version
	shared.Version = "999.0.0"
	defer func() { shared.Version = orig }()

	// A HuggingFace-direct model carrying a curated on-disk size.
	rec := map[string]interface{}{
		"name": "Qwen: Sized HF", "source": "huggingface", "hf_slug": "Qwen/Sized",
		"parameters": float64(27), "disk_size_gb": float64(54.5),
		"output_modalities": []interface{}{"text"},
	}
	if err := store.ImportFullRecord(rec, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	if got := diskSize(t, "Qwen: Sized HF"); got != 54.5 {
		t.Fatalf("disk_size_gb not stored: %v", got)
	}

	// Export the objective catalog; the value must be present in the file.
	exportPath := filepath.Join(t.TempDir(), "exported.json")
	if err := store.ExportCuratedJSON(exportPath); err != nil {
		t.Fatalf("export: %v", err)
	}
	if b, _ := os.ReadFile(exportPath); !strings.Contains(string(b), "disk_size_gb") {
		t.Error("exported catalog should carry disk_size_gb")
	}

	// Rebuild a fresh DB from the export: disk_size_gb must round-trip.
	if dbcore.DB != nil {
		dbcore.DB.Close()
	}
	migratedDB(t)
	if err := store.ImportCuratedJSON(exportPath); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if got := diskSize(t, "Qwen: Sized HF"); got != 54.5 {
		t.Fatalf("disk_size_gb lost in round-trip: %v", got)
	}

	// An OpenRouter model with a curated disk size must keep it across a refresh.
	norm := map[string]interface{}{"name": "OR Sized", "id": "x/or-sized"}
	if err := store.UpsertAPIModel(norm); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCurated("OR Sized", map[string]interface{}{"disk_size_gb": float64(12.5)}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAPIModel(norm); err != nil {
		t.Fatal(err)
	}
	if got := diskSize(t, "OR Sized"); got != 12.5 {
		t.Fatalf("refresh clobbered disk_size_gb: %v", got)
	}
}

func TestCuratedCompatible(t *testing.T) {
	old := shared.Version
	defer func() { shared.Version = old }()

	// dev builds are never gated
	shared.Version = "dev"
	if err := store.CuratedCompatible("9.9.9"); err != nil {
		t.Errorf("dev build should bypass the gate, got %v", err)
	}

	// equal/newer builds may import; older builds are refused
	shared.Version = "0.1.0"
	if err := store.CuratedCompatible("0.1.0"); err != nil {
		t.Errorf("equal version should be compatible, got %v", err)
	}
	if err := store.CuratedCompatible(""); err != nil {
		t.Errorf("empty min version should be compatible, got %v", err)
	}
	if err := store.CuratedCompatible("0.2.0"); err == nil {
		t.Error("older build importing a newer catalog should be refused")
	}
}
