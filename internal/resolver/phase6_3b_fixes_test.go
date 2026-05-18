// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Phase 6 #3b review-fix tests — RED before implementation.
//
// Fix #1: valTypeName scalar precision — errors must name the correct scalar
//         subtype (null/boolean/number/string), not the generic "scalar".
// Fix #2: position info in concat type-mismatch errors — Line/Col must be
//         non-zero so errors are locatable in source.
package resolver_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/resolver"
)

// resolveExpectErr parses src and resolves it, expecting an error.
// Returns the error; fails the test if resolution unexpectedly succeeds.
func resolveExpectErr(t *testing.T, src string) error {
	t.Helper()
	ast, err := parser.Parse(src)
	if err != nil {
		return err
	}
	_, rerr := resolver.Resolve(ast, resolver.Options{})
	if rerr == nil {
		t.Fatalf("expected resolve error, got nil")
	}
	return rerr
}

// --- Fix #1: valTypeName scalar precision ---

// TestValTypeName_NullInErrorMessage asserts that when null is concatenated
// with an incompatible type, the error message names "null" not "scalar".
func TestValTypeName_NullInErrorMessage(t *testing.T) {
	// null + array is a type-mismatch: null is a scalar, not an array.
	err := resolveExpectErr(t, "a = null [1]")
	msg := err.Error()
	if strings.Contains(msg, "scalar") {
		t.Errorf("error message contains generic %q; want specific type name\n  got: %s", "scalar", msg)
	}
	if !strings.Contains(msg, "null") {
		t.Errorf("error message does not contain %q; want null type name\n  got: %s", "null", msg)
	}
}

// TestValTypeName_BooleanInErrorMessage asserts that a boolean concat mismatch
// reports "boolean" not "scalar" in the error message.
func TestValTypeName_BooleanInErrorMessage(t *testing.T) {
	err := resolveExpectErr(t, "a = true [1]")
	msg := err.Error()
	if strings.Contains(msg, "scalar") {
		t.Errorf("error message contains generic %q; want specific type name\n  got: %s", "scalar", msg)
	}
	if !strings.Contains(msg, "boolean") {
		t.Errorf("error message does not contain %q; want boolean type name\n  got: %s", "boolean", msg)
	}
}

// --- Fix #2: position info in concat type-mismatch errors ---

// TestConcatTypeMismatch_HasPosition asserts that a concat type-mismatch error
// carries non-zero Line and Col so callers can locate the error in source.
func TestConcatTypeMismatch_HasPosition(t *testing.T) {
	// Line 2, non-trivial column — a = null [1] where the concat value starts
	// at column 5 (1-based: "a = " is 4 chars, value starts at col 5).
	src := "\na = null [1]\n"
	err := resolveExpectErr(t, src)
	re, ok := err.(*resolver.ResolveError)
	if !ok {
		t.Fatalf("expected *resolver.ResolveError, got %T: %v", err, err)
	}
	if re.Line == 0 {
		t.Errorf("ResolveError.Line == 0; want non-zero source line\n  error: %v", re)
	}
	if re.Col == 0 {
		t.Errorf("ResolveError.Col == 0; want non-zero source column\n  error: %v", re)
	}
}
