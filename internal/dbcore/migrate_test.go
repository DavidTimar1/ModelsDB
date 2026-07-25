package dbcore

import (
	"database/sql"
	"fmt"
	"modelsdb/internal/paths"
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB points the global path vars at a temp dir and opens a fresh DB
// connection (base schema, no migration) for migration/backup tests.
func setupTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := openConn(paths.DbFile); err != nil {
		t.Fatalf("openConn: %v", err)
	}
	t.Cleanup(func() {
		if DB != nil {
			DB.Close()
		}
	})
}

func TestMigrateFreshSetsVersion(t *testing.T) {
	setupTestDB(t)
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	v, err := currentSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	// Idempotent: a second migrate is a no-op and must not error.
	if err := migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateBacksUpNonEmptyDB(t *testing.T) {
	setupTestDB(t)
	// A pre-versioning DB: user_version stays 0 and it already holds data.
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('x','manual','t')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(dbBackups()) == 0 {
		t.Fatal("expected a pre-migration backup for a non-empty DB, found none")
	}
	if v, _ := currentSchemaVersion(); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
}

func TestRestoreFromBackupRoundTrips(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('a','manual','t')`); err != nil {
		t.Fatal(err)
	}
	bp, err := backupDB(0)
	if err != nil {
		t.Fatalf("backupDB: %v", err)
	}
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES ('b','manual','t')`); err != nil {
		t.Fatal(err)
	}
	if n, _ := CountModels(); n != 2 {
		t.Fatalf("pre-restore count = %d, want 2", n)
	}
	if err := restoreFromBackup(bp); err != nil {
		t.Fatalf("restoreFromBackup: %v", err)
	}
	if n, _ := CountModels(); n != 1 {
		t.Fatalf("post-restore count = %d, want 1 (backup had only 'a')", n)
	}
}

// TestMigrateCarriesSpeedRatingOCRIntoCustomColumns pins a fresh DB's full
// migration chain (v0 through the current schemaVersion): every model's
// existing speed/rating/ocr_quality value (the legacy fixed personal columns)
// must land in the new custom-column system as three column definitions -
// "Speed", "Rating", "OCR" - with matching per-model values, an unset value
// (empty string / rating 0) must NOT produce a custom_values row (sparse
// storage), and by the end of the chain the legacy columns themselves must be
// GONE (v5 drops them right after v4 has read them - see both migrations' doc
// comments).
func TestMigrateCarriesSpeedRatingOCRIntoCustomColumns(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`INSERT INTO models (name, source, speed, rating, ocr_quality, updated_at) VALUES
		('Fast Model', 'manual', 'fast', 4, 'good', 't'),
		('Untouched Model', 'manual', '', 0, '', 't')`); err != nil {
		t.Fatal(err)
	}

	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if v, _ := currentSchemaVersion(); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}

	colID := func(name string) int64 {
		var id int64
		if err := DB.QueryRow("SELECT id FROM custom_columns WHERE name=?", name).Scan(&id); err != nil {
			t.Fatalf("custom column %q not created: %v", name, err)
		}
		return id
	}
	speedID, ratingID, ocrID := colID("Speed"), colID("Rating"), colID("OCR")

	assertType := func(id int64, want string) {
		var got string
		if err := DB.QueryRow("SELECT type FROM custom_columns WHERE id=?", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("column %d type = %q, want %q", id, got, want)
		}
	}
	assertType(speedID, "dropdown_text")
	assertType(ratingID, "dropdown_number")
	assertType(ocrID, "dropdown_text")

	assertOptions := func(id int64, want ...string) {
		rows, err := DB.Query("SELECT value FROM custom_column_options WHERE column_id=? ORDER BY position ASC", id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var got []string
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			got = append(got, v)
		}
		if len(got) != len(want) {
			t.Fatalf("column %d options = %v, want %v", id, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("column %d options = %v, want %v", id, got, want)
			}
		}
	}
	assertOptions(speedID, "fast", "avg", "slow")
	assertOptions(ratingID, "1", "2", "3", "4")
	assertOptions(ocrID, "good", "bad")

	value := func(modelName string, columnID int64) (string, bool) {
		var v string
		err := DB.QueryRow("SELECT value FROM custom_values WHERE model_name=? AND column_id=?", modelName, columnID).Scan(&v)
		if err == sql.ErrNoRows {
			return "", false
		}
		if err != nil {
			t.Fatal(err)
		}
		return v, true
	}

	if v, ok := value("Fast Model", speedID); !ok || v != "fast" {
		t.Errorf("Fast Model speed carried = %q, ok=%v, want \"fast\"", v, ok)
	}
	if v, ok := value("Fast Model", ratingID); !ok || v != "4" {
		t.Errorf("Fast Model rating carried = %q, ok=%v, want \"4\"", v, ok)
	}
	if v, ok := value("Fast Model", ocrID); !ok || v != "good" {
		t.Errorf("Fast Model ocr_quality carried = %q, ok=%v, want \"good\"", v, ok)
	}

	if _, ok := value("Untouched Model", speedID); ok {
		t.Error("an unset legacy speed must not produce a custom_values row")
	}
	if _, ok := value("Untouched Model", ratingID); ok {
		t.Error("an unset legacy rating (0) must not produce a custom_values row")
	}
	if _, ok := value("Untouched Model", ocrID); ok {
		t.Error("an unset legacy ocr_quality must not produce a custom_values row")
	}

	// By the end of the chain (v5 ran right after v4), the legacy columns are
	// gone entirely - the contract step of expand/migrate/contract.
	rows2, err := DB.Query("PRAGMA table_info(models)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var cid, notnull, pk int
		var colName, typ string
		var dflt interface{}
		if err := rows2.Scan(&cid, &colName, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if colName == "speed" || colName == "rating" || colName == "ocr_quality" {
			t.Errorf("legacy column %q should have been dropped by the v5 migration", colName)
		}
	}

	// Idempotent: re-running migrate() (a no-op, since user_version is already
	// current) must not duplicate the column definitions or values.
	if err := migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := DB.QueryRow("SELECT COUNT(*) FROM custom_columns WHERE name IN ('Speed','Rating','OCR')").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("custom_columns count after re-migrate = %d, want 3 (no duplicates)", n)
	}
}

// TestMigrateFromV3CarriesPersonalFieldsWithBackup simulates the real-world
// upgrade path: every existing install's DB is already at schema v3 (the
// version shipped before this change), so migrate() only needs to run the new
// v4 step, not the full v1->v4 chain. This pins two things at once: the data
// carry works starting from that exact starting point, and - per this
// project's data-safety requirement that a live-data schema change must be
// verified to take a pre-migration backup, not assumed to - a backup of the
// non-empty DB is written before the migration touches it.
func TestMigrateFromV3CarriesPersonalFieldsWithBackup(t *testing.T) {
	setupTestDB(t)
	if _, err := DB.Exec(`PRAGMA user_version=3`); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO models (name, source, speed, rating, ocr_quality, updated_at) VALUES
		('Real User Model', 'openrouter', 'slow', 2, 'bad', 't')`); err != nil {
		t.Fatal(err)
	}
	if v, _ := currentSchemaVersion(); v != 3 {
		t.Fatalf("test setup: user_version = %d, want 3", v)
	}

	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if len(dbBackups()) == 0 {
		t.Fatal("a non-empty DB upgrading to v4 must be backed up first, found no backup")
	}
	if v, _ := currentSchemaVersion(); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}

	var speedID, ratingID, ocrID int64
	if err := DB.QueryRow("SELECT id FROM custom_columns WHERE name='Speed'").Scan(&speedID); err != nil {
		t.Fatalf("Speed column not created: %v", err)
	}
	if err := DB.QueryRow("SELECT id FROM custom_columns WHERE name='Rating'").Scan(&ratingID); err != nil {
		t.Fatalf("Rating column not created: %v", err)
	}
	if err := DB.QueryRow("SELECT id FROM custom_columns WHERE name='OCR'").Scan(&ocrID); err != nil {
		t.Fatalf("OCR column not created: %v", err)
	}

	check := func(columnID int64, want string) {
		var got string
		if err := DB.QueryRow("SELECT value FROM custom_values WHERE model_name='Real User Model' AND column_id=?", columnID).Scan(&got); err != nil {
			t.Fatalf("read carried value for column %d: %v", columnID, err)
		}
		if got != want {
			t.Errorf("carried value for column %d = %q, want %q", columnID, got, want)
		}
	}
	check(speedID, "slow")
	check(ratingID, "2")
	check(ocrID, "bad")

	// The real upgrade path every existing install takes (v3 -> v5 in one run)
	// must end with the legacy columns dropped too, not just the values carried.
	var probe string
	if err := DB.QueryRow("SELECT speed FROM models LIMIT 1").Scan(&probe); err == nil {
		t.Error("legacy column 'speed' should have been dropped by the v5 migration")
	}
}

func TestPruneBackupsKeepsCap(t *testing.T) {
	setupTestDB(t)
	if err := os.MkdirAll(paths.BackupsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < paths.MaxBackupsToKeep+5; i++ {
		f := filepath.Join(paths.BackupsDir, fmt.Sprintf("modelsdb-v0-x%04d.db", i))
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups()
	if got := len(dbBackups()); got != paths.MaxBackupsToKeep {
		t.Fatalf("after prune: %d backups, want %d", got, paths.MaxBackupsToKeep)
	}
}
