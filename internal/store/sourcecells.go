// How a SOURCE's reading of a model is written into that
// source's own columns.
//
// Every automatic pass - the OpenRouter pull, the ZDR pass, the repository
// import and the variation importer - ends here. Each writes as one source, so
// its cells are stamped as coming from it and a read decides the winner by that
// stamp, never by write order.
//
// A VALUE ONE SOURCE STATES BADLY DOES NOT COST THE OTHERS. An automatic pass
// runs over the whole catalog with nobody watching, so a single unusable figure
// is logged and passed over rather than raised: what it must not do is stop the
// pass recording everything else it read. A value a PERSON typed is the opposite
// case and is refused loudly, because there is somebody there to correct it.

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"sort"
	"strconv"
	"strings"
)

// columnExists reports whether this database holds a column of that key.
func columnExists(key string) (bool, error) {
	_, err := ColumnByKey(key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNoSuchColumn) {
		return false, nil
	}
	return false, err
}

// flagWordOrText carries a capability answer across in the vocabulary a flag
// column holds: "unknown" is the word for an answer nobody has given, which is a
// cell with nothing in it.
func flagWordOrText(text string) string {
	if strings.ToLower(strings.TrimSpace(text)) == "unknown" {
		return ""
	}
	return text
}

// numberCellText renders a figure a source stated as the text a cell holds,
// keeping every digit it was read with.
func numberCellText(bound interface{}) string {
	switch v := bound.(type) {
	case nil:
		return ""
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", bound))
}

// yesNoText words a true/false answer in the vocabulary a flag column holds.
func yesNoText(said bool) string {
	if said {
		return "yes"
	}
	return "no"
}

// numberOrEmpty renders a figure a source stated, leaving out anything that is
// not one.
func numberOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprintf("%v", v))
	if _, err := shared.ParseNumber(text); err != nil {
		return ""
	}
	return text
}

// listCellText renders a list of words as the JSON array a list cell holds.
func listCellText(list []string) string {
	if len(list) == 0 {
		return ""
	}
	encoded, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// recordCells writes one source's values into its own columns and returns the
// keys that were really stored, in key order.
//
// WriteCells is one transaction, so a single unusable value would leave every
// other value in the same call unwritten; on a refusal the values are therefore
// written one at a time, so the good ones land and each bad one is named in the
// log on its own.
func recordCells(src Source, model string, values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if err := ensureShippedColumns(); err != nil {
		log.Printf("Warning: the cell store could not be prepared for %q: %v", model, err)
		return nil
	}
	if err := WriteCells(src, model, values); err == nil {
		return keys
	}
	stored := make([]string, 0, len(keys))
	for _, key := range keys {
		if err := WriteCells(src, model, map[string]string{key: values[key]}); err != nil {
			log.Printf("Warning: %s for %q was not stored: %v", key, model, err)
			continue
		}
		stored = append(stored, key)
	}
	return stored
}

// recordNonEmpty writes only the values a source really stated. An empty one is
// PASSED OVER rather than clearing the cell: a caller that knows nothing about a
// column must not blank what somebody else researched, and a repository that
// states no context length must not clear the one research found.
func recordNonEmpty(src Source, name string, values map[string]string) {
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			delete(values, key)
		}
	}
	recordCells(src, name, values)
}

// storeSourceRecord stores the verbatim record one source sent, under that
// source's own name.
func storeSourceRecord(src Source, model, raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	if _, err := dbcore.DB.Exec(`INSERT INTO source_records (model_name, source, raw_json, read_at)
		VALUES (?,?,?,?) ON CONFLICT(model_name, source) DO UPDATE SET
		  raw_json=excluded.raw_json, read_at=excluded.read_at`,
		model, string(src), raw, dbcore.NowStamp()); err != nil {
		log.Printf("Warning: the record %s sent for %q was not stored: %v", src, model, err)
	}
}

// RecordZDR stores what the ZDR pass derived for one OpenRouter model.
//
// It is exported because that pass runs in the catalog package, which has no
// other way into the cell store.
func RecordZDR(model string, zdr bool) {
	recordCells(SourceOpenRouter, model, map[string]string{"zdr": yesNoText(zdr)})
}

// RecordEURouting stores whether OpenRouter's EU model list holds one
// OpenRouter model.
//
// It is exported because that pass runs in the catalog package, which has no
// other way into the cell store.
func RecordEURouting(model string, inEU bool) {
	recordCells(SourceOpenRouter, model, map[string]string{"eu_routing": yesNoText(inEU)})
}

// RecordThroughput stores the fastest measured speed, in whole tokens per
// second, that the throughput pass found for one OpenRouter model. A figure of
// zero or less clears the cell, so a model nobody measures any more shows
// nothing rather than an old speed.
//
// It is exported because that pass runs in the catalog package, which has no
// other way into the cell store.
func RecordThroughput(model string, tokensPerSecond int) {
	value := ""
	if tokensPerSecond > 0 {
		value = strconv.Itoa(tokensPerSecond)
	}
	recordCells(SourceOpenRouter, model, map[string]string{"throughput": value})
}
