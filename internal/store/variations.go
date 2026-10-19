// Variations: the variations of a model.
//
// A VARIATION is one quantized build of a model - a specific file on a specific
// HuggingFace repository, in which each weight is stored with fewer bits so the
// model takes less space. It is a row on `models` like any other, with two
// things that make it a variation:
//
//   - variant_of holds the NAME of the model it was built from, and
//   - hidden is set when it is created, so it is left out of the default
//     GET /api/models row set and of the browser table's ordinary list.
//
// It is a real row rather than a nested object because the browser table and the
// query API can sort and filter on a COLUMN and on nothing else: a fact buried in
// a nested object has no cell to sort by. That is also why nothing this importer
// measures is stored as JSON - the exact byte count, the bit width and the
// format label are each a column of their own, so each of them can be sorted,
// filtered and read in a cell.
//
// EVERY COLUMN IT FILLS BELONGS TO THE REPOSITORY THAT WAS READ. The importer
// hands over what it READ and RecordFacts writes each figure into the
// huggingface column that states it, where nothing else can reach it. A person
// never types a bit width or a format, so a value in either is a measurement
// rather than a recollection.
//
// The hidden flag is a DEFAULT, not a rule. It is written when the row is created
// and never again, so a user who chooses to un-hide a build keeps that choice
// through a re-import. Hiding is one mechanism with one meaning - keep this row out
// of the default list - and which rows that is remains the user's decision.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"strings"
)

// Variation is one variation to record, as the importer hands it over.
// Every field is read off the repository; none of it is typed by a person.
type Variation struct {
	Parent      string            // the name of the model this was built from
	Name        string            // this build's own display name (the models primary key)
	HFSlug      string            // the repository the build lives on
	Description string            // one line saying what this build is
	Facts       map[string]string // what was read, by Fact key (see facts.go)
}

// ImportVariation records one variation of a model.
//
// A PARENT THAT IS ITSELF A VARIATION IS REFUSED with ErrNotAParent, before
// anything is written, because the table draws a variation as a child row and a
// child row has no children of its own.
//
// On a repeat import of the same build the row is REFRESHED, not replaced: the
// columns this importer owns are rewritten from the repository, and everything
// personal on the row - the note, the favourite flag, the hidden flag, every
// value in a column of the reader's own - is left exactly as it is. That matters most for hidden,
// which is why it is written in the INSERT and named nowhere in the conflict
// clause: re-reading a repository must not undo a user's decision to show a build.
//
// The objective facts a build shares with the model it was made from - what it
// does, what it takes in and gives back, how many parameters it has - are copied
// from the parent row at import time, because they are true of the build too and
// a row without them reads as broken in the table. Prices are NOT copied: a file
// you download has no hosted price.
//
// What the importer MEASURED is recorded last, and separately, because each of
// those figures is the repository's own answer and goes into the repository's own
// column.
func ImportVariation(v Variation) error {
	// The parent is a lookup key and is used exactly as given, for the reason
	// under FileAsVariation below. The build's OWN name is one this import is
	// creating, so it is tidied like every other created name.
	parent := v.Parent
	name := strings.TrimSpace(v.Name)
	if hasNoName(parent) {
		return fmt.Errorf("%w: a variation needs the name of the model it was built from", ErrNoModelName)
	}
	if hasNoName(name) {
		return fmt.Errorf("%w: a variation needs a name of its own", ErrNoModelName)
	}
	if name == parent {
		return fmt.Errorf("a variation cannot be the model it was built from")
	}

	if _, exists, err := IdentityOf(parent); err != nil || !exists {
		return fmt.Errorf("no model called %q to add a version to", parent)
	}
	// A BUILD CANNOT HANG OFF A BUILD. A variation is drawn as a child row of its
	// parent and a child row has no children of its own, so a build recorded
	// under a row that is itself filed under something would be drawn nowhere at
	// all - present in the database and invisible in the table. FileAsVariation
	// refuses the same relation; this is the other route in.
	//
	// Refused BEFORE anything is written, so a request that cannot be drawn
	// leaves no row and no cells behind.
	parentVariantOf, err := variantOf(parent)
	if err != nil {
		return err
	}
	if parentVariantOf != "" {
		return fmt.Errorf("%w: %q is itself a variation of %q, so a build cannot be recorded under it", ErrNotAParent, parent, parentVariantOf)
	}
	// Tool support is deliberately NOT carried over: no repository states it, which
	// is why the only column holding it belongs to OpenRouter, and copying the
	// parent's answer into a source column would make this app the source of a
	// claim nobody made.
	parentCells, err := ModelCells(parent)
	if err != nil {
		return err
	}
	modelType, moe := parentCells["model_type"], parentCells["moe"]
	inMods, outMods := parentCells["input_modalities"], parentCells["output_modalities"]
	ctx := wholeNumberOrNil(parentCells["context_length"])

	// A row that already carries this display name and is a model of its own is not
	// this build: filing it would hide it and change what it is. Only a row that is
	// already a variation is refreshed by a repeat import.
	if existing, held, err := IdentityOf(name); err != nil {
		return err
	} else if held && existing.VariantOf == "" {
		return fmt.Errorf("%w: %q is already a model of its own, not a build, so the build was not recorded under that name", ErrModelExists, name)
	}

	_, err = dbcore.DB.Exec(`
INSERT INTO models (name, source, variant_of, hidden, updated_at)
VALUES (?,'huggingface',?,1,?)
ON CONFLICT(name) DO UPDATE SET
  variant_of=excluded.variant_of,
  updated_at=excluded.updated_at`,
		name, parent, dbcore.NowStamp())
	if err != nil {
		return err
	}
	if err := StampAdded(name); err != nil {
		return err
	}

	// What the build shares with the model it was made from is written as the
	// repository's own answer, because the repository is where both were read.
	// Prices are NOT among them: a file you download has no hosted price.
	recordNonEmpty(SourceHuggingFace, name, map[string]string{
		"repository":        strings.TrimSpace(v.HFSlug),
		"description":       strings.TrimSpace(v.Description),
		"model_type":        modelType,
		"moe":               flagWordOrText(moe),
		"context_length":    numberCellText(ctx),
		"input_modalities":  inMods,
		"output_modalities": outMods,
	})

	if err := inheritFactColumns(parent, name); err != nil {
		return err
	}
	_, err = RecordFacts(name, SourceHuggingFace, v.Facts)
	return err
}

// inheritFactColumns copies the parent's parameter counts onto the build. A
// figure the parent has no answer for is passed over.
//
// EACH FIGURE IS RECORDED UNDER THE SOURCE inheritedFacts names for it: the total
// as the repository's answer, so a later reading of the build's own repository
// is free to correct it, and the active count as research, the only source its
// column declares.
func inheritFactColumns(parent, name string) error {
	cells, err := ModelCells(parent)
	if err != nil {
		return err
	}
	for _, inherited := range inheritedFacts {
		value := stated(cells, inherited.from)
		if value == "" {
			continue
		}
		recordCells(inherited.source, name, map[string]string{inherited.into: value})
	}
	return nil
}

// ParentModelType is the type of the model a build was made from, or "" when the
// row is not filed under one or its parent holds no type. It is what a build
// whose own repository states no task, or a task that maps to nothing, is typed
// as: a quantized copy does what the model it was made from does, and "chat" is
// only the default for a row that has no parent to ask.
func ParentModelType(name string) string {
	parent, err := variantOf(name)
	if err != nil || parent == "" {
		return ""
	}
	cells, err := ModelCells(parent)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cells["model_type"])
}

// HFSlugOf returns a model's HuggingFace repository id and whether the model
// exists at all. The two answers are separate because they lead to different
// replies: an unknown model is a 404, and a known model with no repository is a
// model whose builds cannot be looked up - which is why the control that looks
// them up is ABSENT on such a row rather than present and failing.
func HFSlugOf(name string) (slug string, exists bool, err error) {
	_, held, err := IdentityOf(name)
	if err != nil {
		return "", false, err
	}
	if !held {
		return "", false, nil
	}
	slug, err = RepositoryOf(name)
	return strings.TrimSpace(slug), true, err
}

// VariationsOf lists the names of a model's recorded builds, oldest first. It is
// what a caller uses to tell whether removing a model takes anything else with
// it.
func VariationsOf(parent string) ([]string, error) {
	rows, err := dbcore.DB.Query("SELECT name FROM models WHERE variant_of=? ORDER BY name", parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// FileAsVariation files a model ALREADY IN THE CATALOG under another one, as a
// variation of it. Nothing is fetched and nothing is measured: the row keeps
// every value it holds - its note, its favourite star, every value in a column
// of the reader's own, its size, its format - and gains one fact, that it is a variation of parent. It
// is hidden by the same default a recorded build gets, so the list of models does
// not fill up with them.
//
// It is the second way a variation comes to exist, and it is deliberately not the
// same function as ImportVariation. That one CREATES a row from a repository it
// just read, which is what makes the bit width and the format measurements there;
// this one moves a row that is already here and invents nothing, so it writes
// exactly one column plus the hidden flag and leaves every column of the
// reader's own alone.
//
// FOUR THINGS ARE REFUSED, and each of them would leave the table unable to draw
// itself. A variation is drawn as a child row of its parent, so the relation has
// to be one level deep and acyclic:
//
//   - a model filed under itself, which is a row that is its own child;
//   - a model filed under one of its own variations, which is a cycle;
//   - a model filed under a row that is itself a variation, which is a chain the
//     table cannot draw, since a child row has no children of its own;
//   - a model that HAS variations of its own, whose variations would become
//     grandchildren of the new parent and be drawn nowhere at all.
//
// The name of an unknown model is refused too, with ErrNoSuchModel, so a typo
// fails instead of filing something under nothing.
// BOTH NAMES ARE LOOKUP KEYS AND ARE USED EXACTLY AS GIVEN. models.name is the
// primary key, and the catalog refresh stores an upstream name verbatim - space
// at the end included - so trimming either one here would make those rows
// impossible to file, and impossible to file anything under.
func FileAsVariation(name, parent string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(parent) == "" {
		return fmt.Errorf("both the model to file and the model to file it under are required")
	}
	if name == parent {
		return fmt.Errorf("%q cannot be a variation of itself", name)
	}

	// Reading the row proves it exists; a row already filed under another model
	// simply moves to its new parent, which is the ordinary way a mistake is
	// corrected.
	if _, err := variantOf(name); err != nil {
		return err
	}
	parentVariantOf, err := variantOf(parent)
	if err != nil {
		return err
	}
	if parentVariantOf == name {
		return fmt.Errorf("%q is already a variation of %q, so %q cannot be filed under it", parent, name, name)
	}
	if parentVariantOf != "" {
		return fmt.Errorf("%q is itself a variation of %q, and a variation cannot have variations of its own", parent, parentVariantOf)
	}
	own, err := VariationsOf(name)
	if err != nil {
		return err
	}
	if len(own) > 0 {
		return fmt.Errorf("%q has %d variation(s) of its own, which would be left with nowhere to sit; move those out first", name, len(own))
	}
	_, err = dbcore.DB.Exec("UPDATE models SET variant_of=?, hidden=1, updated_at=? WHERE name=?",
		parent, dbcore.NowStamp(), name)
	return err
}

// UnfileVariation takes a row back out from under the model it was filed under,
// so it is a model of its own again. It is the way back from FileAsVariation and
// from a build recorded under the wrong model, and without it a misfiled row is
// stuck: nothing else clears variant_of, and a row the OpenRouter feed lists
// cannot be deleted either (see app.handleDeleteModel).
//
// IT CHANGES ONE COLUMN AND NOTHING ELSE. hidden in particular is left exactly
// as it stands: filing hid the row and unfiling does not show it, because
// whether a row is on the list is its own decision with its own control, and
// undoing one by doing the other is what made a misfiled row unreachable in the
// first place. Every cell, note, favourite and value in a column of the
// reader's own is untouched too, the added stamp included.
//
// A ROW WITH VARIATIONS OF ITS OWN IS REFUSED, the same relation FileAsVariation
// refuses and for the same reason: those rows are drawn as children of this one,
// and this one is about to stop being where they hang. The way through is to
// unfile them first, which this function is.
//
// The name is a LOOKUP KEY and is used exactly as given, for the reason under
// FileAsVariation above.
func UnfileVariation(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("the model to unfile is required")
	}
	parent, err := variantOf(name)
	if err != nil {
		return err
	}
	if parent == "" {
		return fmt.Errorf("%q is not filed under any model, so there is nothing to unfile", name)
	}
	own, err := VariationsOf(name)
	if err != nil {
		return err
	}
	if len(own) > 0 {
		return fmt.Errorf("%q has %d variation(s) filed under it, which would be left with nowhere to sit; unfile those first", name, len(own))
	}
	_, err = dbcore.DB.Exec("UPDATE models SET variant_of='', updated_at=? WHERE name=?",
		dbcore.NowStamp(), name)
	return err
}

// variantOf returns the name of the model a row is a variation of (” when it is
// an ordinary row), and ErrNoSuchModel when there is no such row at all.
func variantOf(name string) (string, error) {
	var v string
	err := dbcore.DB.QueryRow("SELECT variant_of FROM models WHERE name=?", name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %q", ErrNoSuchModel, name)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}
