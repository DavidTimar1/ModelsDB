package dbcore

import (
	"fmt"
	"testing"
)

// pricingNoteCatalog builds a v50 catalog with the notes and pricing_note
// columns, both declaring hand, and a survivor column that must not move.
func pricingNoteCatalog(t *testing.T) (notesID, pricingID, survivorID int64) {
	t.Helper()
	setupTestDB(t)
	migrateUpTo(t, 50)
	notesID = v45Column(t, "notes", freshLayout, true)
	pricingID = v45Column(t, "pricing_note", freshLayout, true)
	survivorID = v45Column(t, "price_prompt", freshLayout, false, "openrouter", "research")
	return notesID, pricingID, survivorID
}

func noteText(t *testing.T, notesID int64, model string) (string, int) {
	t.Helper()
	rows, err := DB.Query(`SELECT COALESCE(value_text,'') FROM cells WHERE column_id=? AND model_name=? AND source='hand'`, notesID, model)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var text string
	n := 0
	for rows.Next() {
		n++
		if err := rows.Scan(&text); err != nil {
			t.Fatal(err)
		}
	}
	return text, n
}

func assertSound(t *testing.T) {
	t.Helper()
	if err := integrityCheck(); err != nil {
		t.Errorf("integrity_check: %v", err)
	}
	rows, err := DB.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("foreign_key_check reports a row")
	}
}

func assertPricingNoteGone(t *testing.T) {
	t.Helper()
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key='pricing_note'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("pricing_note column still exists after v51")
	}
	if v, _ := currentSchemaVersion(); v != 51 {
		t.Errorf("user_version = %d, want 51", v)
	}
}

// TestPricingNoteRetiredWithNoPricingNotes: an empty pricing_note column goes,
// no note is touched, and no cell is lost beyond the column's own.
func TestPricingNoteRetiredWithNoPricingNotes(t *testing.T) {
	notesID, _, survivorID := pricingNoteCatalog(t)
	putModel(t, "M")
	putSourcedCell(t, notesID, "M", "hand", "mine", "2026-10-01T00:00:00Z")
	putSourcedCell(t, survivorID, "M", "openrouter", "0.1", "2026-10-01T00:00:00Z")
	before, _ := countCells()

	migrateUpTo(t, 51)

	assertPricingNoteGone(t)
	assertSound(t)
	if after, _ := countCells(); after != before {
		t.Errorf("cells went from %d to %d, want level", before, after)
	}
	if text, _ := noteText(t, notesID, "M"); text != "mine" {
		t.Errorf("note = %q, want it untouched", text)
	}
}

// TestPricingNoteMovesIntoANewNote: a model with a pricing note and no Note
// gets a hand Note holding that text.
func TestPricingNoteMovesIntoANewNote(t *testing.T) {
	notesID, pricingID, _ := pricingNoteCatalog(t)
	putModel(t, "M")
	putSourcedCell(t, pricingID, "M", "hand", "Video: $0.08/second", "2026-10-01T00:00:00Z")
	before, _ := countCells()

	migrateUpTo(t, 51)

	assertPricingNoteGone(t)
	assertSound(t)
	if text, n := noteText(t, notesID, "M"); n != 1 || text != "Video: $0.08/second" {
		t.Errorf("note = %q in %d cell(s), want the pricing note in one new cell", text, n)
	}
	if after, _ := countCells(); after != before {
		t.Errorf("cells went from %d to %d, want level (one moved)", before, after)
	}
}

// TestPricingNoteIsAppendedToAnExistingNote: both texts are kept, the note
// first, a newline between.
func TestPricingNoteIsAppendedToAnExistingNote(t *testing.T) {
	notesID, pricingID, _ := pricingNoteCatalog(t)
	putModel(t, "M")
	putSourcedCell(t, notesID, "M", "hand", "reads code well", "2026-10-01T00:00:00Z")
	putSourcedCell(t, pricingID, "M", "hand", "Batch: half price", "2026-10-01T00:00:00Z")
	before, _ := countCells()

	migrateUpTo(t, 51)

	assertPricingNoteGone(t)
	assertSound(t)
	if text, n := noteText(t, notesID, "M"); n != 1 || text != "reads code well\nBatch: half price" {
		t.Errorf("note = %q in %d cell(s), want both texts joined by a newline", text, n)
	}
	if after, _ := countCells(); after != before-1 {
		t.Errorf("cells went from %d to %d, want exactly the pricing cell gone", before, after)
	}
}

// TestPricingNoteIdenticalToTheNoteIsNotDuplicated: a note that already reads
// the same text is left exactly as it was.
func TestPricingNoteIdenticalToTheNoteIsNotDuplicated(t *testing.T) {
	notesID, pricingID, _ := pricingNoteCatalog(t)
	putModel(t, "M")
	putSourcedCell(t, notesID, "M", "hand", "per second", "2026-10-01T00:00:00Z")
	putSourcedCell(t, pricingID, "M", "hand", " per second ", "2026-10-01T00:00:00Z")

	migrateUpTo(t, 51)

	assertPricingNoteGone(t)
	assertSound(t)
	if text, _ := noteText(t, notesID, "M"); text != "per second" {
		t.Errorf("note = %q, want it unchanged", text)
	}
	var stamp string
	if err := DB.QueryRow(`SELECT written_at FROM cells WHERE column_id=? AND model_name='M'`, notesID).Scan(&stamp); err != nil {
		t.Fatal(err)
	}
	if stamp != "2026-10-01T00:00:00Z" {
		t.Errorf("an identical note was rewritten (written_at %s)", stamp)
	}
}

// TestPricingNoteBlankCellsAreDroppedAndBlankNoteIsReplaced: a blank pricing
// note carries nothing, and a blank Note takes the pricing note as its text.
func TestPricingNoteBlankCellsAreDroppedAndBlankNoteIsReplaced(t *testing.T) {
	notesID, pricingID, _ := pricingNoteCatalog(t)
	putModel(t, "Blank")
	putModel(t, "Empty")
	putSourcedCell(t, pricingID, "Blank", "hand", "   ", "2026-10-01T00:00:00Z")
	putSourcedCell(t, notesID, "Empty", "hand", "  ", "2026-10-01T00:00:00Z")
	putSourcedCell(t, pricingID, "Empty", "hand", "images extra", "2026-10-01T00:00:00Z")

	migrateUpTo(t, 51)

	assertPricingNoteGone(t)
	assertSound(t)
	if _, n := noteText(t, notesID, "Blank"); n != 0 {
		t.Errorf("a blank pricing note created a note cell")
	}
	if text, n := noteText(t, notesID, "Empty"); n != 1 || text != "images extra" {
		t.Errorf("note = %q in %d cell(s), want the pricing note in place of the blank one", text, n)
	}
}

// TestPricingNoteMigrationIsANoOpWithoutTheColumn: a catalog that never had
// the column keeps every cell and lands on 51.
func TestPricingNoteMigrationIsANoOpWithoutTheColumn(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 50)
	survivorID := v45Column(t, "price_prompt", freshLayout, false, "openrouter", "research")
	putModel(t, "M")
	putSourcedCell(t, survivorID, "M", "openrouter", "0.1", "2026-10-01T00:00:00Z")
	before, _ := countCells()

	migrateUpTo(t, 51)

	assertPricingNoteGone(t)
	if after, _ := countCells(); after != before {
		t.Errorf("cells went from %d to %d, want level", before, after)
	}
}

// TestPricingNoteRefusesAnUnexpectedDrop: a cell lost for a reason the
// migration does not account for aborts it rather than committing the loss.
func TestPricingNoteRefusesAnUnexpectedDrop(t *testing.T) {
	_, pricingID, survivorID := pricingNoteCatalog(t)
	putModel(t, "M")
	putSourcedCell(t, pricingID, "M", "hand", "caveat", "2026-10-01T00:00:00Z")
	putSourcedCell(t, survivorID, "M", "openrouter", "0.1", "2026-10-01T00:00:00Z")
	trigger := fmt.Sprintf(`CREATE TRIGGER surprise_v51 AFTER DELETE ON columns WHEN old.column_key='pricing_note'
		BEGIN DELETE FROM cells WHERE column_id=%d; END`, survivorID)
	if _, err := DB.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	before, _ := countCells()

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := thePricingNoteColumnIsRetired(tx); err == nil {
		t.Fatal("expected the migration to refuse a drop it cannot account for, got no error")
	}
	tx.Rollback()

	if after, _ := countCells(); after != before {
		t.Errorf("cells after the refused, rolled-back migration = %d, want %d", after, before)
	}
}
