package dbcore

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// This file builds the Architecture line migration v52 writes from the facts a
// HuggingFace import measured. The line has two sentences, each clause present
// only when its facts are:
//
//	A: layers, what holds a cache, recurrent layers, head figures, cache bytes.
//	B: class, weight dtype and file format, quantization and bits.
//
// A figure the importer got wrong is left out rather than written; see the
// migration's own comment for which rows that is.

// archFacts is one model's folded facts, keyed by the column key each came from.
type archFacts map[string]string

// number reads a fact as a positive finite number, and answers false for a
// missing, unreadable, zero or negative one.
func (f archFacts) number(key string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(f[key]), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, false
	}
	return n, true
}

// layerTypes lists the kind of each layer, in order, where the repository lists
// them.
func (f archFacts) layerTypes() []string {
	var out []string
	for _, part := range strings.Split(f["layer_types"], ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// cacheFiguresAreUnsound reports a row whose stored cache figures over-state the
// cache: a sliding-window layer is counted as holding a full-length cache, and
// a Qwen3-Next style model's attention is counted the same way.
func (f archFacts) cacheFiguresAreUnsound() bool {
	for _, t := range f.layerTypes() {
		if t == "sliding_attention" {
			return true
		}
	}
	return strings.HasPrefix(f["architecture_class"], "Qwen3Next")
}

// count writes a whole number with thousands commas, and any other number as it
// reads.
func count(n float64) string {
	if n == math.Trunc(n) && math.Abs(n) < 1e15 {
		s := strconv.FormatInt(int64(n), 10)
		var out []byte
		for i := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				out = append(out, ',')
			}
			out = append(out, s[i])
		}
		return string(out)
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// architectureLine builds the full line for a model that has none, or whose
// entry says only "unknown". It answers "" when the facts give nothing to say.
func architectureLine(f archFacts) string {
	sentences := []string{}
	if a := sentence(sentenceAClauses(f)); a != "" {
		sentences = append(sentences, a)
	}
	if b := sentence(weightClauses(f, "")); b != "" {
		sentences = append(sentences, b)
	}
	return strings.Join(sentences, " ")
}

// sentence joins clauses with "; ", capitalises the first letter and ends with a
// full stop, and answers "" for no clauses.
func sentence(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	s := strings.Join(clauses, "; ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

// sentenceAClauses are the layer, cache and head clauses.
func sentenceAClauses(f archFacts) []string {
	var clauses []string
	unsound := f.cacheFiguresAreUnsound()
	types := f.layerTypes()
	hidden, haveHidden := f.number("hidden_layers")
	attn, haveAttn := f.number("attention_layers")
	kv, haveKV := f.number("kv_bytes_per_token")
	heads, haveHeads := f.number("kv_heads")
	dim, haveDim := f.number("head_dim")

	var layers string
	if haveHidden {
		layers = count(hidden) + " layers"
		if len(types) > 0 {
			layers += " (" + layerTypeCounts(types) + ")"
		}
	}
	var cache string
	switch {
	case unsound || !haveAttn:
	case haveKV && haveHidden && attn == hidden && len(types) == 0:
		cache = "all hold a KV cache"
	case haveKV && haveHidden:
		cache = count(attn) + " hold a KV cache"
	case haveKV:
		layers = count(attn) + " layers hold a KV cache"
	case haveHidden:
		cache = count(attn) + " attention layers"
	default:
		layers = count(attn) + " attention layers"
	}
	switch {
	case layers != "" && cache != "":
		clauses = append(clauses, layers+", "+cache)
	case layers != "":
		clauses = append(clauses, layers)
	}

	if rec, ok := f.number("recurrent_layers"); ok {
		clauses = append(clauses, count(rec)+" recurrent layers")
	} else if noRecurrentState(f, types, haveAttn) {
		clauses = append(clauses, "no recurrent state")
	}

	hiddenSize, haveSize := f.number("hidden_size")
	attnHeads, haveAttnHeads := f.number("attention_heads")
	var width []string
	if haveSize {
		width = append(width, "hidden size "+count(hiddenSize))
	}
	if haveAttnHeads {
		width = append(width, count(attnHeads)+" attention heads")
	}
	var headClause []string
	if !unsound && haveDim {
		headClause = append(headClause, "head dim "+count(dim))
	}
	if !unsound && haveHeads {
		headClause = append(headClause, count(heads)+" KV heads")
	}
	switch {
	case len(headClause) > 0 && len(width) > 0:
		clauses = append(clauses, strings.Join(headClause, ", ")+" ("+strings.Join(width, ", ")+")")
	case len(headClause) > 0:
		clauses = append(clauses, strings.Join(headClause, ", "))
	case len(width) > 0:
		clauses = append(clauses, strings.Join(width, ", "))
	}

	if !unsound && haveKV {
		clauses = append(clauses, "KV cache "+count(kv)+" bytes per token")
	}
	if state, ok := f.number("state_bytes_per_layer"); ok {
		if _, recurrent := f.number("recurrent_layers"); recurrent {
			clauses = append(clauses, "recurrent state "+count(state)+" bytes per recurrent layer")
		}
	}
	return clauses
}

// noRecurrentState reports that a row with attention layers and no recurrent
// layers has no recurrent state: every listed layer type is an attention kind,
// and the row is not a Qwen3-Next style model whose cell may simply be missing.
func noRecurrentState(f archFacts, types []string, haveAttn bool) bool {
	if strings.HasPrefix(f["architecture_class"], "Qwen3Next") {
		return false
	}
	if strings.TrimSpace(f["recurrent_layers"]) == "0" {
		return true
	}
	if !haveAttn {
		return false
	}
	for _, t := range types {
		if t != "full_attention" && t != "sliding_attention" {
			return false
		}
	}
	return true
}

// layerTypeCounts reads "30 linear_attention, 10 full_attention": the most
// common kind first, ties by name.
func layerTypeCounts(types []string) string {
	counts := map[string]int{}
	for _, t := range types {
		counts[t]++
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%d %s", counts[n], n)
	}
	return strings.Join(parts, ", ")
}

// weightClauses are the class, weights and quantization clauses. With text set,
// a clause whose exact word text already holds is left out; with text empty,
// every clause is written. Bits per weight appear only below 16, because the
// importer fell back to the dtype's width where a quantization block states its
// bits somewhere it did not read.
func weightClauses(f archFacts, text string) []string {
	var clauses []string
	has := func(word string) bool { return text != "" && hasWord(text, word) }

	if class := f["architecture_class"]; class != "" && !has(class) {
		clauses = append(clauses, "class "+class)
	}
	dtype, format := f["weight_dtype"], f["weight_file_format"]
	needDtype := dtype != "" && !has(dtype)
	needFormat := format != "" && !has(format)
	switch {
	case needDtype && needFormat:
		clauses = append(clauses, "weights "+dtype+" in "+format)
	case needDtype:
		clauses = append(clauses, "weights "+dtype)
	case needFormat:
		clauses = append(clauses, "weights in "+format)
	}

	method := f["quantization_method"]
	needMethod := method != "" && !has(method)
	var bitsText string
	if bits, ok := f.number("declared_weight_bits"); ok && bits < 16 {
		phrase := count(bits) + " bits per weight"
		if !has(count(bits) + " bits") {
			bitsText = phrase
		}
	}
	switch {
	case needMethod && bitsText != "":
		clauses = append(clauses, "quantization "+method+", "+bitsText)
	case needMethod:
		clauses = append(clauses, "quantization "+method)
	case bitsText != "":
		clauses = append(clauses, bitsText)
	}
	return clauses
}

// architectureAddition is what an entry that already says something gains: the
// hidden size and attention heads, and the weights facts, each only where its
// exact word is missing from the entry. It never touches the layer and cache
// figures an entry states, because the entries were checked by hand and the
// stored figures were not always right. It answers "" when nothing is missing.
func architectureAddition(f archFacts, entry string) string {
	var clauses []string
	var width []string
	if size, ok := f.number("hidden_size"); ok && !strings.Contains(strings.ToLower(entry), "hidden size") {
		width = append(width, "hidden size "+count(size))
	}
	if heads, ok := f.number("attention_heads"); ok && !strings.Contains(strings.ToLower(entry), "attention heads") {
		width = append(width, count(heads)+" attention heads")
	}
	if len(width) > 0 {
		clauses = append(clauses, strings.Join(width, ", "))
	}
	clauses = append(clauses, weightClauses(f, entry)...)
	return sentence(clauses)
}

// hasWord reports whether text holds word as a whole word, ignoring case, so
// float16 is not found inside bfloat16.
func hasWord(text, word string) bool {
	text, word = strings.ToLower(text), strings.ToLower(word)
	for from := 0; ; {
		i := strings.Index(text[from:], word)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(word)
		if (start == 0 || !isWordByte(text[start-1])) && (end == len(text) || !isWordByte(text[end])) {
			return true
		}
		from = start + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z')
}
