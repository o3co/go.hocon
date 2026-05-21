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
// Multi-agent review (Claude + Codex) on go.hocon#100 + Copilot review on
// go.hocon#101 flagged this as silent-data-loss; this test pins the
// stricter behavior.
func TestIncludeFile_DoesNotSilentlyMaskMalformedIncludes(t *testing.T) {
	cases := []struct {
		label string
		src   string
	}{
		// Pre-path skip — non-skippable tokens before the quoted path:
		{"file-empty-then-comma-field", `include file() , b = "x"`},
		{"file-int-then-bare-field", `include file(42) b = "x"`},
		{"required-empty-then-field", `include required() b = "x"`},
		{"required-unknown-resource", `include required(fileX("bar.conf"))`},
		{"required-unknown-resource-bare", `include required(abc("bar.conf"))`},

		// Bare resource word without `(` — must reject per grammar:
		// `include file "x"` should error (the `file(...)` form requires parens
		// after the resource word, per the docs and existing fixtures). Same
		// for `required(file "x")` (inner-whitespace form). Copilot review
		// on PR#101.
		{"bare-file-no-paren", `include file "x"`},
		{"required-bare-file-no-paren", `include required(file "x")`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			_, err := parser.Parse(tc.src)
			if err == nil {
				t.Fatalf("Parse(%q): expected error (must NOT silently consume `b = \"x\"`, accept unknown resource name, or accept bare resource word without `(`), got nil", tc.src)
			}
		})
	}
}

// TestIncludeFile_DoesNotConsumeNextFieldOnSameLine asserts that the post-path
// noise loop in parseInclude only consumes trailing `)` close-paren tokens —
// NOT arbitrary tokens that look like the start of the next field. HOCON
// allows field separator omission on the same line (see `skipSeparator` in
// parser.go), so `include file("a") b = "x"` is a valid two-field document.
// Pre-fix, the post-path loop scanned until newline/`}`/EOF/comma, silently
// consuming `b = "x"` and dropping the field. Copilot review on PR#101
// flagged the symmetric pre/post-path bug; this test pins the post-path fix.
func TestIncludeFile_DoesNotConsumeNextFieldOnSameLine(t *testing.T) {
	cases := []struct {
		label string
		src   string
	}{
		{"file-then-trailing-field-same-line", `include file("a") b = "x"`},
		{"required-file-then-trailing-field-same-line", `include required(file("a")) b = "x"`},
		{"required-quoted-then-trailing-field-same-line", `include required("a") b = "x"`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			root, err := parser.Parse(tc.src)
			// Parse should succeed at the syntax layer (include resolution
			// happens later in the resolver). Even if include resolution would
			// fail (no such file), parser.Parse is responsible only for AST
			// construction.
			if err != nil {
				t.Fatalf("Parse(%q): unexpected parse error: %v", tc.src, err)
			}
			// We expect exactly 2 root fields: the include + the `b` field.
			// (The include node is stored as a FieldNode with an IncludeNode
			// value; the `b` field is a regular FieldNode with key ["b"].)
			if len(root.Fields) != 2 {
				keys := make([]string, 0, len(root.Fields))
				for _, f := range root.Fields {
					if len(f.Key) > 0 {
						keys = append(keys, f.Key[0])
					} else {
						keys = append(keys, "<include>")
					}
				}
				t.Errorf("Parse(%q): expected 2 root fields (include + b), got %d (keys: %v)", tc.src, len(root.Fields), keys)
			}
			// The `b` field must be present in the root.
			foundB := false
			for _, f := range root.Fields {
				if len(f.Key) == 1 && f.Key[0] == "b" {
					foundB = true
					break
				}
			}
			if !foundB {
				t.Errorf("Parse(%q): expected `b` field in root, got: %#v", tc.src, root.Fields)
			}
		})
	}
}
