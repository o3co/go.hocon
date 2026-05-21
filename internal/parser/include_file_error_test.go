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
// parse error pointing to "expected include path string" — per the include
// grammar where the file(...) form requires a quoted-string filename.
//
// Post-xx.hocon#34 (Option C, mirrors ts.hocon parseInclude): parens are
// ordinary unquoted-continue chars, so `file(42)` lexes as a single unquoted
// `file(42)` token. The parser still rejects when no quoted string follows;
// the error message wording changed (was "filename string") to reflect the
// new skip-until-quoted-string strategy.
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
			// The new parseInclude can fail down two paths:
			//   - reaches EOF after consuming `file(...)` token → "expected include path string in include file(...) directive"
			//   - encounters a non-skip-able token (e.g. `{`) before finding the quoted string → "unexpected token ... before include path string"
			// Both contain "include path string" as a common substring.
			if !strings.Contains(err.Error(), "include path string") {
				t.Errorf("Parse(%q) error = %q; want substring %q", tc.src, err.Error(), "include path string")
			}
		})
	}
}

// Note: `TestIncludeFile_MissingClosingParen` and `TestIncludeRequired_MissingOuterClosingParen`
// were removed in xx.hocon#34 / go.hocon#100. The Option C migration adopts ts.hocon's
// permissive include-arg handling (skip-until-newline after consuming the path string),
// which matches Lightbend's lenient behavior. Inputs like `include file("foo"` and
// `include required(file("foo")` now parse successfully with the resolved path; missing
// trailing `)` is no longer a strict-spec violation in this impl.

// TestIncludeFile_DoesNotSilentlyMaskMalformedIncludes asserts that the
// post-#34 parser does NOT silently swallow real statement-boundary tokens
// (`,` / `=` / `{` / `}` / identifiers / numbers) while scanning forward
// for the quoted path string. Pre-fix, the skip loop scanned to the next
// quoted string without stopping at statement boundaries, which would have
// turned `include file() , b = "x"` into `include "x"` and dropped `b`.
// Multi-agent review (Claude + Codex) on go.hocon#100 flagged this as
// silent-data-loss; this test pins the stricter behavior.
func TestIncludeFile_DoesNotSilentlyMaskMalformedIncludes(t *testing.T) {
	cases := []struct {
		label string
		src   string
	}{
		{"file-empty-then-comma-field", `include file() , b = "x"`},
		{"file-int-then-bare-field", `include file(42) b = "x"`},
		{"required-empty-then-field", `include required() b = "x"`},
		{"required-unknown-resource", `include required(fileX("bar.conf"))`},
		{"required-unknown-resource-bare", `include required(abc("bar.conf"))`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			_, err := parser.Parse(tc.src)
			if err == nil {
				t.Fatalf("Parse(%q): expected error (must NOT silently consume `b = \"x\"` or accept unknown resource name), got nil", tc.src)
			}
		})
	}
}
