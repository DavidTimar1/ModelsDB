package dbcore

import "database/sql"

// This file is migration v44: drops ignored_models and its index. The Ignore
// feature is gone, and nothing holds a model's key back from coming again: an
// OpenRouter row is written by every refresh (catalog.FetchModels) and cannot
// be deleted at all.
//
// No data is carried forward: what the table held was never anything but a key
// meant to stop a row coming back, and nothing this app does reads it.
func theIgnoredModelsTableIsDropped(tx *sql.Tx) error {
	if _, err := tx.Exec("DROP INDEX IF EXISTS idx_ignored_models_name"); err != nil {
		return err
	}
	_, err := tx.Exec("DROP TABLE IF EXISTS ignored_models")
	return err
}
