// Which column each catalogue fact lands in, and which source
// may state it there.

package store

import "strings"

// factCellKeys says which column each catalogue fact's values land in, for
// every fact except the one whose answer means a different thing depending on
// who states it (see factCellKeysBySource below).
//
// TestEveryMirroredFactLandsInAShippedColumn holds this pairing against the
// columns the app ships.
var factCellKeys = map[string]string{
	"tokenizer":              "tokenizer",
	"instruct_type":          "instruct_type",
	"supported_parameters":   "supported_parameters",
	"max_completion_tokens":  "max_completion_tokens",
	"moderated":              "moderated",
	"knowledge_cutoff":       "knowledge_cutoff",
	"expiration_date":        "expiration_date",
	"reasoning_mandatory":    "reasoning_mandatory",
	"alias_target":           "alias_target",
	"price_cache_read":       "price_cache_read",
	"price_cache_write":      "price_cache_write",
	"price_cache_write_1h":   "price_cache_write_1h",
	"price_audio_cache_read": "price_audio_cache_read",
	"price_image_output":     "price_image_output",
	"price_audio_output":     "price_audio_output",
	"price_web_search":       "price_web_search",
	"parameters":             "parameters",
	"weights_size_gb":        "weights_size_gb",
	"bits_per_weight":        "build_bits_per_weight",
	"build_format":           "build_format",
	"build_revision":         "build_revision",
	"build_files":            "build_files",
	"build_size_bytes":       "build_size_bytes",
	"base_model":             "base_model",
	"gguf_architecture":      "gguf_architecture",
}

// FactCellKeys returns a copy of that pairing, for the test that holds it
// against the columns the app ships.
func FactCellKeys() map[string]string {
	out := make(map[string]string, len(factCellKeys))
	for fact, key := range factCellKeys {
		out[fact] = key
	}
	return out
}

// factCellKeysBySource says which column a fact lands in when the answer
// depends on WHO read it. The author is the one such fact: OpenRouter's
// answer is the vendor it attributes the model to, and a repository's answer
// is the account that published it - two different facts about two different
// subjects, each with a column of its own, never merged into one.
var factCellKeysBySource = map[string]map[Source]string{
	"author": {
		SourceOpenRouter:  "vendor",
		SourceHuggingFace: "repo_owner",
	},
}

// sourceCellKeyOf answers the column one source's reading of a fact lands in,
// and "" where that source does not state it at all.
func sourceCellKeyOf(fact string, source Source) string {
	if perSource, split := factCellKeysBySource[fact]; split {
		return perSource[source]
	}
	return factCellKeys[fact]
}

// factOfKey is the reverse of the two pairings above: which catalogue fact a
// column holds, read off its bare key.
var factOfKey = indexFactsByKey()

func indexFactsByKey() map[string]string {
	m := make(map[string]string, len(factCellKeys)+2)
	for fact, key := range factCellKeys {
		m[key] = fact
	}
	for fact, perSource := range factCellKeysBySource {
		for _, key := range perSource {
			m[key] = fact
		}
	}
	return m
}

// factOfColumnKey answers the catalogue fact a column holds, and "" for a
// column no fact pairing names - a column the reader invented, or one of the
// app's own static columns like notes or added.
func factOfColumnKey(key string) string {
	return factOfKey[key]
}

// inheritedFacts pairs each figure a build is given from the model it was made
// from with the column that figure lives in, and the source it is recorded
// under. Both parameter counts are true of the build as well - a 4-bit copy of
// a 32-billion-parameter model is still a 32-billion-parameter model. The total
// is copied as the repository's answer, into the SAME column the build's own
// repository would write, so a later reading of that repository is free to
// correct it. The active count is copied as research, because no repository
// states it and its column declares research alone.
var inheritedFacts = []struct {
	from, into string
	source     Source
}{
	{"parameters", "parameters", SourceHuggingFace},
	{"active_parameters", "active_parameters", SourceResearch},
}

// recordSourceFacts writes what one source stated into the columns that
// source may state, filtered by Facts' own Sources list - a fact this source
// does not state is passed over even if the caller's map happens to carry a
// value for its key.
//
// EVERY FACT IS WRITTEN, not only the ones kept as a column of the reader's
// own. A source column is not a choice: it holds what the source said, and
// what a person chooses is which columns the table draws.
//
// It returns the column keys that were really stored, so a caller never claims a
// write that was refused.
func recordSourceFacts(model string, source Source, read map[string]string) []string {
	values := map[string]string{}
	for fact, value := range read {
		f, known := factByKey[fact]
		if !known || !f.StatedBy(source) {
			continue
		}
		key := sourceCellKeyOf(fact, source)
		if key == "" {
			continue
		}
		if value = strings.TrimSpace(value); value == "" {
			continue
		}
		// The three measured figures are held at two decimal places wherever they
		// are stored, so a value read out of a column and written back into it is
		// the value that was there.
		values[key] = roundedForStorage(key, value)
	}
	return recordCells(source, model, values)
}

// columnStem turns a column's display name into a column key: lower case,
// with every run of anything else read as one underscore. "Build size (GB)"
// becomes "build_size_gb".
func columnStem(name string) string {
	var b strings.Builder
	lastWasBreak := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasBreak = false
			continue
		}
		if !lastWasBreak {
			b.WriteByte('_')
			lastWasBreak = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// namedColumnKeys pairs a heading with the column key it names, for a heading
// whose natural stem does not match the key it belongs to. "Active parameters
// (B)" stems to "active_parameters_b", but the column is "active_parameters".
var namedColumnKeys = map[string]string{
	"active parameters (b)": "active_parameters",
}

// columnKeyForColumnName is the column key a heading names.
//
// A HEADING THE APP KNOWS TAKES THE KEY ITS FACT IS STATED UNDER, not the key
// the heading's own words would stem to: "Parameters (B)" is the column
// "parameters", because that is the column HuggingFace already states it in -
// so creating a column under this heading either reuses that shipped column
// (and CreateOwnColumn then refuses it as already existing, pointing the
// reader at it) or, for a heading the app has never heard of, mints a column
// of the reader's own from the heading's own words.
func columnKeyForColumnName(name string) string {
	if key, named := namedColumnKeys[strings.ToLower(strings.TrimSpace(name))]; named {
		return key
	}
	for _, f := range Facts {
		if !strings.EqualFold(strings.TrimSpace(f.Column), strings.TrimSpace(name)) {
			continue
		}
		if key, mapped := factCellKeys[f.Key]; mapped {
			return key
		}
		if _, split := factCellKeysBySource[f.Key]; split {
			return f.Key
		}
	}
	return columnStem(name)
}
