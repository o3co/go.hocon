// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import (
	"testing"
)

// makeScalar is a test helper for building ScalarVal pointers quickly.
func makeScalar(s string) *ScalarVal { return &ScalarVal{Raw: s, Type: ScalarString} }

// makeObj builds an ObjectVal from a slice of key/string pairs.
// Keys appear in slice order, values are ScalarString.
func makeObj(pairs ...string) *ObjectVal {
	obj := newObjectVal()
	for i := 0; i+1 < len(pairs); i += 2 {
		obj.set(pairs[i], makeScalar(pairs[i+1]))
	}
	return obj
}

// scalarStrings extracts the Raw string from each element of arr.
func scalarStrings(arr *ArrayVal) []string {
	if arr == nil {
		return nil
	}
	out := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*ScalarVal)
		if !ok {
			out[i] = "<non-scalar>"
			continue
		}
		out[i] = sv.Raw
	}
	return out
}

// ── eligibility table ─────────────────────────────────────────────────────────

func TestNumericObjectToArray_Eligibility(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantIn  bool // should the key appear in the result?
		comment string
	}{
		// eligible
		{"zero", "0", true, "canonical zero"},
		{"one", "1", true, "single digit"},
		{"ten", "10", true, "two digits"},
		{"max_i32", "2147483647", true, "INT32_MAX"},

		// rejected — leading sign
		{"plus_one", "+1", false, "leading +"},
		{"minus_one", "-1", false, "leading -"},
		{"minus_zero", "-0", false, "leading - on zero"},

		// rejected — leading zero
		{"double_zero", "00", false, "leading zero (double zero)"},
		{"zero_one", "01", false, "leading zero (01)"},
		{"zero_zero_seven", "007", false, "leading zero (007)"},

		// rejected — whitespace
		{"space_one", " 1", false, "leading space"},
		{"one_space", "1 ", false, "trailing space"},

		// rejected — empty
		{"empty", "", false, "empty key"},

		// rejected — overflow
		{"overflow", "2147483648", false, "INT32_MAX + 1"},
		{"big", "99999999999", false, "large overflow"},

		// rejected — non-decimal forms
		{"hex", "0x1", false, "hex prefix"},
		{"exp", "1e2", false, "exponential"},
		{"decimal", "1.0", false, "decimal point"},
		{"binary", "0b10", false, "binary prefix"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build an object with the test key (value="TARGET") plus a guaranteed-eligible
			// baseline key "999" (value="BASELINE") so the object always has at least one
			// eligible key and conversion does not short-circuit for unrelated reasons.
			// Using "999" avoids collision with any of the test keys above.
			obj := newObjectVal()
			obj.set(tc.key, makeScalar("TARGET"))
			obj.set("999", makeScalar("BASELINE"))

			arr, ok := numericObjectToArray(obj)
			if !ok {
				t.Fatalf("unexpected nil result (object has the '999' baseline key)")
			}

			targetFound := false
			for _, elem := range arr.Elements {
				if sv, isSV := elem.(*ScalarVal); isSV && sv.Raw == "TARGET" {
					targetFound = true
				}
			}
			if tc.wantIn && !targetFound {
				t.Errorf("key %q (%s): expected TARGET in result, not found", tc.key, tc.comment)
			}
			if !tc.wantIn && targetFound {
				t.Errorf("key %q (%s): expected TARGET NOT in result, but it was found", tc.key, tc.comment)
			}
		})
	}
}

// ── nil / empty input ────────────────────────────────────────────────────────

func TestNumericObjectToArray_NilInput(t *testing.T) {
	arr, ok := numericObjectToArray(nil)
	if ok || arr != nil {
		t.Errorf("nil input: expected (nil,false), got (%v,%v)", arr, ok)
	}
}

func TestNumericObjectToArray_EmptyObject(t *testing.T) {
	arr, ok := numericObjectToArray(newObjectVal())
	if ok || arr != nil {
		t.Errorf("empty object: expected (nil,false) per S15.4, got (%v,%v)", arr, ok)
	}
}

// ── S15.5: non-integer keys ignored ──────────────────────────────────────────

func TestNumericObjectToArray_NonIntKeysIgnored(t *testing.T) {
	obj := makeObj("0", "a", "foo", "IGNORED", "1", "c")
	arr, ok := numericObjectToArray(obj)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	got := scalarStrings(arr)
	want := []string{"a", "c"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

// All-non-int keys → no conversion (helper contract; no specific S-row).
// Spec rows S15.1–S15.7 cover spec-defined cases; this is helper-internal coverage.
func TestNumericObjectToArray_AllNonInt(t *testing.T) {
	obj := makeObj("foo", "a", "bar", "b")
	arr, ok := numericObjectToArray(obj)
	if ok || arr != nil {
		t.Errorf("all-non-int: expected (nil,false), got (%v,%v)", arr, ok)
	}
}

// ── S15.6: gap compaction ────────────────────────────────────────────────────

func TestNumericObjectToArray_GapCompaction(t *testing.T) {
	obj := makeObj("0", "a", "2", "c")
	arr, ok := numericObjectToArray(obj)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	got := scalarStrings(arr)
	want := []string{"a", "c"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("gap compaction: got %v want %v", got, want)
	}
}

// ── S15.7: sort by integer key ───────────────────────────────────────────────

func TestNumericObjectToArray_SortByKey(t *testing.T) {
	obj := makeObj("1", "b", "0", "a")
	arr, ok := numericObjectToArray(obj)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	got := scalarStrings(arr)
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("sort: got %v want %v", got, want)
	}
}

// ── basic happy path ─────────────────────────────────────────────────────────

func TestNumericObjectToArray_Basic(t *testing.T) {
	obj := makeObj("0", "a", "1", "b")
	arr, ok := numericObjectToArray(obj)
	if !ok {
		t.Fatal("expected conversion to succeed")
	}
	got := scalarStrings(arr)
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("basic: got %v want %v", got, want)
	}
}
