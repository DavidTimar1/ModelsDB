package store

import (
	"errors"
	"modelsdb/internal/dbcore"
	"strings"
	"testing"
)

// TestAValueReadIsAValueThatCanBeWrittenBack is the promise the whole write side
// is built on: a cell answered by a read is named by the same key a write takes,
// so a value can be posted straight back with nothing looked up in between.
func TestAValueReadIsAValueThatCanBeWrittenBack(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if err := SaveCells("Qwen3 32B", map[string]string{"notes": "reads long diffs well"}); err != nil {
		t.Fatalf("SaveCells: %v", err)
	}
	stored := cellsOf(t, "Qwen3 32B")
	if err := SaveCells("Qwen3 32B", map[string]string{"notes": stored["notes"]}); err != nil {
		t.Fatalf("writing back what was read: %v", err)
	}
	if again := cellsOf(t, "Qwen3 32B"); again["notes"] != stored["notes"] {
		t.Errorf("the value changed on the way round: %q, was %q", again["notes"], stored["notes"])
	}
}

// TestASaveOnAnEmptyFetchedCellLandsAsResearch: a value nothing automatic has
// stated is not information from OpenRouter, so typing it does not edit
// fetched information. It lands as research, on a column OpenRouter states, a
// column a repository measures, and a price alike.
func TestASaveOnAnEmptyFetchedCellLandsAsResearch(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")

	for _, key := range []string{"context_length", "parameters", "price_prompt"} {
		if err := SaveCells("Qwen3 32B", map[string]string{key: "8192"}); err != nil {
			t.Errorf("%s: a write into the empty cell was refused: %v", key, err)
		}
	}
	held := cellsOf(t, "Qwen3 32B")
	for _, key := range []string{"context_length", "parameters", "price_prompt"} {
		if held[key] != "8192" {
			t.Errorf("%s holds %q, want the researched answer", key, held[key])
		}
		if src := sourceOfCell(t, key, "Qwen3 32B"); src != string(SourceResearch) {
			t.Errorf("%s was written as %q, want research", key, src)
		}
	}
}

// TestASaveOnAFetchedCellHoldingAnAutomaticValueIsRefusedNamingTheSource is
// the rule itself: what an update maintains is never typed over, and the
// refusal says whose answer stands and what it is.
func TestASaveOnAFetchedCellHoldingAnAutomaticValueIsRefusedNamingTheSource(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if err := WriteCells(SourceOpenRouter, "Qwen3 32B", map[string]string{"context_length": "128000"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceHuggingFace, "Qwen3 32B", map[string]string{"parameters": "32.76"}); err != nil {
		t.Fatal(err)
	}

	err := SaveCells("Qwen3 32B", map[string]string{"context_length": "131072"})
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
	want := "Context length holds OpenRouter's answer, 128000, which an update maintains; nothing typed replaces it"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("refusal = %q, want it to carry %q", err, want)
	}
	err = SaveCells("Qwen3 32B", map[string]string{"parameters": "70"})
	if !errors.Is(err, ErrNotWritable) || !strings.Contains(err.Error(), "HuggingFace's answer, 32.76") {
		t.Errorf("a measured value was not refused naming HuggingFace: %v", err)
	}
	held := cellsOf(t, "Qwen3 32B")
	if held["context_length"] != "128000" || held["parameters"] != "32.76" {
		t.Errorf("the automatic values moved: %v", held)
	}
	// A refused key leaves a good one in the same request unwritten too.
	err = SaveCells("Qwen3 32B", map[string]string{"notes": "kept?", "context_length": "1"})
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["notes"] != "" {
		t.Errorf("the note beside a refused key was written: %q", held["notes"])
	}
}

// TestASaveOnHisOwnColumnLandsAsHand: a note, a star and a
// column the reader made are theirs, written as hand whatever else is on the
// row, and a research write replaces or clears an earlier research cell the
// way a hand write always has.
func TestASaveOnHisOwnColumnLandsAsHand(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if _, err := CreateOwnColumn(ColumnSpec{Label: "Rating"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveCells("Qwen3 32B", map[string]string{
		"notes": "reads code well", "favorite": "yes", "rating": "good",
		"context_length": "8192",
	}); err != nil {
		t.Fatalf("SaveCells: %v", err)
	}
	for _, key := range []string{"notes", "favorite", "rating"} {
		if src := sourceOfCell(t, key, "Qwen3 32B"); src != string(SourceHand) {
			t.Errorf("%s was written as %q, want hand", key, src)
		}
	}
	if src := sourceOfCell(t, "context_length", "Qwen3 32B"); src != string(SourceResearch) {
		t.Errorf("context_length was written as %q, want research", src)
	}

	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": "16384"}); err != nil {
		t.Fatalf("replace the researched value: %v", err)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["context_length"] != "16384" {
		t.Errorf("context_length holds %q, want the replacement 16384", held["context_length"])
	}
	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": ""}); err != nil {
		t.Fatalf("clear the researched value: %v", err)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["context_length"] != "" {
		t.Errorf("context_length holds %q after clearing, want nothing", held["context_length"])
	}
}

// TestAnOvertakenResearchCellCanBeClearedButNotReplaced: once an automatic
// source states the fact, the researched value beneath it is stuck unless an
// empty value is accepted, so an empty value always is - it deletes the
// research cell and nothing else - while a non-empty value stays refused.
func TestAnOvertakenResearchCellCanBeClearedButNotReplaced(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": "8192"}); err != nil {
		t.Fatalf("research into the empty cell: %v", err)
	}
	if err := WriteCells(SourceOpenRouter, "Qwen3 32B", map[string]string{"context_length": "128000"}); err != nil {
		t.Fatal(err)
	}
	_, _, overtaken, err := AllProvenance()
	if err != nil {
		t.Fatal(err)
	}
	if !overtaken["Qwen3 32B"]["context_length"] {
		t.Errorf("the research cell beneath OpenRouter's is not reported as overtaken: %v", overtaken)
	}

	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": "9000"}); !errors.Is(err, ErrNotWritable) {
		t.Errorf("replacing the overtaken research: err = %v, want ErrNotWritable", err)
	}
	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": ""}); err != nil {
		t.Fatalf("clearing the overtaken research: %v", err)
	}
	var research int
	if err := dbcore.DB.QueryRow(`SELECT COUNT(*) FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.column_key='context_length' AND v.model_name=? AND v.source='research'`, "Qwen3 32B").Scan(&research); err != nil {
		t.Fatal(err)
	}
	if research != 0 {
		t.Errorf("%d research cell(s) remain after the clear, want none", research)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["context_length"] != "128000" {
		t.Errorf("context_length = %q after the clear, want OpenRouter's 128000 untouched", held["context_length"])
	}
	if _, _, overtaken, err = AllProvenance(); err != nil {
		t.Fatal(err)
	} else if overtaken["Qwen3 32B"]["context_length"] {
		t.Errorf("still reported as overtaken after the clear")
	}
}

// sourceOfCell reads which source a model's one cell in a column carries.
func sourceOfCell(t *testing.T, key, model string) string {
	t.Helper()
	var src string
	if err := dbcore.DB.QueryRow(`SELECT v.source FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.column_key=? AND v.model_name=?`, key, model).Scan(&src); err != nil {
		t.Fatalf("read the source of %s on %q: %v", key, model, err)
	}
	return src
}

// TestASaveRefusesTheOneColumnNothingTypedMayReach: "added" is stated by the
// app alone, because it is the arrival date and nothing outside this catalog
// can restate it once it exists; the refusal names the source that does.
func TestASaveRefusesTheOneColumnNothingTypedMayReach(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")

	err := SaveCells("Qwen3 32B", map[string]string{AddedColumnKey: "1999-01-01T00:00:00Z"})
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
	if !strings.Contains(err.Error(), "app") {
		t.Errorf("the refusal must name the source that does state it, got %q", err)
	}
}

// TestASaveRefusesADottedKey: a column key carries no namespace any more, and a
// request that still sends one must not mint a column the rest of the app can
// never write or read through its own bare key.
func TestASaveRefusesADottedKey(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")

	err := SaveCells("Qwen3 32B", map[string]string{"openrouter.context_length": "8192"})
	if !errors.Is(err, ErrNoSuchColumn) {
		t.Fatalf("err = %v, want ErrNoSuchColumn", err)
	}
	if held := cellsOf(t, "Qwen3 32B"); len(held) != 0 {
		t.Errorf("the refused write stored %v", held)
	}
}

// TestARefusedKeyLeavesEveryOtherCellInTheSameRequestUnwritten. An empty value
// CLEARS a cell, so a write that half-lands does not fail, it deletes.
func TestARefusedKeyLeavesEveryOtherCellInTheSameRequestUnwritten(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if err := SaveCells("Qwen3 32B", map[string]string{"notes": "kept"}); err != nil {
		t.Fatal(err)
	}
	err := SaveCells("Qwen3 32B", map[string]string{
		"notes":        "replaced",
		AddedColumnKey: "1999-01-01T00:00:00Z",
	})
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["notes"] != "kept" {
		t.Errorf("the note is %q, want the value the refused request did not replace", held["notes"])
	}
}

// TestAValueTheColumnWillNotHoldIsRefusedByTheColumnItself: one rule for a
// column the app ships and a column somebody invented alike.
func TestAValueTheColumnWillNotHoldIsRefusedByTheColumnItself(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": "about 32 thousand"}); !errors.Is(err, ErrCellValue) {
		t.Errorf("err = %v, want ErrCellValue", err)
	}
	if err := SaveCells("Qwen3 32B", map[string]string{"supports_tools": "maybe"}); !errors.Is(err, ErrCellValue) {
		t.Errorf("a flag column took %q: err = %v", "maybe", err)
	}
}

// TestANumberColumnRefusesNaNInfinityAndHexadecimal: a NaN binds to SQL as NULL,
// so the cell would sort as unanswered while the page showed a value.
func TestANumberColumnRefusesNaNInfinityAndHexadecimal(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	for _, value := range []string{"NaN", "Inf", "-Infinity", "1e999", "0x1p-2", "1_000"} {
		if err := SaveCells("Qwen3 32B", map[string]string{"context_length": value}); !errors.Is(err, ErrCellValue) {
			t.Errorf("a number column took %q: err = %v", value, err)
		}
	}
	if err := SaveCells("Qwen3 32B", map[string]string{"context_length": "32768"}); err != nil {
		t.Errorf("a plain number was refused: %v", err)
	}
}

// TestAnUnknownKeyIsRefusedAndNamesTheColumnsRoute: making a column is the
// reader's own act, so a key nobody holds is refused rather than minted, and
// the refusal says where a column is made.
func TestAnUnknownKeyIsRefusedAndNamesTheColumnsRoute(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	err := SaveCells("Qwen3 32B", map[string]string{"trained_on": "public code"})
	if !errors.Is(err, ErrNoSuchColumn) {
		t.Fatalf("err = %v, want ErrNoSuchColumn", err)
	}
	if !strings.Contains(err.Error(), "POST /api/columns") {
		t.Errorf("the refusal must name where a column is made, got %q", err)
	}
	if _, err := ColumnByKey("trained_on"); !errors.Is(err, ErrNoSuchColumn) {
		t.Errorf("a column was minted after all: %v", err)
	}
}

// TestASaveWritesNoModelItDoesNotAlreadyHold, so a mistyped name fails instead
// of minting a row carrying whatever was sent.
func TestASaveWritesNoModelItDoesNotAlreadyHold(t *testing.T) {
	seedInto(t)
	if err := SaveCells("Nothing: Here", map[string]string{"notes": "hello"}); !errors.Is(err, ErrNoSuchModel) {
		t.Fatalf("err = %v, want ErrNoSuchModel", err)
	}
	var rows int
	if err := dbcore.DB.QueryRow("SELECT COUNT(*) FROM models").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d rows exist, want none: a save creates no model", rows)
	}
}

// TestEveryEmptyColumnIsWritableOnEveryRowShape: which route created a row
// decides nothing about a write. On every row shape a value typed into an
// empty fetched cell lands as research and a value in a column of the reader's
// own lands as hand; only a cell an automatic source already holds refuses.
func TestEveryEmptyColumnIsWritableOnEveryRowShape(t *testing.T) {
	seedInto(t)
	rows := []struct{ what, source, id string }{
		{"an OpenRouter model", "openrouter", "acme/one"},
		{"a model imported from a repository", "huggingface", ""},
		{"a model added by hand", "manual", ""},
	}
	values := map[string]string{
		"context_length":     "4096",
		"price_completion":   "0.000002",
		"model_type":         "chat",
		"supports_tools":     "yes",
		"supports_reasoning": "no",
		"moe":                "no",
		"notes":              "a note",
		"favorite":           "yes",
		"parameters":         "32.76",
		"repository":         "acme/one",
	}
	for _, row := range rows {
		if _, err := dbcore.DB.Exec("INSERT INTO models (name, source, updated_at) VALUES (?,?,?)",
			row.what, row.source, dbcore.NowStamp()); err != nil {
			t.Fatalf("%s: %v", row.what, err)
		}
		if row.id != "" {
			if err := WriteCells(SourceOpenRouter, row.what,
				map[string]string{"id": row.id, "price_prompt": "0.09"}); err != nil {
				t.Fatalf("%s: record what OpenRouter states: %v", row.what, err)
			}
		}
		if err := SaveCells(row.what, values); err != nil {
			t.Errorf("%s: a write of every empty column was refused: %v", row.what, err)
			continue
		}
		held := cellsOf(t, row.what)
		for key, want := range values {
			if held[key] != want {
				t.Errorf("%s: %s holds %q, want %q", row.what, key, held[key], want)
			}
		}
		if row.id != "" {
			err := SaveCells(row.what, map[string]string{"price_prompt": "0.01"})
			if !errors.Is(err, ErrNotWritable) {
				t.Errorf("%s: typing over OpenRouter's price: err = %v, want ErrNotWritable", row.what, err)
			}
		}
	}
}

// TestAddingAModelTakesTheSameCellsASaveDoes, and refuses the same keys - before
// the row is inserted, so a refusal leaves nothing behind under that name.
func TestAddingAModelTakesTheSameCellsASaveDoes(t *testing.T) {
	seedInto(t)
	if err := AddModelWithCells("My Local Build", map[string]string{
		"notes":          "runs on my own machine",
		"context_length": "8192",
		"parameters":     "27",
	}); err != nil {
		t.Fatalf("AddModelWithCells: %v", err)
	}
	held := cellsOf(t, "My Local Build")
	if held["notes"] != "runs on my own machine" || held["context_length"] != "8192" {
		t.Errorf("the cells sent with the model are %v", held)
	}
	// ALL FIELDS: the fetched ones land as research, his own as hand.
	if src := sourceOfCell(t, "context_length", "My Local Build"); src != string(SourceResearch) {
		t.Errorf("context_length on an added model was written as %q, want research", src)
	}
	if src := sourceOfCell(t, "notes", "My Local Build"); src != string(SourceHand) {
		t.Errorf("notes on an added model was written as %q, want hand", src)
	}

	if err := AddModelWithCells("Another Build", map[string]string{AddedColumnKey: "1999-01-01T00:00:00Z"}); !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
	if _, exists, err := IdentityOf("Another Build"); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Error("a refused add left a row behind")
	}
}

// TestTheThreeMeasuredFiguresAreStoredAtTwoDecimals wherever they are written,
// so a value read out of a column and written back into it is the value that was
// already there.
func TestTheThreeMeasuredFiguresAreStoredAtTwoDecimals(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	if err := SaveCells("Qwen3 32B", map[string]string{"parameters": "32.762123264"}); err != nil {
		t.Fatal(err)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["parameters"] != "32.76" {
		t.Errorf("parameters holds %q, want 32.76", held["parameters"])
	}
}
