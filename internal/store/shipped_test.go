package store

import (
	"encoding/json"
	"fmt"
	"modelsdb/internal/dbcore"
	"sort"
	"strings"
	"testing"
)

// columnText is one column as it is ANSWERED, which is how two of them are
// compared here: a field held behind a pointer compares by what it says rather
// than by where it sits in memory.
func columnText(t *testing.T, column Column) string {
	t.Helper()
	raw, err := json.Marshal(column)
	if err != nil {
		t.Fatalf("read %s back as JSON: %v", column.Key, err)
	}
	return string(raw)
}

// seedInto gives one test a fresh catalog with the shipped columns in it.
func seedInto(t *testing.T) []string {
	t.Helper()
	cellStoreDB(t)
	created, err := SeedShippedColumns()
	if err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
	return created
}

// TestEveryShippedColumnIsWellFormed is the test that replaces counting a list
// in Go: whatever the app ships, every sealed column has a valid value type, a
// label, and a bare key that carries no dot - a source is no longer part of a
// column's key.
func TestEveryShippedColumnIsWellFormed(t *testing.T) {
	seedInto(t)
	columns, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}

	var sealedCount int
	for _, c := range columns {
		// A column the reader owns is not one the app ships, and a real database
		// always holds some, so the question is asked of the sealed ones only.
		if !c.Sealed {
			continue
		}
		sealedCount++
		if !ValidValueType(c.ValueType) || c.Label == "" {
			t.Errorf("%s is %+v", c.Key, c)
		}
		if strings.Contains(c.Key, ".") {
			t.Errorf("%s carries a dot, and a column key names no source any more", c.Key)
		}
	}

	// The set is a decision rather than a detail, so its size is stated here and
	// moves only when somebody means it to.
	if sealedCount != len(shippedColumns) {
		t.Errorf("%d sealed columns ship, want %d", sealedCount, len(shippedColumns))
	}
}

// TestEveryShippedColumnIsOneOfTheTwoKinds. A column of the reader's own
// declares hand alone and nothing automatic ever touches it; every other
// column declares its automatic sources with research last, so a gap can be
// filled until a source states the fact; added declares the app alone. No
// column declares hand beside anything, and none declares collection.
func TestEveryShippedColumnIsOneOfTheTwoKinds(t *testing.T) {
	seedInto(t)
	declared, err := ListColumnSources()
	if err != nil {
		t.Fatal(err)
	}
	var yours []string
	for _, c := range shippedColumns {
		got := declared[c.Key]
		if fmt.Sprint(got) != fmt.Sprint(c.Sources) {
			t.Errorf("%s declares %v on disk, and shipped.go says %v", c.Key, got, c.Sources)
		}
		switch {
		case len(got) == 1 && got[0] == SourceHand:
			yours = append(yours, c.Key)
		case len(got) == 1 && got[0] == SourceApp:
			if c.Key != AddedColumnKey {
				t.Errorf("%s is stated by the app alone, and only %s is", c.Key, AddedColumnKey)
			}
		default:
			if got[len(got)-1] != SourceResearch {
				t.Errorf("%s declares %v: an automatic column ends with research", c.Key, got)
			}
			for _, src := range got {
				if src == SourceHand {
					t.Errorf("%s declares hand beside an automatic source", c.Key)
				}
			}
		}
	}
	sort.Strings(yours)
	if strings.Join(yours, ",") != "architecture_basis,favorite,notes" {
		t.Errorf("the columns of the reader's own are %v, want architecture_basis, favorite and notes", yours)
	}
}

// TestTheArchitectureColumnShipsInStepWithItsMigration. Migration v52 creates
// the column on a database that lacks it, with its own copy of the key, label and
// description because dbcore sits below store; this holds the seed's column and
// the migration's copy to one definition.
func TestTheArchitectureColumnShipsInStepWithItsMigration(t *testing.T) {
	seedInto(t)
	column, err := ColumnByKey(dbcore.ArchitectureColumnKey)
	if err != nil {
		t.Fatalf("%s is not shipped: %v", dbcore.ArchitectureColumnKey, err)
	}
	if column.Label != dbcore.ArchitectureColumnLabel || column.About != dbcore.ArchitectureColumnAbout {
		t.Errorf("the seed and the migration describe the column differently: %q / %q", column.Label, column.About)
	}
	if !column.Sealed || column.Visible || column.ValueType != ValueTypeText {
		t.Errorf("the column should be sealed, hidden and text: sealed=%v visible=%v type=%s", column.Sealed, column.Visible, column.ValueType)
	}
}

// TestTheVisibleColumnsAreTheOnesTheTableHasAlwaysShown. Everything else exists,
// holds its values, and sits behind the column chooser - so what is on screen is
// a decision rather than a consequence of what happens to be filled.
func TestTheVisibleColumnsAreTheOnesTheTableHasAlwaysShown(t *testing.T) {
	seedInto(t)
	columns, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}
	var visible []string
	for _, c := range columns {
		if c.Visible {
			visible = append(visible, c.Key)
		}
	}
	// In the order they are laid out, left to right.
	want := []string{
		"model_type", "context_length", "price_prompt", "price_completion",
		"supports_tools", "supports_reasoning", "zdr", "eu_routing", "throughput", "notes", "favorite",
		"repository", "description", "created_at", "input_modalities",
		"output_modalities", "price_unit",
	}
	if strings.Join(visible, ",") != strings.Join(want, ",") {
		t.Errorf("the visible columns are %v, want %v", visible, want)
	}
}

// TestSeedingTwiceCreatesNothingTwice. Seeding runs on a catalog that may
// already have been seeded, so it has to be a no-op the second time.
func TestSeedingTwiceCreatesNothingTwice(t *testing.T) {
	first := seedInto(t)
	if len(first) != len(shippedColumns) {
		t.Fatalf("the first seed created %d columns, want %d", len(first), len(shippedColumns))
	}
	before, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}

	again, err := SeedShippedColumns()
	if err != nil {
		t.Fatalf("seed a second time: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("the second seed created %v, want nothing", again)
	}
	after, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("%d columns exist after seeding twice, want %d", len(after), len(before))
	}
	for i := range after {
		// Compared as the JSON each column answers with, so a field held behind a
		// pointer - the colour rule - is compared by what it SAYS. Two identical
		// rules read at different moments are two different addresses.
		if columnText(t, after[i]) != columnText(t, before[i]) {
			t.Errorf("%s changed on the second seed: %s, was %s",
				after[i].Key, columnText(t, after[i]), columnText(t, before[i]))
		}
	}
}

// TestSeedingLeavesEverythingSomebodyChangedAlone. The label, where the column
// sits, whether it is shown, its kind and its option list are all things a
// person may have set on purpose, and a seed that corrected them would undo that
// work every time the app started.
func TestSeedingLeavesEverythingSomebodyChangedAlone(t *testing.T) {
	cellStoreDB(t)

	// Somebody got to one of the shipped keys first, and made it a different
	// shape entirely: a choice column with their own list of options.
	makeColumn(t, ColumnSpec{Key: "model_type", Label: "Kind of thing", ValueType: ValueTypeChoice,
		Options: []string{"chat", "image", "embedding"}, Position: 40, Hidden: true})

	if _, err := SeedShippedColumns(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	his, err := ColumnByKey("model_type")
	if err != nil {
		t.Fatal(err)
	}
	if his.Label != "Kind of thing" || his.ValueType != ValueTypeChoice || his.Visible ||
		his.Position != 40 || strings.Join(his.Options, ",") != "chat,image,embedding" {
		t.Errorf("the column somebody made was changed by the seed: %+v", his)
	}

	// And what the seed itself created is his to change afterwards.
	if err := SetColumnVisible("zdr", false); err != nil {
		t.Fatal(err)
	}
	if err := RelabelColumn("zdr", "Privacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedShippedColumns(); err != nil {
		t.Fatal(err)
	}
	changed, err := ColumnByKey("zdr")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Label != "Privacy" || changed.Visible {
		t.Errorf("the seed put back what somebody changed: %+v", changed)
	}
}

// TestASeededCatalogAnswersEverySourceThatWroteIt walks one model through the
// shipped set the way the sources write it: each source's own answer is its
// own cell, an automatic answer outranks a researched one without moving it,
// and a note is the reader's alone.
func TestASeededCatalogAnswersEverySourceThatWroteIt(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")

	if err := WriteCells(SourceOpenRouter, "Qwen3 32B", map[string]string{
		"context_length": "32768",
		"supports_tools": "yes",
		"price_prompt":   "0.0000015",
		"zdr":            "no",
		"vendor":         "qwen",
	}); err != nil {
		t.Fatalf("write what OpenRouter states: %v", err)
	}
	if err := WriteCells(SourceResearch, "Qwen3 32B", map[string]string{
		"context_length": "131072",
		"parameters":     "32.76",
	}); err != nil {
		t.Fatalf("write what research found: %v", err)
	}
	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{
		"notes": "reads long diffs well",
	}); err != nil {
		t.Fatalf("write his own note: %v", err)
	}

	stored, err := ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"context_length":     "32768",     // OpenRouter's answer outranks the researched one
		"parameters":         "32.76",     // nothing automatic has stated it, so research shows
		"supports_tools":     "yes",       // the only source stating it
		"price_prompt":       "0.0000015", // exactly as upstream wrote it
		"zdr":                "no",
		"notes":              "reads long diffs well",
		"supports_reasoning": "", // nobody has said either way
	} {
		if stored[key] != want {
			t.Errorf("%s is %q, want %q", key, stored[key], want)
		}
	}
}

// TestAResearchAnswerShowsOnlyUntilASourceStatesTheFact is the whole of the
// rank rule from the reading side: research fills a gap, an automatic source
// arriving later is what a read answers with, and clearing the research cell
// changes nothing about what the source said.
func TestAResearchAnswerShowsOnlyUntilASourceStatesTheFact(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")

	if err := WriteCells(SourceResearch, "Qwen3 32B", map[string]string{
		"context_length": "8192",
		"moe":            "no",
	}); err != nil {
		t.Fatalf("write what research found: %v", err)
	}
	stored, err := ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	if stored["context_length"] != "8192" || stored["moe"] != "no" {
		t.Errorf("with nothing automatic stated the researched values do not show: %v", stored)
	}

	if err := WriteCells(SourceOpenRouter, "Qwen3 32B", map[string]string{"context_length": "131072"}); err != nil {
		t.Fatalf("write what OpenRouter states: %v", err)
	}
	if err := WriteCells(SourceHuggingFace, "Qwen3 32B", map[string]string{"moe": "yes"}); err != nil {
		t.Fatalf("write what the repository states: %v", err)
	}
	stored, err = ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"context_length": "131072", // OpenRouter has stated it, so research is outranked
		"moe":            "yes",    // the repository has stated it
	} {
		if stored[key] != want {
			t.Errorf("%s is %q, want %q", key, stored[key], want)
		}
	}

	// The researched cell is still there beneath, and clearing it moves nothing.
	if err := WriteCells(SourceResearch, "Qwen3 32B", map[string]string{"context_length": ""}); err != nil {
		t.Fatal(err)
	}
	stored, err = ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	if stored["context_length"] != "131072" {
		t.Errorf("with the research cleared the column is %q, want OpenRouter's 131072", stored["context_length"])
	}
}

// TestDeclareShippedSourcesRewritesAReorderedColumn: whatever a database
// carried forward says about a column's sources, the start-up rewrite puts
// shipped.go's order back, renumbered from 0.
func TestDeclareShippedSourcesRewritesAReorderedColumn(t *testing.T) {
	seedInto(t)
	column, err := ColumnByKey("context_length")
	if err != nil {
		t.Fatal(err)
	}
	// The layout v40 left on a migrated catalog: hand at 0, the rest from 1,
	// with the two automatic sources the wrong way round.
	if _, err := dbcore.DB.Exec("DELETE FROM column_sources WHERE column_id=?", column.ID); err != nil {
		t.Fatal(err)
	}
	for src, rank := range map[Source]int{SourceHand: 0, SourceHuggingFace: 1, SourceOpenRouter: 2} {
		if _, err := dbcore.DB.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)", column.ID, string(src), rank); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := DeclareShippedSources(); err != nil {
		t.Fatalf("DeclareShippedSources: %v", err)
	}
	if got := rankedSources(t, column.ID); got != "openrouter=0,huggingface=1,research=2" {
		t.Errorf("context_length declares %s, want openrouter=0,huggingface=1,research=2", got)
	}

	// A column the reader made declares hand alone, and a second run changes
	// nothing.
	own, err := CreateOwnColumn(ColumnSpec{Label: "Rating"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DeclareShippedSources(); err != nil {
		t.Fatal(err)
	}
	if got := rankedSources(t, own.ID); got != "hand=0" {
		t.Errorf("the reader's column declares %s, want hand=0", got)
	}
}

// TestDeclareShippedSourcesReappliesAShippedColumnsAbout: the description is
// the app's own sentence, so a database seeded before it changed reads the
// current one after a start, while a relabelled heading stays the reader's.
func TestDeclareShippedSourcesReappliesAShippedColumnsAbout(t *testing.T) {
	seedInto(t)
	if _, err := dbcore.DB.Exec("UPDATE columns SET about='How much context the model takes, or your own answer where you have given one.', label='Window' WHERE column_key='context_length'"); err != nil {
		t.Fatal(err)
	}
	if _, err := DeclareShippedSources(); err != nil {
		t.Fatal(err)
	}
	column, err := ColumnByKey("context_length")
	if err != nil {
		t.Fatal(err)
	}
	if column.About != shippedByKey["context_length"].About {
		t.Errorf("about = %q after a start, want shipped.go's %q", column.About, shippedByKey["context_length"].About)
	}
	if column.Label != "Window" {
		t.Errorf("label = %q after a start, want the reader's Window left alone", column.Label)
	}
}

// TestDeclareShippedSourcesSkipsAColumnWithAStrandedCellAndStarts is the
// soft failure: a stored cell whose source the code no longer declares is a
// migration's job, so that ONE column keeps the declarations it had, and the
// rewrite of every other column goes ahead - the server starts.
func TestDeclareShippedSourcesSkipsAColumnWithAStrandedCellAndStarts(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Qwen3 32B")
	column, err := ColumnByKey("context_length")
	if err != nil {
		t.Fatal(err)
	}
	other, err := ColumnByKey("parameters")
	if err != nil {
		t.Fatal(err)
	}
	// context_length once declared hand and holds a hand cell; parameters is
	// declared the wrong way round with no cell.
	if _, err := dbcore.DB.Exec("DELETE FROM column_sources WHERE column_id IN (?,?)", column.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	for src, rank := range map[Source]int{SourceHand: 0, SourceOpenRouter: 1, SourceHuggingFace: 2} {
		if _, err := dbcore.DB.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)", column.ID, string(src), rank); err != nil {
			t.Fatal(err)
		}
	}
	for src, rank := range map[Source]int{SourceResearch: 0, SourceHuggingFace: 1} {
		if _, err := dbcore.DB.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)", other.ID, string(src), rank); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{"context_length": "8192"}); err != nil {
		t.Fatal(err)
	}

	rewritten, err := DeclareShippedSources()
	if err != nil {
		t.Fatalf("DeclareShippedSources stopped the start: %v", err)
	}
	if rewritten != len(shippedColumns)-1 {
		t.Errorf("%d columns were rewritten, want every shipped column but the one with the stranded cell (%d)", rewritten, len(shippedColumns)-1)
	}
	if got := rankedSources(t, column.ID); got != "hand=0,openrouter=1,huggingface=2" {
		t.Errorf("the column with the stranded cell declares %s, want its old hand=0,openrouter=1,huggingface=2 left alone", got)
	}
	if got := rankedSources(t, other.ID); got != "huggingface=0,research=1" {
		t.Errorf("the other column declares %s, want huggingface=0,research=1", got)
	}
	if held := cellsOf(t, "Qwen3 32B"); held["context_length"] != "8192" {
		t.Errorf("the stranded cell is %q, want it still readable as 8192", held["context_length"])
	}
}

// rankedSources reads a column's declarations as source=rank pairs by rank.
func rankedSources(t *testing.T, columnID int64) string {
	t.Helper()
	rows, err := dbcore.DB.Query("SELECT source, rank FROM column_sources WHERE column_id=? ORDER BY rank ASC", columnID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var src string
		var rank int
		if err := rows.Scan(&src, &rank); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s=%d", src, rank))
	}
	return strings.Join(out, ",")
}
