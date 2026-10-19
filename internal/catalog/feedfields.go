package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"sort"
	"strconv"
	"strings"
)

// leafSighting is one field path's contribution from a single feed record:
// whether the record carried a non-null value under that path, its JSON kind,
// one sample value to show, and whether every non-null value it carried there
// reads as a number. Multiple array elements under the same "[]" path collapse
// into one leafSighting per record, because feed_fields counts MODELS that
// state a field, not repetitions inside one model's own record.
type leafSighting struct {
	hasValue bool
	kind     string
	sample   string
	// numeric is true while every non-null value this record carried at the
	// path was a JSON number or a string that parses as one. The feed sends
	// every price as a string ("0.000001"), and a column of prices that sorts
	// by letters is useless, so observeFeedFields records such a path as a
	// number when every model agrees.
	numeric bool
}

// objectAsOneListPath is the one bare object path the walker never recurses
// into: see the comment on walkFeedValue's map case.
const objectAsOneListPath = "pricing_skus"

// walkFeedRecord walks one raw OpenRouter feed record into every leaf path it
// carries. An object's keys extend the path with a dot; ANY non-empty array -
// of scalars or of objects alike - is ONE leaf of kind "list" at the array's
// own path, never walked into element by element, so a list of price windows
// or leaderboard rows is one fact, not one fact per key inside it, and one
// window's price can never be paired with another window's hours. A leaf that is
// JSON null, or
// an EMPTY array, still marks its path as existing, but with no kind or
// sample of its own - see recordLeaf: a model sending [] states nothing a
// cell could hold, and a column made for a path that is [] everywhere would
// hold nothing.
func walkFeedRecord(m map[string]interface{}) map[string]leafSighting {
	out := map[string]leafSighting{}
	walkFeedValue("", m, out)
	return out
}

func walkFeedValue(prefix string, v interface{}, out map[string]leafSighting) {
	switch val := v.(type) {
	case map[string]interface{}:
		// pricing_skus (the video listing) states a different SET OF KEYS per
		// model - up to 34 distinct keys across 29 models, most stated by only
		// one or two - so walking it like an ordinary object would make one
		// sparse column per key. A price list is one fact, the same reason an
		// array of price windows gets one column: this object becomes ONE list
		// leaf at its own path, never walked key by key.
		if prefix == objectAsOneListPath {
			if len(val) == 0 {
				recordLeaf(prefix, out, "", "")
				return
			}
			recordLeaf(prefix, out, store.ValueTypeList, feedSample(v))
			return
		}
		for key, child := range val {
			walkFeedValue(joinFeedPath(prefix, key), child, out)
		}
	case []interface{}:
		if len(val) == 0 {
			recordLeaf(prefix, out, "", "")
			return
		}
		recordLeaf(prefix, out, store.ValueTypeList, feedSample(v))
	default:
		recordLeaf(prefix, out, feedKind(v), feedSample(v))
	}
}

func joinFeedPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// recordLeaf merges one occurrence of path into out. A non-null occurrence
// (kind != "") marks the path as having a value this record and records its
// kind and a sample; a null occurrence leaves a prior non-null occurrence
// under the same path (from an earlier array element) untouched, so a path
// under "[]" reads as having a value for this record when ANY element does.
func recordLeaf(path string, out map[string]leafSighting, kind, sample string) {
	cur := out[path]
	if kind != "" {
		numeric := kind == store.ValueTypeNumber || (kind == store.ValueTypeText && isNumericText(sample))
		if cur.hasValue {
			numeric = numeric && cur.numeric
		}
		cur.hasValue = true
		cur.kind = kind
		cur.sample = sample
		cur.numeric = numeric
	}
	out[path] = cur
}

// isNumericText reports whether s is a string a number column would take: the
// feed quotes every price as one.
func isNumericText(s string) bool {
	_, err := shared.ParseNumber(strings.TrimSpace(s))
	return err == nil
}

// feedKind maps one JSON scalar to the value type feed_fields records it
// under. A JSON null carries no type of its own, so it maps to "" - the
// signal to recordLeaf that this occurrence contributes no kind or sample.
func feedKind(v interface{}) string {
	switch v.(type) {
	case bool:
		return store.ValueTypeFlag
	case float64:
		return store.ValueTypeNumber
	case string:
		return store.ValueTypeText
	default:
		return ""
	}
}

// feedSample renders one value the way a sample_value cell shows it: a string
// verbatim, anything else (a number, a bool, a list) as the text that value's
// own JSON encoding reads as.
func feedSample(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// feedValuesAtPath walks m along path's dotted segments, expanding a "[]"-
// suffixed segment across every element of the array found there, and answers
// every non-null leaf value reached this way - more than one when path passes
// through "[]" and more than one element states it, none when path does not
// resolve in m at all or every value it resolves to is null.
func feedValuesAtPath(m map[string]interface{}, path string) []interface{} {
	return collectFeedValues(m, strings.Split(path, "."))
}

func collectFeedValues(v interface{}, segments []string) []interface{} {
	if len(segments) == 0 {
		if v == nil {
			return nil
		}
		return []interface{}{v}
	}
	seg := segments[0]
	rest := segments[1:]
	key := strings.TrimSuffix(seg, "[]")
	isArray := key != seg

	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	child, present := obj[key]
	if !present {
		return nil
	}
	if !isArray {
		return collectFeedValues(child, rest)
	}
	arr, ok := child.([]interface{})
	if !ok {
		return nil
	}
	var out []interface{}
	for _, elem := range arr {
		out = append(out, collectFeedValues(elem, rest)...)
	}
	return out
}

// feedValueText renders one JSON leaf value as the text a column of valueType
// stores, or answers false when v cannot be read as that kind (a number
// column asked for a value that is not a number, or a flag column asked for
// one that is not a bool).
func feedValueText(valueType string, v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch valueType {
	case store.ValueTypeNumber:
		switch n := v.(type) {
		case float64:
			return strconv.FormatFloat(n, 'f', -1, 64), true
		case string:
			// The feed quotes every price as a string. The digits are kept as
			// sent rather than re-rendered through a float, so "0.00000005"
			// stays "0.00000005".
			n = strings.TrimSpace(n)
			if !isNumericText(n) {
				return "", false
			}
			return n, true
		}
		return "", false
	case store.ValueTypeFlag:
		b, ok := v.(bool)
		if !ok {
			return "", false
		}
		if b {
			return "yes", true
		}
		return "no", true
	default: // ValueTypeText
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s == "" {
				return "", false
			}
			return s, true
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// FillFeedFieldColumn writes columnKey, sourced SourceOpenRouter, on every
// model in the library that states a value at path, from the feed records
// already fetched (data) and their raw JSON bytes (raw, the same records in
// the same order - see fetchFeedFrom). FetchModels runs it on every refresh for
// every column tied to a feed path, the ones that refresh has just created
// included, so a column made from the feed is filled the moment it exists and
// stays current like any other OpenRouter-fed one.
//
// A LIST COLUMN whose leaf is an array of OBJECTS (a price window, a
// leaderboard row) takes one member per element - see objectArrayMembers,
// which needs raw to read each element's keys in the order the feed itself
// sent them. A list column whose leaf is an array of scalars, or reached
// through a "[]" segment, takes every value feedValuesAtPath finds as one
// JSON array (listMembersOf); every other kind takes the first value found.
func FillFeedFieldColumn(data []map[string]interface{}, raw []json.RawMessage, path, columnKey, valueType string, inLibrary map[string]bool) (written int, err error) {
	// One model whose value cannot be read or written costs only that model: the
	// failures are collected and returned together after every other model has
	// been filled.
	var failures []error
	for i, m := range data {
		name := shared.GetStr(m, "name")
		if name == "" || !inLibrary[name] {
			continue
		}
		vals := feedValuesAtPath(m, path)
		if len(vals) == 0 {
			continue
		}
		var text string
		if valueType == store.ValueTypeList {
			var members []interface{}
			switch {
			case flatObjectLeaf(vals):
				members, err = flatObjectMembers(raw[i], path)
				if err != nil {
					failures = append(failures, fmt.Errorf("read %s for %q in the feed's own key order: %w", path, name, err))
					continue
				}
			case objectArrayLeaf(vals):
				members, err = objectArrayMembers(raw[i], path)
				if err != nil {
					failures = append(failures, fmt.Errorf("read %s for %q in the feed's own key order: %w", path, name, err))
					continue
				}
			default:
				members = listMembersOf(vals)
			}
			text = jsonArrOf(members)
			if text == "" {
				continue
			}
		} else {
			t, ok := feedValueText(valueType, vals[0])
			if !ok {
				continue
			}
			text = t
		}
		if err := store.WriteCells(store.SourceOpenRouter, name, map[string]string{columnKey: text}); err != nil {
			failures = append(failures, fmt.Errorf("fill %s for %q: %w", columnKey, name, err))
			continue
		}
		written++
	}
	return written, errors.Join(failures...)
}

// flatObjectLeaf reports whether vals is the single value of a bare FLAT
// OBJECT leaf (pricing_skus): one value, itself a JSON object rather than an
// array. Such a leaf needs flatObjectMembers, not objectArrayMembers, because
// there is no array to take elements from - the object's own keys are the
// members.
func flatObjectLeaf(vals []interface{}) bool {
	if len(vals) != 1 {
		return false
	}
	_, ok := vals[0].(map[string]interface{})
	return ok
}

// flatObjectMembers reads the object at path out of one record's raw JSON
// bytes and answers one member per key, in the feed's own key order: "key:
// value". pricing_skus (the video listing's per-SKU price list) is the one path
// whose keys are themselves the members - see objectAsOneListPath in
// walkFeedValue. A key whose value is null, or an empty object, contributes
// nothing.
func flatObjectMembers(raw json.RawMessage, path string) ([]interface{}, error) {
	obj, err := navigateRawPath(raw, path)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	pairs, err := orderedObjectPairs(obj)
	if err != nil {
		return nil, err
	}
	members := make([]interface{}, 0, len(pairs))
	for _, p := range pairs {
		v, ok := memberFieldText(p.value)
		if !ok {
			continue
		}
		members = append(members, p.key+": "+v)
	}
	return members, nil
}

// objectArrayLeaf reports whether vals - what feedValuesAtPath found at a
// list column's path - is the single value of a bare array-of-objects leaf
// (pricing.overrides, benchmarks.design_arena): one value, itself an array,
// whose own first element is an object. Such a leaf needs objectArrayMembers,
// not listMembersOf, because a window's own key order only survives in the
// raw JSON bytes, never in the parsed map.
func objectArrayLeaf(vals []interface{}) bool {
	if len(vals) != 1 {
		return false
	}
	arr, ok := vals[0].([]interface{})
	if !ok || len(arr) == 0 {
		return false
	}
	_, ok = arr[0].(map[string]interface{})
	return ok
}

// objectArrayMembers reads the array at path out of one record's raw JSON
// bytes and answers one member per element, in element order: each element's
// own key=value pairs, joined by ", ", in the exact order the feed sent them
// - not the arbitrary order Go's map[string]interface{} would range them in,
// which is why this reads raw rather than the already-parsed record. A key
// whose value is null, or absent, contributes nothing; a nested array or
// object renders as compact JSON.
func objectArrayMembers(raw json.RawMessage, path string) ([]interface{}, error) {
	elems, err := rawArrayElements(raw, path)
	if err != nil {
		return nil, err
	}
	members := make([]interface{}, 0, len(elems))
	for _, elem := range elems {
		text, err := memberText(elem)
		if err != nil {
			return nil, err
		}
		if text != "" {
			members = append(members, text)
		}
	}
	return members, nil
}

// navigateRawPath walks raw along path's dot-separated segments (path
// carries no "[]" here: it is the leaf's own bare path) and answers the raw
// JSON bytes found there, or nil when a segment is absent. Only the bytes at
// each step matter to the callers below, so navigation reads each level as
// map[string]json.RawMessage: a plain, order-blind lookup of the one key
// wanted at each step. objectArrayMembers and flatObjectMembers are its two
// callers, for an array-of-objects leaf and a flat-object leaf respectively.
func navigateRawPath(raw json.RawMessage, path string) (json.RawMessage, error) {
	cur := raw
	for _, seg := range strings.Split(path, ".") {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(cur, &obj); err != nil {
			return nil, fmt.Errorf("read %q as an object: %w", seg, err)
		}
		child, present := obj[seg]
		if !present {
			return nil, nil
		}
		cur = child
	}
	return cur, nil
}

// rawArrayElements navigates raw to path and answers the array found there as
// its elements' own raw JSON, or nil when the path resolves nowhere.
func rawArrayElements(raw json.RawMessage, path string) ([]json.RawMessage, error) {
	cur, err := navigateRawPath(raw, path)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(cur, &arr); err != nil {
		return nil, fmt.Errorf("read the array at %q: %w", path, err)
	}
	return arr, nil
}

// memberText renders one array element (one price window, one leaderboard
// row) as its own key=value pairs, joined by ", ", walking its keys with
// orderedObjectPairs so they come out in the feed's own order rather than a
// Go map's.
func memberText(elem json.RawMessage) (string, error) {
	pairs, err := orderedObjectPairs(elem)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		v, ok := memberFieldText(p.value)
		if !ok {
			continue
		}
		parts = append(parts, p.key+"="+v)
	}
	return strings.Join(parts, ", "), nil
}

// orderedPair is one key and its raw JSON value, in the order orderedObjectPairs
// read them off the wire.
type orderedPair struct {
	key   string
	value json.RawMessage
}

// orderedObjectPairs walks a JSON object's own bytes token by token, the one
// way encoding/json preserves source key order: Unmarshal into a Go map does
// not, because a map has none. raw must be a JSON object; anything else is an
// error.
func orderedObjectPairs(raw json.RawMessage) ([]orderedPair, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected a JSON object, got %v", tok)
	}
	var pairs []orderedPair
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected a string key, got %v", keyTok)
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, fmt.Errorf("read the value of %q: %w", key, err)
		}
		pairs = append(pairs, orderedPair{key: key, value: val})
	}
	if _, err := dec.Token(); err != nil { // consume the closing '}'
		return nil, err
	}
	return pairs, nil
}

// memberFieldText renders one field's raw JSON value as the text
// key=value puts after the "=": a JSON string unquoted, a number or bool
// verbatim (the raw bytes ARE the literal digits the feed sent, so a price
// keeps its exact precision with no float round-trip), a nested array or
// object as compact JSON, and null or absent as no text at all - the field is
// left out of the join rather than written as "key=" or "key=null".
func memberFieldText(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", false
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", false
		}
		return s, true
	case '{', '[':
		var compact bytes.Buffer
		if err := json.Compact(&compact, trimmed); err != nil {
			return "", false
		}
		return compact.String(), true
	default:
		return string(trimmed), true
	}
}

// listMembersOf flattens what feedValuesAtPath found into the members a list
// column stores. A path whose leaf is itself an array ("reasoning.supported_efforts"
// is ["max","high","low"]) resolves to ONE value that is the whole array, and
// storing that value as a member would nest it: [["max","high","low"]], which
// dbcore.ParseArr reads back as a single member. So an array found at the leaf
// contributes its members, and a scalar contributes itself.
func listMembersOf(vals []interface{}) []interface{} {
	var out []interface{}
	for _, v := range vals {
		if members, ok := v.([]interface{}); ok {
			out = append(out, members...)
			continue
		}
		out = append(out, v)
	}
	return out
}

// jsonArrOf JSON-encodes vals as the text a list column stores, or "" when
// there is nothing to encode.
func jsonArrOf(vals []interface{}) string {
	if len(vals) == 0 {
		return ""
	}
	b, err := json.Marshal(vals)
	if err != nil {
		return ""
	}
	return string(b)
}

// observeFeedFields walks every raw record the feed carried this refresh and
// aggregates them into one FieldObservation per leaf path: how many records
// stated a non-null value for it this refresh, its kind, and a sample drawn
// from one of the records that had one.
//
// A PATH EVERY MODEL STATES AS A NUMBER OR A NUMERIC STRING IS RECORDED AS A
// NUMBER, whatever kind the last record said: the column made from it then
// sorts by the figure, and FillFeedFieldColumn accepts the quoted figure as
// the number it parses to.
func observeFeedFields(data []map[string]interface{}) map[string]store.FieldObservation {
	out := map[string]store.FieldObservation{}
	numeric := map[string]bool{}
	for _, m := range data {
		for path, leaf := range walkFeedRecord(m) {
			obs := out[path]
			if leaf.hasValue {
				if obs.Models == 0 {
					numeric[path] = leaf.numeric
				} else {
					numeric[path] = numeric[path] && leaf.numeric
				}
				obs.Models++
				obs.Kind = leaf.kind
				if obs.Sample == "" {
					obs.Sample = leaf.sample
				}
			}
			out[path] = obs
		}
	}
	for path, obs := range out {
		if obs.Models > 0 && numeric[path] {
			obs.Kind = store.ValueTypeNumber
			out[path] = obs
		}
	}
	return out
}

// feedColumnCandidates names, sorted, every observed path the refresh makes a
// column for: one some model states a value at, that no shipped column or the
// model row already reads (feedPathIsRead), that is not tied to a column yet
// (tiedPaths), and that is not also a prefix of other paths - a path that is
// both a leaf and a branch ("benchmarks.design_arena" is [] on some models and
// an array of objects on others) gets no column of its own, only its leaves do.
//
// A path with no non-null value on any model this refresh makes nothing: the
// column would hold nothing, and it is made on the first refresh some model
// states it.
func feedColumnCandidates(observed map[string]store.FieldObservation, tiedPaths map[string]bool) []string {
	var out []string
	for path, obs := range observed {
		if obs.Models == 0 || tiedPaths[path] || feedPathIsRead(path) || isPrefixOfAnotherPath(path, observed) {
			continue
		}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// isPrefixOfAnotherPath reports whether some other observed path continues
// past path, through an object key or through an array of objects.
func isPrefixOfAnotherPath(path string, observed map[string]store.FieldObservation) bool {
	for other := range observed {
		if strings.HasPrefix(other, path+".") || strings.HasPrefix(other, path+"[]") {
			return true
		}
	}
	return false
}
