// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package parser_test — include directive argument-shape error paths.
// These assert spec-aligned rejection for malformed `include file(...)` and
// `include required(...)` forms whose error messages were uncovered before
// this file existed. Each test pins a single spec-driven error condition.

package parser_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
)

// TestIncludeFile_NonStringArgument asserts that `include file(...)` with a
// non-string argument (e.g. an integer literal or an empty arg list) raises a
// parse error pointing to "filename string" — per the include grammar where
// the file(...) form requires a quoted-string filename.
func TestIncludeFile_NonStringArgument(t *testing.T) {
	cases := []struct {
		label string
		src   string
	}{
		{"integer-literal", `include file(42)`},
		{"empty-arg", `include file()`},
		{"brace-arg", `include file({a:1})`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			_, err := parser.Parse(tc.src)
			if err == nil {
				t.Fatalf("Parse(%q): expected error, got nil", tc.src)
			}
			if !strings.Contains(err.Error(), "filename string") {
				t.Errorf("Parse(%q) error = %q; want substring %q", tc.src, err.Error(), "filename string")
			}
		})
	}
}

// TestIncludeFile_MissingClosingParen asserts that `include file("foo"` with
// no closing `)` raises a parse error pointing to "')' after filename".
func TestIncludeFile_MissingClosingParen(t *testing.T) {
	src := `include file("foo"`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatalf("Parse(%q): expected error, got nil", src)
	}
	if !strings.Contains(err.Error(), "')' after filename") {
		t.Errorf("Parse(%q) error = %q; want substring %q", src, err.Error(), "')' after filename")
	}
}

// TestIncludeRequired_MissingOuterClosingParen asserts that
// `include required(file("foo")` (inner `)` for file present, outer `)` for
// required missing) raises a parse error pointing to "')' to close required".
func TestIncludeRequired_MissingOuterClosingParen(t *testing.T) {
	src := `include required(file("foo")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatalf("Parse(%q): expected error, got nil", src)
	}
	if !strings.Contains(err.Error(), "')' to close required") {
		t.Errorf("Parse(%q) error = %q; want substring %q", src, err.Error(), "')' to close required")
	}
}
