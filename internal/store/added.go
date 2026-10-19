// When a model first entered THIS catalog.
//
// It is a different fact from the listing dates beside it. OpenRouter's
// created_at cell is when OpenRouter published the listing and HuggingFace's is
// when the repository was made; both are the source's own answer and both are
// stated again on every read. When a row appeared in this database is nobody's answer
// but this database's, no source can ever restate it, and nothing but the moment
// of insertion can record it - which is why it is written here, at every path
// that creates a row, rather than derived later.

package store

import "modelsdb/internal/dbcore"

// AddedColumnKey is the column that date lives in. Its only source is the app
// itself (SourceApp): no source states it and no hand write may correct it,
// because the moment of insertion is the fact and nothing else can restate it.
const AddedColumnKey = "added"

// StampAdded records that a model has entered this catalog, unless it already
// carries that date.
//
// IT NEVER OVERWRITES. Every path that creates a row calls it, and three of them
// also run on a row that is already here - the OpenRouter refresh upserts, an
// import re-imports, a build is read again - so a write that replaced the value
// would move the date forward on every refresh and the column would answer "the
// last time anything touched this row", which models.updated_at already answers.
//
// A FAILURE IS RETURNED, NOT SWALLOWED, and every caller passes it on. The date
// exists nowhere else and nothing can work it out afterwards, so a row created
// without one is a fact lost for good rather than a value that can be filled in
// later.
func StampAdded(modelName string) error {
	if hasNoName(modelName) {
		return ErrNoModelName
	}
	held, err := ModelCells(modelName)
	if err != nil {
		return err
	}
	if held[AddedColumnKey] != "" {
		return nil
	}
	if err := ensureShippedColumns(); err != nil {
		return err
	}
	return WriteCells(SourceApp, modelName, map[string]string{AddedColumnKey: dbcore.NowStamp()})
}
