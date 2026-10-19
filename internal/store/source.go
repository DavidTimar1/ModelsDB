// Source: who stated a cell.
//
// The source lives in the cell's own primary key, so one column (context_length)
// holds OpenRouter's answer, HuggingFace's answer and a researched answer as
// separate rows, and a column is not duplicated once per source that can state
// it.

package store

import "fmt"

// Source names who stated a cell. It is stored verbatim in cells.source and
// in column_sources.source, and it must match a row of the sources table that
// internal/dbcore seeds - these exact strings are duplicated there because dbcore
// sits below store in the import DAG and cannot import this type. Change them
// together.
type Source string

// The sources a cell can be stated by.
const (
	SourceOpenRouter  Source = "openrouter"
	SourceHuggingFace Source = "huggingface"
	SourceResearch    Source = "research"
	SourceHand        Source = "hand"
	SourceApp         Source = "app"
)

// fetchedSources are the AUTOMATIC sources: the ones that state a value by
// reading something outside this app, on every refresh or import, without
// anybody typing. A column one of them can state is permanent, because the
// source states it again on every read (see DeleteColumn), and a cell one of
// them holds is never typed over (see sourceForOwnWrite in savecells.go).
// research is deliberately NOT here: it is what fills a gap until an
// automatic source states the fact, and it is outranked the moment one does.
var fetchedSources = map[Source]bool{
	SourceOpenRouter:  true,
	SourceHuggingFace: true,
}

// Valid reports whether src is one of the sources this app knows.
func (src Source) Valid() bool {
	switch src {
	case SourceOpenRouter, SourceHuggingFace, SourceResearch, SourceHand, SourceApp:
		return true
	}
	return false
}

// Kind groups the sources by what kind of write they make: "fetched" for an
// automatic source stating what something outside this app said, "research"
// for a value found by a person or an assistant to fill a gap no automatic
// source has stated yet, "hand" for the reader's own answer in a column of
// theirs, and "static" for the app's own bookkeeping (added is the only column
// stated as SourceApp). sourceForOwnWrite asks it whether a cell on file is
// automatic, and AllProvenance counts only automatic cells for a dispute.
func (src Source) Kind() string {
	switch src {
	case SourceOpenRouter, SourceHuggingFace:
		return "fetched"
	case SourceResearch:
		return "research"
	case SourceHand:
		return "hand"
	case SourceApp:
		return "static"
	}
	return ""
}

// allSources lists every source, for a refusal a person reads.
func allSources() []string {
	return []string{string(SourceOpenRouter), string(SourceHuggingFace), string(SourceResearch), string(SourceHand), string(SourceApp)}
}

// validSource refuses anything that is not one of the sources.
func validSource(src Source) error {
	if src.Valid() {
		return nil
	}
	return fmt.Errorf("%w: %q is not a source this app knows. The sources are %s",
		ErrColumnKey, src, quotedList(allSources()))
}
