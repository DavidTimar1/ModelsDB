package catalog

import (
	"encoding/json"
	"modelsdb/internal/store"
	"reflect"
	"testing"
)

// TestWalkFeedRecordNestedObject asserts a plain nested object extends the
// path with a dot, and a JSON null still marks the path as existing but with
// no kind or sample.
func TestWalkFeedRecordNestedObject(t *testing.T) {
	m := map[string]interface{}{
		"id": "openai/gpt-5",
		"top_provider": map[string]interface{}{
			"context_length":     float64(128000),
			"moderated":          true,
			"unknown_field_here": nil,
		},
	}
	out := walkFeedRecord(m)

	if got := out["id"]; !got.hasValue || got.kind != store.ValueTypeText || got.sample != "openai/gpt-5" {
		t.Errorf("id = %+v", got)
	}
	if got := out["top_provider.context_length"]; !got.hasValue || got.kind != store.ValueTypeNumber || got.sample != "128000" {
		t.Errorf("top_provider.context_length = %+v", got)
	}
	if got := out["top_provider.moderated"]; !got.hasValue || got.kind != store.ValueTypeFlag || got.sample != "true" {
		t.Errorf("top_provider.moderated = %+v", got)
	}
	if got := out["top_provider.unknown_field_here"]; got.hasValue {
		t.Errorf("a null leaf should not read as having a value, got %+v", got)
	}
}

// TestWalkFeedRecordArrayOfScalars asserts an array of scalars is recorded as
// ONE leaf of kind "list" rather than being walked element by element, and an
// empty array marks its path as existing with no value, the same as null.
func TestWalkFeedRecordArrayOfScalars(t *testing.T) {
	m := map[string]interface{}{
		"supported_parameters": []interface{}{"temperature", "top_p"},
		"empty_list":           []interface{}{},
	}
	out := walkFeedRecord(m)

	got := out["supported_parameters"]
	if !got.hasValue || got.kind != store.ValueTypeList {
		t.Errorf("supported_parameters = %+v, want a single list leaf", got)
	}
	if got.sample != `["temperature","top_p"]` {
		t.Errorf("supported_parameters sample = %q", got.sample)
	}
	if _, walked := out["supported_parameters[]"]; walked {
		t.Errorf("an array of scalars must not be walked as [] elements")
	}
	// An empty array says nothing a cell could hold, so the path exists for
	// this record and carries no value - a column made for a path that is []
	// on every model would hold nothing.
	got, seen := out["empty_list"]
	if !seen {
		t.Errorf("empty_list is not recorded as a path at all")
	}
	if got.hasValue {
		t.Errorf("empty_list = %+v, want no value", got)
	}
}

// TestWalkFeedRecordArrayOfObjects asserts a non-empty array of objects
// becomes ONE leaf at the array's own path, of kind "list" - the same
// treatment as an array of scalars, and NOT walked into element by element,
// so no "[]" path is ever produced for it (the CEO's decision of
// 2026-09-18, override_windows.md: a list of objects is one fact, not one
// fact per key inside it).
func TestWalkFeedRecordArrayOfObjects(t *testing.T) {
	m := map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{"uptime": float64(99.9), "quantization": nil},
			map[string]interface{}{"uptime": nil, "quantization": "fp8"},
		},
	}
	out := walkFeedRecord(m)

	got := out["endpoints"]
	if !got.hasValue || got.kind != store.ValueTypeList {
		t.Errorf("endpoints = %+v, want a single list leaf", got)
	}
	if _, walked := out["endpoints[]"]; walked {
		t.Errorf("an array of objects must not itself become a [] leaf")
	}
	if _, walked := out["endpoints[].uptime"]; walked {
		t.Errorf("an array of objects must not be walked into [] elements")
	}
}

// TestFeedValuesAtPathExpandsArraySegments checks feedValuesAtPath (which
// FillFeedFieldColumn uses to read one path back out of a raw record)
// against a path that passes through "[]": it should return every non-null
// value found across every element, in element order.
func TestFeedValuesAtPathExpandsArraySegments(t *testing.T) {
	m := map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{"uptime": float64(99.9)},
			map[string]interface{}{"uptime": nil},
			map[string]interface{}{"uptime": float64(87.1)},
		},
	}
	got := feedValuesAtPath(m, "endpoints[].uptime")
	want := []interface{}{float64(99.9), float64(87.1)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("feedValuesAtPath = %v, want %v", got, want)
	}

	if got := feedValuesAtPath(m, "endpoints[].missing"); got != nil {
		t.Errorf("a path that resolves nowhere should answer nil, got %v", got)
	}
}

// TestObserveFeedFieldsCountsModelsWithAValue exercises the aggregation
// FetchModels feeds into store.RecordFeedFields: how many of several raw
// records stated a non-null value at each path.
func TestObserveFeedFieldsCountsModelsWithAValue(t *testing.T) {
	data := []map[string]interface{}{
		{"id": "a/1", "context_length": float64(8192)},
		{"id": "a/2", "context_length": nil},
		{"id": "a/3", "context_length": float64(4096)},
	}
	out := observeFeedFields(data)

	if got := out["id"]; got.Models != 3 {
		t.Errorf("id.Models = %d, want 3", got.Models)
	}
	cl := out["context_length"]
	if cl.Models != 2 {
		t.Errorf("context_length.Models = %d, want 2 (one record was null)", cl.Models)
	}
	if cl.Kind != store.ValueTypeNumber {
		t.Errorf("context_length.Kind = %q, want number", cl.Kind)
	}
}

// TestFeedValueTextReadsEachKindOfLeaf checks the pure value-selection
// helpers FillFeedFieldColumn is built from: a number from a JSON number OR
// from the quoted figure the feed sends every price as, a flag from a bool,
// and text from a string. The write itself needs a real database and is
// covered end to end in the app package (feedfields_test.go).
func TestFeedValueTextReadsEachKindOfLeaf(t *testing.T) {
	if got, ok := feedValueText(store.ValueTypeNumber, float64(37.2)); !ok || got != "37.2" {
		t.Errorf("feedValueText(number, 37.2) = %q, %v", got, ok)
	}
	if got, ok := feedValueText(store.ValueTypeNumber, "0.00000005"); !ok || got != "0.00000005" {
		t.Errorf("feedValueText(number, \"0.00000005\") = %q, %v, want the digits as sent", got, ok)
	}
	if _, ok := feedValueText(store.ValueTypeNumber, "not a number"); ok {
		t.Errorf("feedValueText(number, string) should refuse")
	}
	if got, ok := feedValueText(store.ValueTypeFlag, true); !ok || got != "yes" {
		t.Errorf("feedValueText(flag, true) = %q, %v", got, ok)
	}
	if got, ok := feedValueText(store.ValueTypeText, "Qwen"); !ok || got != "Qwen" {
		t.Errorf("feedValueText(text, Qwen) = %q, %v", got, ok)
	}
	if _, ok := feedValueText(store.ValueTypeText, nil); ok {
		t.Errorf("feedValueText(text, nil) should refuse")
	}

	if got := jsonArrOf([]interface{}{"a", "b"}); got != `["a","b"]` {
		t.Errorf("jsonArrOf = %q", got)
	}
	if got := jsonArrOf(nil); got != "" {
		t.Errorf("jsonArrOf(nil) = %q, want empty", got)
	}
}

// TestListMembersOfDoesNotNestAnArrayLeaf pins the list fill: a path whose
// leaf is itself an array resolves to ONE value that is the whole array, and
// a list column stores its members, never an array holding that array.
func TestListMembersOfDoesNotNestAnArrayLeaf(t *testing.T) {
	m := map[string]interface{}{
		"reasoning": map[string]interface{}{"supported_efforts": []interface{}{"max", "high", "low"}},
	}
	got := jsonArrOf(listMembersOf(feedValuesAtPath(m, "reasoning.supported_efforts")))
	if got != `["max","high","low"]` {
		t.Errorf("the list fill of reasoning.supported_efforts = %s, want [\"max\",\"high\",\"low\"]", got)
	}
	// Scalars reached through "[]" are members one each, and an array reached
	// through "[]" contributes its members too.
	overrides := map[string]interface{}{
		"overrides": []interface{}{
			map[string]interface{}{"utc_days": []interface{}{float64(1), float64(2)}},
			map[string]interface{}{"utc_days": []interface{}{float64(3)}},
		},
	}
	if got := jsonArrOf(listMembersOf(feedValuesAtPath(overrides, "overrides[].utc_days"))); got != `[1,2,3]` {
		t.Errorf("the list fill of overrides[].utc_days = %s, want [1,2,3]", got)
	}
	if got := jsonArrOf(listMembersOf([]interface{}{"a", "b"})); got != `["a","b"]` {
		t.Errorf("listMembersOf over scalars = %s, want [\"a\",\"b\"]", got)
	}
}

// TestObserveFeedFieldsRecordsNumericStringsAsANumber: the feed quotes every
// price, so a path every model states as a numeric string, or as a mix of
// numbers and numeric strings, is recorded as a number; one string that is
// not a figure keeps it text.
func TestObserveFeedFieldsRecordsNumericStringsAsANumber(t *testing.T) {
	data := []map[string]interface{}{
		{"warm_up_seconds": "0.000001", "note": "0.5", "mixed": "1"},
		{"warm_up_seconds": "0", "note": "free", "mixed": float64(2)},
		{"warm_up_seconds": nil, "note": nil, "mixed": nil},
	}
	out := observeFeedFields(data)
	if got := out["warm_up_seconds"]; got.Kind != store.ValueTypeNumber || got.Models != 2 {
		t.Errorf("warm_up_seconds = %+v, want kind number over 2 models", got)
	}
	if got := out["note"]; got.Kind != store.ValueTypeText {
		t.Errorf("note = %+v, want text because one model's value is not a figure", got)
	}
	if got := out["mixed"]; got.Kind != store.ValueTypeNumber {
		t.Errorf("mixed = %+v, want number when every value is a number or a numeric string", got)
	}
}

// TestFeedColumnCandidatesNamesOnlyThePathsThatNeedAColumn covers each
// exclusion in one table: a path a shipped column reads, the top-level name,
// a path with no value on any model, and a path already tied to a column all
// make nothing; a bare list-of-objects path (benchmarks.design_arena) makes
// one column, the same as any other unread path, because walkFeedRecord no
// longer produces a "[]" path underneath it to conflict with; the rest come
// back sorted.
func TestFeedColumnCandidatesNamesOnlyThePathsThatNeedAColumn(t *testing.T) {
	observed := map[string]store.FieldObservation{
		"name":                        {Kind: store.ValueTypeText, Models: 3},
		"context_length":              {Kind: store.ValueTypeNumber, Models: 3},
		"top_provider.context_length": {Kind: store.ValueTypeNumber, Models: 3},
		"benchmarks.design_arena":     {Kind: store.ValueTypeList, Models: 2},
		"per_request_limits":          {Kind: store.ValueTypeText, Models: 0},
		"reasoning.supported_efforts": {Kind: store.ValueTypeList, Models: 2},
		"alias_target.name":           {Kind: store.ValueTypeText, Models: 1},
	}
	tied := map[string]bool{"reasoning.supported_efforts": true}
	got := feedColumnCandidates(observed, tied)
	want := []string{"alias_target.name", "benchmarks.design_arena", "top_provider.context_length"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("feedColumnCandidates = %v, want %v", got, want)
	}
}

// TestOrderedObjectPairsKeepsTheFeedsOwnKeyOrder proves the one thing a Go
// map cannot: orderedObjectPairs reads an object's keys back in the exact
// order the feed's bytes carried them, not sorted and not Go's random map
// order. It also asserts a nested array survives as its own raw bytes,
// untouched, for memberFieldText to compact afterwards.
func TestOrderedObjectPairsKeepsTheFeedsOwnKeyOrder(t *testing.T) {
	raw := json.RawMessage(`{"prompt":0.00000156,"completion":0.0000078,"min_prompt_tokens":32000,"utc_days":["mon","tue"]}`)
	pairs, err := orderedObjectPairs(raw)
	if err != nil {
		t.Fatalf("orderedObjectPairs: %v", err)
	}
	wantKeys := []string{"prompt", "completion", "min_prompt_tokens", "utc_days"}
	if len(pairs) != len(wantKeys) {
		t.Fatalf("orderedObjectPairs = %d pair(s), want %d", len(pairs), len(wantKeys))
	}
	for i, want := range wantKeys {
		if pairs[i].key != want {
			t.Errorf("pair %d key = %q, want %q (feed order broken)", i, pairs[i].key, want)
		}
	}
}

// TestMemberFieldTextRendersEachKindOfValue: a string unquoted, a number or
// bool as the literal bytes the feed sent (so a price keeps its exact
// digits), a nested array or object as compact JSON, and null or absent as
// no text at all.
func TestMemberFieldTextRendersEachKindOfValue(t *testing.T) {
	cases := []struct {
		raw    string
		want   string
		wantOK bool
	}{
		{`"fp8"`, "fp8", true},
		{`0.00000156`, "0.00000156", true},
		{`32000`, "32000", true},
		{`true`, "true", true},
		{`["mon","tue"]`, `["mon","tue"]`, true},
		{`{"a": 1, "b": 2}`, `{"a":1,"b":2}`, true},
		{`null`, "", false},
	}
	for _, c := range cases {
		got, ok := memberFieldText(json.RawMessage(c.raw))
		if ok != c.wantOK || got != c.want {
			t.Errorf("memberFieldText(%s) = %q, %v; want %q, %v", c.raw, got, ok, c.want, c.wantOK)
		}
	}
}

// TestMemberTextJoinsKeyValuePairsInFeedOrderSkippingAbsent pins the exact
// shape a price window renders as: "key=value" pairs, joined by ", ", in the
// feed's own key order, a null or missing key left out rather than written as
// "key=" or "key=null" - the Qwen3 Max window 0 example from the decision
// (override_windows.md): prompt=0.00000156, completion=0.0000078,
// min_prompt_tokens=32000, no utc_start on this window.
func TestMemberTextJoinsKeyValuePairsInFeedOrderSkippingAbsent(t *testing.T) {
	raw := json.RawMessage(`{"prompt":0.00000156,"completion":0.0000078,"min_prompt_tokens":32000,"utc_start":null}`)
	got, err := memberText(raw)
	if err != nil {
		t.Fatalf("memberText: %v", err)
	}
	want := "prompt=0.00000156, completion=0.0000078, min_prompt_tokens=32000"
	if got != want {
		t.Errorf("memberText = %q, want %q", got, want)
	}
}

// TestObjectArrayMembersOneMemberPerElementInArrayOrder walks a two-window
// array (Qwen3 Max's shape: window 0 keyed by min_prompt_tokens, window 1 by
// a different min_prompt_tokens and a higher price) and checks both members
// come back whole, in array order, each its own key=value join.
func TestObjectArrayMembersOneMemberPerElementInArrayOrder(t *testing.T) {
	raw := json.RawMessage(`{"pricing":{"overrides":[
		{"prompt":0.00000156,"completion":0.0000078,"min_prompt_tokens":32000},
		{"prompt":0.00000195,"completion":0.00000975,"min_prompt_tokens":128000}
	]}}`)
	members, err := objectArrayMembers(raw, "pricing.overrides")
	if err != nil {
		t.Fatalf("objectArrayMembers: %v", err)
	}
	want := []interface{}{
		"prompt=0.00000156, completion=0.0000078, min_prompt_tokens=32000",
		"prompt=0.00000195, completion=0.00000975, min_prompt_tokens=128000",
	}
	if !reflect.DeepEqual(members, want) {
		t.Errorf("objectArrayMembers = %v, want %v", members, want)
	}
}

// TestObjectArrayLeafDistinguishesObjectArraysFromScalarArrays: the check
// FillFeedFieldColumn uses to choose between the raw, order-preserving fill
// (an array of objects) and the ordinary listMembersOf fill (an array of
// scalars, or a "[]"-expanded path with several values).
func TestObjectArrayLeafDistinguishesObjectArraysFromScalarArrays(t *testing.T) {
	objectArray := []interface{}{
		[]interface{}{map[string]interface{}{"prompt": float64(1)}},
	}
	if !objectArrayLeaf(objectArray) {
		t.Errorf("objectArrayLeaf(%v) = false, want true", objectArray)
	}
	scalarArray := []interface{}{[]interface{}{"max", "high", "low"}}
	if objectArrayLeaf(scalarArray) {
		t.Errorf("objectArrayLeaf(%v) = true, want false", scalarArray)
	}
	expandedThroughBrackets := []interface{}{float64(99.9), float64(87.1)}
	if objectArrayLeaf(expandedThroughBrackets) {
		t.Errorf("objectArrayLeaf(%v) = true, want false", expandedThroughBrackets)
	}
}
