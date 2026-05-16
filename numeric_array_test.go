// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package hocon_test — S15 numeric-object-to-array conformance tests.
// Loads each xx.hocon fixture from testdata/hocon/numeric-obj-array/ and
// asserts the o3co accessor result per the spec and divergence docs.
package hocon_test

import (
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// mustParseFile is a test helper that parses a fixture file and fatals on error.
func mustParseFile(t *testing.T, path string) *hocon.Config {
	t.Helper()
	cfg, err := hocon.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", path, err)
	}
	return cfg
}

// assertSlice compares two []string for equality (length + element-wise).
func assertSlice(t *testing.T, path string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: length=%d want %d; got %v want %v", path, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: [%d]=%q want %q", path, i, got[i], want[i])
		}
	}
}

// ── na01: basic accessor conversion ──────────────────────────────────────────

// TestS15_Fixture_na01_BasicAccessor verifies S15.1: object {"0":"a","1":"b"}
// is converted to ["a","b"] when GetStringSlice is called.
func TestS15_Fixture_na01_BasicAccessor(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na01-basic.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"a", "b"})
}

// ── na02: laziness — object is accessible as object ───────────────────────────

// TestS15_Fixture_na02_LazyGetObject verifies S15.2: a numeric-keyed object
// remains accessible as an object; conversion is NOT eager.
func TestS15_Fixture_na02_LazyGetObject(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na02-lazy-getobject.conf")
	// Object access must NOT trigger conversion.
	sub := cfg.GetConfig("items")
	if got := sub.GetString("0"); got != "a" {
		t.Errorf("items.0=%q want %q (laziness violated)", got, "a")
	}
	if got := sub.GetString("1"); got != "b" {
		t.Errorf("items.1=%q want %q (laziness violated)", got, "b")
	}
	// Array access DOES trigger conversion.
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"a", "b"})
}

// ── na03a: concat left-list ───────────────────────────────────────────────────

// TestS15_Fixture_na03a_ConcatLeftList verifies S15.3: [a] ${obj} where obj has
// numeric keys → arr = ["a", "x", "y"] after concat-time conversion.
func TestS15_Fixture_na03a_ConcatLeftList(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na03a-concat-left-list.conf")
	got := cfg.GetStringSlice("arr")
	assertSlice(t, "arr", got, []string{"a", "x", "y"})
}

// ── na03b: concat right-list ─────────────────────────────────────────────────

// TestS15_Fixture_na03b_ConcatRightList verifies S15.3 symmetric case: ${obj} [a]
// → arr = ["x", "y", "a"] after concat-time conversion.
func TestS15_Fixture_na03b_ConcatRightList(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na03b-concat-right-list.conf")
	got := cfg.GetStringSlice("arr")
	assertSlice(t, "arr", got, []string{"x", "y", "a"})
}

// ── na03c: two-object concat NOT converted ────────────────────────────────────

// TestS15_Fixture_na03c_TwoObjConcatNoConvert verifies that ${obj1} ${obj2} with
// both sides being objects produces an S10.3 object merge — no array conversion.
// The merged object IS accessible as an object, and GetStringSlice triggers
// accessor-side conversion.
func TestS15_Fixture_na03c_TwoObjConcatNoConvert(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na03c-concat-two-objs.conf")
	// The resolved arr is the merged object {"0":"x","1":"y","2":"z","3":"w"}.
	// Accessor-side conversion fires for GetStringSlice.
	got := cfg.GetStringSlice("arr")
	assertSlice(t, "arr", got, []string{"x", "y", "z", "w"})
}

// ── na03d: multi-piece concat left-to-right pairwise (NORMATIVE) ─────────────

// TestS15_Fixture_na03d_MultiPieceConcatNormative verifies the NORMATIVE left-to-right
// pairwise fold: ${obj1} ${obj2} [a].
// Fold: join(obj1,obj2) → merged object {0:x,1:y,2:z,3:w};
//
//	join(merged, [a]) → numericObjectToArray → ["x","y","z","w","a"].
func TestS15_Fixture_na03d_MultiPieceConcatNormative(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na03d-concat-multi-piece.conf")
	got := cfg.GetStringSlice("arr")
	assertSlice(t, "arr", got, []string{"x", "y", "z", "w", "a"})
}

// ── na04: empty object NOT converted ─────────────────────────────────────────

// TestS15_Fixture_na04_EmptyNotConverted verifies S15.4: empty {} is NOT
// converted; GetStringSliceOption returns None (type mismatch).
func TestS15_Fixture_na04_EmptyNotConverted(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na04-empty.conf")
	if cfg.GetStringSliceOption("items").IsSome() {
		t.Errorf("empty object must NOT convert to array (S15.4)")
	}
}

// ── na05: non-integer keys ignored ───────────────────────────────────────────

// TestS15_Fixture_na05_NonIntKeysIgnored verifies S15.5: {"0":"a","foo":"b","1":"c"}
// → ["a","c"] (non-int key "foo" is ignored).
func TestS15_Fixture_na05_NonIntKeysIgnored(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na05-non-int-keys.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"a", "c"})
}

// ── na06: gap compaction ──────────────────────────────────────────────────────

// TestS15_Fixture_na06_GapCompaction verifies S15.6: {"0":"a","2":"c"} → ["a","c"]
// (gap at index 1 is compacted away).
func TestS15_Fixture_na06_GapCompaction(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na06-gaps.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"a", "c"})
}

// ── na07: sort by integer key ─────────────────────────────────────────────────

// TestS15_Fixture_na07_SortByKey verifies S15.7: {"1":"b","0":"a"} → ["a","b"]
// (sorted by integer key value, regardless of insertion order).
func TestS15_Fixture_na07_SortByKey(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na07-sort.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"a", "b"})
}

// ── na08: leading zero rejected (E2) ─────────────────────────────────────────

// TestS15_Fixture_na08_LeadingZeroRejected verifies E2 (extra-spec): "00" is
// non-canonical and ineligible. Only "0" is eligible → result is ["b"].
// Lightbend divergence: Lightbend accepts "00" as 0 (HashMap collision,
// non-deterministic winner). o3co rejects "00" for canonical-text guarantee.
// See testdata/expected/numeric-obj-array/na08-leading-zero.divergence.md.
func TestS15_Fixture_na08_LeadingZeroRejected(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na08-leading-zero.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"b"})
}

// ── na09: negative keys ignored ───────────────────────────────────────────────

// TestS15_Fixture_na09_NegativeIgnored verifies that "-1" is ineligible (negative).
// Only "0" is eligible → result is ["b"].
func TestS15_Fixture_na09_NegativeIgnored(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na09-negative.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"b"})
}

// ── na10a: plus sign rejected (E3) ───────────────────────────────────────────

// TestS15_Fixture_na10a_PlusSignRejected verifies E3 (extra-spec): "+1" has a
// leading sign character and is ineligible. Only "0" is eligible → result is ["b"].
// Lightbend divergence: Lightbend accepts "+1" as 1 → result would be ["b","a"].
// See testdata/expected/numeric-obj-array/na10a-plus-sign.divergence.md.
func TestS15_Fixture_na10a_PlusSignRejected(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na10a-plus-sign.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"b"})
}

// ── na10b: minus-zero rejected (E4) ──────────────────────────────────────────

// TestS15_Fixture_na10b_MinusZeroRejected verifies E4 (extra-spec): "-0" has a
// leading sign character and is ineligible. Only "0" is eligible → result is ["b"].
// Lightbend divergence: Lightbend HashMap collision, non-deterministic winner.
// See testdata/expected/numeric-obj-array/na10b-minus-zero.divergence.md.
func TestS15_Fixture_na10b_MinusZeroRejected(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na10b-minus-zero.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"b"})
}

// ── na11: overflow rejected ───────────────────────────────────────────────────

// TestS15_Fixture_na11_OverflowRejected verifies that "99999999999" exceeds i32
// range and is rejected. Only "0" is eligible → result is ["b"].
func TestS15_Fixture_na11_OverflowRejected(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na11-overflow.conf")
	got := cfg.GetStringSlice("items")
	assertSlice(t, "items", got, []string{"b"})
}

// ── na12: all-non-int keys → no conversion ───────────────────────────────────

// TestS15_Fixture_na12_AllNonInt verifies that when no keys are eligible,
// numericObjectToArray returns None and GetStringSliceOption returns None
// (type mismatch — no conversion possible).
func TestS15_Fixture_na12_AllNonInt(t *testing.T) {
	cfg := mustParseFile(t, "testdata/hocon/numeric-obj-array/na12-no-eligible.conf")
	if cfg.GetStringSliceOption("items").IsSome() {
		t.Errorf("all-non-int keys: expected None (no conversion), got Some")
	}
}
