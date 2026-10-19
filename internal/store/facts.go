// Every fact this app can record about a model, and the column
// each one is recorded in.
//
// A FACT BELONGS TO WHOEVER STATED IT. `models` carries the row's identity and
// nothing else; every fact is a cell, stamped with the source that said it. So
// a source never overwrites another source's answer: each hands over what it
// READ, keyed by fact, and RecordFacts stores each value as that source's own
// cell in the one column that answers the fact.

package store

import (
	"fmt"
	"strings"
)

// Fact is one fact this app can record about a model, and the column it is
// answered in.
//
// Key is what a request names it by, and it never changes. Column is the heading
// the fact reads under.
//
// Sources is what stops one source's silence being read as an answer. Two
// sources describe a model in different words, and a fact one of them never
// states must not be written from the other's guess, so RecordFacts refuses a
// key the source it was handed cannot state at all.
type Fact struct {
	Key     string
	Column  string
	Type    string // the kind of value the fact's column holds
	About   string // one line, for the person deciding whether to keep it
	Sources []Source
}

// Facts is every fact this app can record, in the order the import list shows
// them: what the OpenRouter catalog states first, then what a repository states
// about the model, then the facts only a downloadable build has.
//
// EVERY FACT HERE IS A CELL, never a column of models. The one place that rule is
// decided is this list, so a new fact is added here and in factCellKeys
// (factcells.go), which says whose column it lands in.
var Facts = []Fact{
	{"author", "Author", ValueTypeText, "Who published the model", []Source{SourceOpenRouter, SourceHuggingFace}},

	// What OpenRouter states about a model and no column of models holds.
	{"tokenizer", "Tokenizer", ValueTypeText, "Which tokenizer family the model uses, as OpenRouter classifies it", []Source{SourceOpenRouter}},
	{"instruct_type", "Instruct type", ValueTypeText, "Which prompt template the model expects: chatml, llama3, deepseek-r1", []Source{SourceOpenRouter}},
	{"supported_parameters", "Supported parameters", ValueTypeList, "Which request parameters the model accepts: temperature, tools, response_format", []Source{SourceOpenRouter}},
	{"max_completion_tokens", "Max completion tokens", ValueTypeNumber, "The longest answer the model will produce in one call", []Source{SourceOpenRouter}},
	{"moderated", "Moderated", ValueTypeFlag, "Whether the provider runs its own moderation over the calls: yes or no", []Source{SourceOpenRouter}},
	{"knowledge_cutoff", "Knowledge cutoff", ValueTypeText, "The date the model's training data stops at", []Source{SourceOpenRouter}},
	{"expiration_date", "Expiration date", ValueTypeText, "The date the listing stops being served", []Source{SourceOpenRouter}},
	{"reasoning_mandatory", "Reasoning mandatory", ValueTypeFlag, "Whether the model always reasons and cannot be asked not to: yes or no", []Source{SourceOpenRouter}},
	{"alias_target", "Alias target", ValueTypeText, "The model this listing is an alias of, where it is one", []Source{SourceOpenRouter}},
	{"price_cache_read", "Cache read price", ValueTypeNumber, "What one cached input token costs to read back", []Source{SourceOpenRouter}},
	{"price_cache_write", "Cache write price", ValueTypeNumber, "What one input token costs to put into the cache", []Source{SourceOpenRouter}},
	{"price_cache_write_1h", "Cache write price (1h)", ValueTypeNumber, "The same, for a cache entry kept an hour", []Source{SourceOpenRouter}},
	{"price_audio_cache_read", "Audio cache read price", ValueTypeNumber, "What one cached audio input token costs to read back", []Source{SourceOpenRouter}},
	{"price_image_output", "Image output price", ValueTypeNumber, "What one produced image costs", []Source{SourceOpenRouter}},
	{"price_audio_output", "Audio output price", ValueTypeNumber, "What one produced audio token costs", []Source{SourceOpenRouter}},
	{"price_web_search", "Web search price", ValueTypeNumber, "What one web search made on the model's behalf costs", []Source{SourceOpenRouter}},

	// What a HuggingFace repository states about the model itself.
	{"parameters", "Parameters (B)", ValueTypeNumber, "How many parameters the model has, in billions", []Source{SourceHuggingFace}},
	{"weights_size_gb", "Weights size (GB)", ValueTypeNumber, "How much disk the model's own weight files take, in GB", []Source{SourceHuggingFace}},

	// Only a downloadable build has these.
	{"bits_per_weight", "Bits per weight", ValueTypeNumber, "How many bits one weight takes in THIS build", []Source{SourceHuggingFace}},
	{"build_format", "Build format", ValueTypeText, "How the build is stored, with its quantization label: GGUF Q4_K_M, AWQ 4-bit, FP8", []Source{SourceHuggingFace}},
	{"build_revision", "Build revision", ValueTypeText, "The exact commit that repository's main branch pointed at when it was read", []Source{SourceHuggingFace}},
	{"build_files", "Build files", ValueTypeText, "Which file or files make up the build", []Source{SourceHuggingFace}},
	{"build_size_bytes", "Build size (bytes)", ValueTypeNumber, "The exact size of this build's own files - a DIFFERENT figure from the model's weights size", []Source{SourceHuggingFace}},
	{"base_model", "Base model", ValueTypeText, "The model this build was made from, as the repository states it", []Source{SourceHuggingFace}},
	{"gguf_architecture", "GGUF architecture", ValueTypeText, "The architecture name inside a GGUF file", []Source{SourceHuggingFace}},
}

// factByKey indexes Facts for the lookups that do not care about the order.
var factByKey = indexFacts()

func indexFacts() map[string]Fact {
	m := make(map[string]Fact, len(Facts))
	for _, f := range Facts {
		m[f.Key] = f
	}
	return m
}

// StatedBy reports whether that source can state this fact.
func (f Fact) StatedBy(source Source) bool {
	for _, s := range f.Sources {
		if s == source {
			return true
		}
	}
	return false
}

// FactChoice is one line of the list an import shows: the fact, the column that
// answers it, which sources can state it, and whether this database already has
// a column of the reader's own for it.
type FactChoice struct {
	Key     string   `json:"key"`
	Column  string   `json:"column"`
	Type    string   `json:"type"`
	About   string   `json:"about"`
	Sources []Source `json:"sources"`
	Exists  bool     `json:"exists"`
}

// FactChoices lists every fact this app records, with the column of the
// reader's own that answers it where they have one.
//
// NOTHING IS TICKED OR UNTICKED. Every fact a source states goes into that
// source's own column, always, because a source column is not a choice: it holds
// what the source said. What the reader chooses is which columns the table
// DRAWS, which is a display preference and not a question about what to record.
// "exists" answers the one thing that varies: whether this database holds a
// column of theirs for that fact.
func FactChoices() ([]FactChoice, error) {
	own, err := factsWithAColumn()
	if err != nil {
		return nil, err
	}
	out := make([]FactChoice, 0, len(Facts))
	for _, f := range Facts {
		out = append(out, FactChoice{
			Key:     f.Key,
			Column:  f.Column,
			Type:    f.Type,
			About:   f.About,
			Sources: f.Sources,
			Exists:  own[f.Key],
		})
	}
	return out, nil
}

// factsWithAColumn returns the facts this catalog holds a column for, read off
// the columns that exist. Every fact's column ships, so this is close to every
// fact there is. The reader does not create a column of their own to keep a fact
// beside a source's: the source states it in the one column and an empty cell can
// be filled in there.
func factsWithAColumn() (map[string]bool, error) {
	if err := ensureShippedColumns(); err != nil {
		return nil, err
	}
	columns, err := ListColumns()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, column := range columns {
		if fact := factOfColumnKey(column.Key); fact != "" {
			out[fact] = true
		}
	}
	return out, nil
}

// RecordFacts writes what one source stated into that source's own columns, and
// answers which COLUMN KEYS it really stored, in catalogue order: the keys a read
// answers and a save takes, which for a few facts differ from the fact's own key
// (a repository's "author" is the column repo_owner). A value a column refused is
// not listed.
//
// A SOURCE OWNS ITS OWN COLUMNS, so there is nothing to decide and nothing to
// refuse: what OpenRouter says goes into an openrouter column and what a
// repository says goes into a huggingface one, and neither can reach the other's
// value or the reader's. A key this source cannot state at all is skipped
// whatever it carried, so a caller passing the wrong map cannot fill a column
// with another source's answer, and a fact this source did not state writes
// nothing.
//
// NOTHING OF THE READER'S IS TOUCHED, and that is what makes an unattended
// pass safe. The catalog refresh runs on every startup over every model with
// nobody watching; a note, a star and a column of the reader's own are cells
// no source may write, and a researched value in a source's column is its own
// cell beneath the source's, outranked rather than overwritten.
func RecordFacts(model string, source Source, read map[string]string) ([]string, error) {
	if model == "" {
		return nil, fmt.Errorf("a model name is required to record what a source stated")
	}
	stored := map[string]bool{}
	for _, key := range recordSourceFacts(model, source, read) {
		stored[key] = true
	}

	written := []string{}
	for _, f := range Facts {
		if !f.StatedBy(source) {
			continue
		}
		if strings.TrimSpace(read[f.Key]) == "" {
			continue
		}
		column := sourceCellKeyOf(f.Key, source)
		if column == "" || !stored[column] {
			continue
		}
		written = append(written, column)
	}
	return written, nil
}
