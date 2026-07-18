// Package store - seeding an empty SQLite DB from the durable curated.json export.
//
// The database (modelsdb.db) is the single source of truth for ALL model data,
// objective and personal (notes/ratings/etc.). curated.json is a one-way export
// of just the OBJECTIVE subset for the public repo; it is also the rebuild source
// for an EMPTY database (a fresh clone / fresh install): non-API models in full,
// plus curated fields for every model, with OpenRouter metadata then filled by the
// startup fetch. Personal data is never written to a file - back it up by copying
// the database. With no curated.json present, an empty DB seeds from the catalog
// embedded in the binary; with neither present, the startup fetch populates it.
package store

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/seedcatalog"
	"modelsdb/internal/shared"
	"os"
)

// CuratedDoc is the on-disk shape of curated.json: a metadata header plus the
// model records. It is the type used for BOTH export (store.go) and import.
type CuratedDoc struct {
	SchemaVersion int                      `json:"schema_version"`
	MinAppVersion string                   `json:"min_app_version"`
	Models        []map[string]interface{} `json:"models"`
}

// readCurated parses curated.json. It accepts the current object shape
// ({schema_version, min_app_version, models:[...]}) and, defensively, a bare
// array (the pre-0.1.0 shape) so an un-refreshed file still imports.
func readCurated(path string) (CuratedDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return CuratedDoc{}, err
	}
	return parseCurated(b)
}

// parseCurated decodes curated.json bytes into a CuratedDoc, accepting the
// current object shape ({schema_version, min_app_version, models:[...]}) and,
// defensively, a bare array (the pre-0.1.0 shape).
func parseCurated(b []byte) (CuratedDoc, error) {
	var doc CuratedDoc
	if err := json.Unmarshal(b, &doc); err == nil && doc.Models != nil {
		return doc, nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(b, &arr); err != nil {
		return CuratedDoc{}, err
	}
	return CuratedDoc{Models: arr}, nil
}

// CuratedCompatible reports an error if this build is too old to import a
// catalog declaring the given min_app_version. "dev" builds are never blocked.
func CuratedCompatible(minVer string) error {
	if minVer == "" || shared.IsDevVersion() {
		return nil
	}
	if shared.CompareSemver(shared.Version, minVer) < 0 {
		return fmt.Errorf("this model catalog needs ModelsDB %s or newer, but this build is %s - please update ModelsDB", minVer, shared.Version)
	}
	return nil
}

// EnsureSeeded rebuilds an EMPTY DB. It seeds, in order, from a local
// curated.json, then the catalog embedded in the binary; with neither present it
// leaves the DB empty and the startup OpenRouter fetch populates it.
//
// curated.json is the rebuild source ONLY for an empty DB. Once the DB has data
// it is the source of truth, and curated.json is a derived export - so a curated
// backup restored onto a populated DB is NOT auto-imported (it would otherwise
// clobber the DB). To roll back, replace modelsdb.db (it is migrated forward on
// open) or clear it and restart so this seed rebuilds from curated.json.
func EnsureSeeded() error {
	n, err := dbcore.CountModels()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil // existing DB is the source of truth; nothing to seed
	}
	if paths.FileExists(paths.CuratedJsonFile) {
		log.Printf("Seeding DB from %s", paths.CuratedJsonFile)
		return ImportCuratedJSON(paths.CuratedJsonFile)
	}
	// The binary embeds the published catalog, so a fresh install seeds itself
	// offline on first launch - no network fetch, no setup prompt.
	if seedcatalog.Available() {
		log.Printf("Seeding DB from the embedded catalog (%d bytes)", len(seedcatalog.Bytes))
		return ImportCuratedBytes(seedcatalog.Bytes)
	}
	log.Printf("Empty DB and no %s; startup fetch will populate it", paths.CuratedJsonFile)
	return nil
}

// ExportMissing writes curated.json if it does not yet exist, so a fresh install
// materializes the public objective catalog WITHOUT overwriting one the user
// already has. It only ever touches curated.json: personal data lives solely in
// the database (the source of truth) and is never written to a file, so there is
// nothing else to materialize. The catalog is otherwise rewritten only by a real
// data change (an in-app save or a user-run update). See "Data durability" in
// CLAUDE.md.
func ExportMissing() {
	if !paths.FileExists(paths.CuratedJsonFile) {
		if err := ExportCuratedJSON(paths.CuratedJsonFile); err != nil {
			log.Printf("Warning: could not create %s: %v", paths.CuratedJsonFile, err)
		}
	}
}

// ImportCuratedJSON rebuilds the DB from the durable export. Non-API models are
// recreated in full; curated fields are applied to every model (minimal rows are
// created for OpenRouter models so the startup fetch can enrich them).
func ImportCuratedJSON(path string) error {
	doc, err := readCurated(path)
	if err != nil {
		return err
	}
	return importDoc(doc, path)
}

// ImportCuratedBytes rebuilds the DB from an in-memory curated catalog (the
// binary's embedded seed), so a fresh install can populate an empty DB offline
// on first launch without fetching anything.
func ImportCuratedBytes(b []byte) error {
	doc, err := parseCurated(b)
	if err != nil {
		return err
	}
	return importDoc(doc, "embedded catalog")
}

// importDoc applies a parsed curated document to the DB: it gates on the
// catalog's min_app_version, then recreates non-API models in full and applies
// curated fields to every record. src is a label for logging only.
func importDoc(doc CuratedDoc, src string) error {
	if err := CuratedCompatible(doc.MinAppVersion); err != nil {
		log.Printf("Refusing to import %s: %v", src, err)
		return err
	}
	for _, r := range doc.Models {
		if shared.GetStr(r, "source") != "openrouter" {
			if err := ImportFullRecord(r, shared.GetStr(r, "source"), shared.GetStr(r, "model_type")); err != nil {
				return err
			}
			// Persist the exported ZDR for non-OpenRouter models: they have no API
			// to re-derive from (the rule makes them ZDR, but an explicit export
			// value, when present, round-trips). OpenRouter models are re-derived
			// by the startup refresh, so their column is not seeded here.
			if z, ok := r["zdr"]; ok {
				flag := 1
				if b, isBool := z.(bool); isBool && !b {
					flag = 0
				}
				dbcore.DB.Exec("UPDATE models SET zdr=? WHERE name=?", flag, shared.GetStr(r, "name"))
			}
		}
		if err := UpdateCurated(shared.GetStr(r, "name"), r); err != nil {
			return err
		}
	}
	log.Printf("Imported %d records from %s", len(doc.Models), src)
	return nil
}
