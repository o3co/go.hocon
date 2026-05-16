// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import (
	"regexp"
	"sort"
	"strconv"
)

// eligibleKeyRe is the pre-filter for numerically-eligible object keys per the
// S15 integer-key parse rule.  Only keys that match this pattern are considered
// candidate integer keys; all others are silently ignored.
//
// The pattern enforces:
//   - "0" is the only zero form  (leading zeros like "00", "007" are rejected)
//   - No leading sign character  ("+1", "-0", "-1" are all rejected)
//   - Decimal digits only        ("0x1", "1e2", "1.0" are rejected)
//   - No whitespace              (" 1", "1 " are rejected)
//   - Non-empty                  ("" is rejected)
//
// This is intentionally stricter than Go's strconv.ParseInt, which accepts
// "+1", "-0", "00" etc.  The pre-filter must run BEFORE any native parse call.
var eligibleKeyRe = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

// NumericObjectToArray is the exported entry point for use by the hocon package.
// It delegates to numericObjectToArray.
func NumericObjectToArray(obj *ObjectVal) (*ArrayVal, bool) {
	return numericObjectToArray(obj)
}

// numericObjectToArray attempts to convert a numerically-indexed ObjectVal into
// an ArrayVal by the S15 semantics:
//
//  1. value must be a non-nil ObjectVal.
//  2. The object must not be empty (S15.4: empty object is NOT converted).
//  3. At least one key must be eligible per the integer-key parse rule.
//
// Eligible keys are sorted by their integer value ascending; gaps are eliminated
// (S15.6 compaction); non-eligible keys are silently ignored (S15.5).
//
// Returns (arr, true) when conversion succeeds.
// Returns (nil, false) in all other cases — caller handles type mismatch.
func numericObjectToArray(obj *ObjectVal) (*ArrayVal, bool) {
	if obj == nil {
		return nil, false
	}
	// S15.4: empty object is explicitly NOT converted.
	if len(obj.keys) == 0 {
		return nil, false
	}

	type kv struct {
		n int
		v Val
	}

	var eligible []kv
	for _, key := range obj.keys {
		// Pre-filter: must match ^(0|[1-9][0-9]*)$.
		if !eligibleKeyRe.MatchString(key) {
			continue
		}
		// Range check: must fit in i32 (max 2147483647).
		n, err := strconv.ParseInt(key, 10, 32)
		if err != nil {
			// Overflow or other parse failure — reject.
			continue
		}
		eligible = append(eligible, kv{n: int(n), v: obj.values[key]})
	}

	// No eligible keys → no conversion possible.
	if len(eligible) == 0 {
		return nil, false
	}

	// Sort by integer key ascending (S15.7).
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].n < eligible[j].n
	})

	// Project to value array, eliminating gaps (S15.6).
	arr := &ArrayVal{Elements: make([]Val, len(eligible))}
	for i, kv := range eligible {
		arr.Elements[i] = kv.v
	}
	return arr, true
}
