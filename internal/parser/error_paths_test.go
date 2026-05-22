// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package parser_test — targeted tests for parser error paths identified in #37.
//
// Each test exercises a specific newError() call site from the #37 scope and
// pins the error-message contract. Most call sites were uncovered before this
// file existed; a few overlap with pre-existing tests (e.g. direct-form
// `include url(...)` / `include classpath(...)`) and are kept here as contract
// pins so the error wording can't silently drift. The test names and comments
// identify the source location and issue-scope item for traceability.
package parser_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
)

// ---------------------------------------------------------------------------
// ParseBytes — trivial wrapper, previously 0% covered.
// ---------------------------------------------------------------------------

// TestParseBytes_OK verifies that ParseBytes delegates correctly to Parse for
// valid input (ParseBytes at parser.go:74 was 0% covered).
func TestParseBytes_OK(t *testing.T) {
	obj, err := parser.ParseBytes([]byte(`a = 1`))
	if err != nil {
		t.Fatalf("ParseBytes(%q) unexpected error: %v", `a = 1`, err)
	}
	if len(obj.Fields) != 1 || len(obj.Fields[0].Key) != 1 || obj.Fields[0].Key[0] != "a" {
		t.Errorf("unexpected parse result: %+v", obj)
	}
}

// TestParseBytes_Error verifies that ParseBytes propagates parse errors from Parse.
func TestParseBytes_Error(t *testing.T) {
	_, err := parser.ParseBytes([]byte(`{ unclosed`))
	if err == nil {
		t.Fatal("ParseBytes: expected error for unclosed brace, got nil")
	}
}

// ---------------------------------------------------------------------------
// errors.go: Error.Error() with Line == 0 (the "no position" branch).
// ---------------------------------------------------------------------------

// TestError_NoPosition verifies the "parse error: <msg>" branch of Error.Error()
// when Line == 0 (errors.go:25). parser.Parse("") produces a positional error
// at line 1, so we construct the struct directly.
func TestError_NoPosition(t *testing.T) {
	e := &parser.Error{Message: "synthetic", Line: 0, Col: 0}
	got := e.Error()
	if !strings.Contains(got, "parse error:") {
		t.Errorf("Error.Error() with Line=0: expected 'parse error:' prefix, got %q", got)
	}
	if strings.Contains(got, "line") {
		t.Errorf("Error.Error() with Line=0: unexpected 'line' in output, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// include url(...) / include classpath(...) — top-level case (already partially
// covered), plus the paths that go through skipToIncludePath when the
// url/classpath keyword appears inside a file(...) or required(...) qualifier
// with whitespace before the inner keyword (lines 282, 285 in parser.go).
// ---------------------------------------------------------------------------

// TestIncludeURL_DirectForm pins the "include url(...) is not supported" error
// from the top-level case switch in parseInclude (line 452).
func TestIncludeURL_DirectForm(t *testing.T) {
	_, err := parser.Parse(`include url("http://example.com/foo.conf")`)
	if err == nil {
		t.Fatal("expected error for include url(...), got nil")
	}
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'url ... not supported' in error, got: %v", err)
	}
}

// TestIncludeURL_WithParenPrefix pins the url(...) rejection inside
// skipToIncludePath (parser.go:282) — triggered when the token is `(url(` or
// similar (whitespace before `url`). Input: `include file( url("x"))`.
func TestIncludeURL_WithParenPrefix(t *testing.T) {
	_, err := parser.Parse(`include file( url("http://foo"))`)
	if err == nil {
		t.Fatal("expected error for include file( url(...)), got nil")
	}
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'url ... not supported' in error, got: %v", err)
	}
}

// TestIncludeClasspath_DirectForm pins the "include classpath(...) is not
// supported" error from the top-level switch (line 455).
func TestIncludeClasspath_DirectForm(t *testing.T) {
	_, err := parser.Parse(`include classpath("reference.conf")`)
	if err == nil {
		t.Fatal("expected error for include classpath(...), got nil")
	}
	if !strings.Contains(err.Error(), "classpath") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'classpath ... not supported' in error, got: %v", err)
	}
}

// TestIncludeClasspath_WithParenPrefix pins the classpath(...) rejection inside
// skipToIncludePath (parser.go:285) — triggered by `include file( classpath(...))`.
func TestIncludeClasspath_WithParenPrefix(t *testing.T) {
	_, err := parser.Parse(`include file( classpath("reference.conf"))`)
	if err == nil {
		t.Fatal("expected error for include file( classpath(...)), got nil")
	}
	if !strings.Contains(err.Error(), "classpath") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'classpath ... not supported' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// include required(url(...)) / include required(classpath(...)) not supported
// (parser.go:342, 345 — same-token form via innerPrefix).
// ---------------------------------------------------------------------------

// TestIncludeRequiredURL_SameToken pins the "include required(url(...)) is not
// supported" error from the same-token innerPrefix check (line 342) — triggered
// when the lexer merges `required(url(` as a single unquoted token.
func TestIncludeRequiredURL_SameToken(t *testing.T) {
	_, err := parser.Parse(`include required(url("http://example.com"))`)
	if err == nil {
		t.Fatal("expected error for include required(url(...)), got nil")
	}
	if !strings.Contains(err.Error(), "required(url") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'required(url...) not supported' in error, got: %v", err)
	}
}

// TestIncludeRequiredClasspath_SameToken pins the "include required(classpath(...))
// is not supported" error (line 345) — triggered when the lexer merges
// `required(classpath(` as a single unquoted token.
func TestIncludeRequiredClasspath_SameToken(t *testing.T) {
	_, err := parser.Parse(`include required(classpath("reference.conf"))`)
	if err == nil {
		t.Fatal("expected error for include required(classpath(...)), got nil")
	}
	if !strings.Contains(err.Error(), "required(classpath") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'required(classpath...) not supported' in error, got: %v", err)
	}
}

// TestIncludeRequiredURL_WhitespaceSeparated pins the url(...) rejection inside
// skipToIncludePath (parser.go:282) when called from the required(...) branch with
// whitespace between `required` and its argument, followed by `url(`.
// Input: `include required( url("http://foo"))`.
func TestIncludeRequiredURL_WhitespaceSeparated(t *testing.T) {
	_, err := parser.Parse(`include required( url("http://foo"))`)
	if err == nil {
		t.Fatal("expected error for include required( url(...)), got nil")
	}
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'url ... not supported' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Missing `(` after `required` in include directive (parser.go:333).
// ---------------------------------------------------------------------------

// TestIncludeRequired_MissingParen_Equals pins the "expected '(' after 'required'
// in include directive" error (line 333) when `required` is followed by `=`
// (an unambiguous non-paren token) instead of `(`.
func TestIncludeRequired_MissingParen_Equals(t *testing.T) {
	_, err := parser.Parse(`include required = x`)
	if err == nil {
		t.Fatal("expected error for include required = x, got nil")
	}
	if !strings.Contains(err.Error(), "expected '(' after 'required'") {
		t.Errorf("expected \"expected '(' after 'required'\" in error, got: %v", err)
	}
}

// TestIncludeRequired_MissingParen_QuotedString pins the same error (line 333)
// when `required` is followed directly by a quoted string (omitting the `(`).
// This covers the `!strings.HasPrefix(p.current.Value, "(")` branch for quoted tokens.
func TestIncludeRequired_MissingParen_QuotedString(t *testing.T) {
	_, err := parser.Parse(`include required "foo.conf"`)
	if err == nil {
		t.Fatal("expected error for include required \"foo.conf\" (no paren), got nil")
	}
	if !strings.Contains(err.Error(), "expected '(' after 'required'") {
		t.Errorf("expected \"expected '(' after 'required'\" in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Missing `(` after `file` in include directive (parser.go:411).
// ---------------------------------------------------------------------------

// TestIncludeFile_MissingParen_Equals pins the "expected '(' after 'file' in
// include directive" error (line 411) when `file` is followed by `=`.
func TestIncludeFile_MissingParen_Equals(t *testing.T) {
	_, err := parser.Parse(`include file = x`)
	if err == nil {
		t.Fatal("expected error for include file = x, got nil")
	}
	if !strings.Contains(err.Error(), "expected '(' after 'file' in include directive") {
		t.Errorf("expected \"expected '(' after 'file' in include directive\" in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Missing `(` after `file` inside include required(file ...) (parser.go:354).
// ---------------------------------------------------------------------------

// TestIncludeRequiredFile_MissingInnerParen pins the "expected '(' after 'file'
// in include required(file(...))" error (line 354) when `required(file` is
// followed by a non-paren token. Input: `include required(file "x")`.
func TestIncludeRequiredFile_MissingInnerParen(t *testing.T) {
	_, err := parser.Parse(`include required(file "x")`)
	if err == nil {
		t.Fatal("expected error for include required(file \"x\"), got nil")
	}
	if !strings.Contains(err.Error(), "expected '(' after 'file'") {
		t.Errorf("expected \"expected '(' after 'file'\" in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// skipToIncludePath: EOF/newline before finding a quoted path string (line 272).
// ---------------------------------------------------------------------------

// TestIncludeRequired_EOF_BeforePath pins the "expected include path string in
// include required(...) directive" error (skipToIncludePath line 272) when the
// `required(` token is consumed but the input ends at EOF before a quoted string.
func TestIncludeRequired_EOF_BeforePath(t *testing.T) {
	_, err := parser.Parse(`include required(`)
	if err == nil {
		t.Fatal("expected error for include required( at EOF, got nil")
	}
	if !strings.Contains(err.Error(), "expected include path string") {
		t.Errorf("expected 'expected include path string' in error, got: %v", err)
	}
}

// TestIncludeRequired_Newline_BeforePath pins the same error when a newline
// interrupts the required(...) argument before a path string is found.
func TestIncludeRequired_Newline_BeforePath(t *testing.T) {
	_, err := parser.Parse("include required(\n")
	if err == nil {
		t.Fatal("expected error for include required( + newline, got nil")
	}
	if !strings.Contains(err.Error(), "expected include path string") {
		t.Errorf("expected 'expected include path string' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invalid int literals in value position (parseSingleValue, parser.go:822).
// ---------------------------------------------------------------------------

// TestParseSingleValue_InvalidInt pins the "invalid int %q" error (line 822)
// when a TokenInt value overflows int64. The lexer accepts very large digit
// sequences as TokenInt; strconv.ParseInt then fails.
func TestParseSingleValue_InvalidInt(t *testing.T) {
	_, err := parser.Parse(`a = 9999999999999999999999`)
	if err == nil {
		t.Fatal("expected error for overflowing int literal, got nil")
	}
	if !strings.Contains(err.Error(), "invalid int") {
		t.Errorf("expected 'invalid int' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "9999999999999999999999") {
		t.Errorf("expected the literal value in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invalid float literals in value position (parseSingleValue, parser.go:833).
// ---------------------------------------------------------------------------

// TestParseSingleValue_InvalidFloat pins the "invalid float %q" error (line 833)
// when a TokenFloat value is out of range for float64. The lexer produces a
// TokenFloat for digit-dot-digit sequences; strconv.ParseFloat fails for
// values beyond ±MaxFloat64.
func TestParseSingleValue_InvalidFloat(t *testing.T) {
	_, err := parser.Parse(`a = 1.7976931348623157e+309`)
	if err == nil {
		t.Fatal("expected error for out-of-range float literal, got nil")
	}
	if !strings.Contains(err.Error(), "invalid float") {
		t.Errorf("expected 'invalid float' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unexpected token in value position (parseSingleValue default case, parser.go:844).
// ---------------------------------------------------------------------------

// TestParseSingleValue_UnexpectedToken_RBrace pins the "unexpected token" error
// (line 844) when a `}` appears in value position. This is the default branch
// of the parseSingleValue switch; TokenRBrace is not a valid value-start token.
func TestParseSingleValue_UnexpectedToken_RBrace(t *testing.T) {
	_, err := parser.Parse(`a = }`)
	if err == nil {
		t.Fatal("expected error for `a = }`, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected token") {
		t.Errorf("expected 'unexpected token' in error, got: %v", err)
	}
}

// TestParseSingleValue_UnexpectedToken_RBracket pins the same error path
// (line 844) when `]` appears in a non-array value position.
func TestParseSingleValue_UnexpectedToken_RBracket(t *testing.T) {
	_, err := parser.Parse(`a = ]`)
	if err == nil {
		t.Fatal("expected error for `a = ]`, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected token") {
		t.Errorf("expected 'unexpected token' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Empty key (parseKey, parser.go:750).
// ---------------------------------------------------------------------------

// TestParseKey_EmptyKey_DotAtNewline pins the "empty key" error (line 750)
// when the key position contains a lone "." followed by a newline. The "."
// token's unquoted branch produces no path segments (split on "." yields only
// empties), the trailing-dot continuation fires (HasSuffix "."), but the next
// token is a TokenNewline which stops the continuation — leaving parts empty.
func TestParseKey_EmptyKey_DotAtNewline(t *testing.T) {
	_, err := parser.Parse(".\n= x")
	if err == nil {
		t.Fatal("expected error for dot-then-newline key, got nil")
	}
	if !strings.Contains(err.Error(), "empty key") {
		t.Errorf("expected 'empty key' in error, got: %v", err)
	}
}

// TestParseKey_EmptyKey_DotAtEOF pins the same error when "." is the only
// token before EOF.
func TestParseKey_EmptyKey_DotAtEOF(t *testing.T) {
	_, err := parser.Parse(".")
	if err == nil {
		t.Fatal("expected error for bare dot at EOF, got nil")
	}
	if !strings.Contains(err.Error(), "empty key") {
		t.Errorf("expected 'empty key' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// validateKeySegment: unquoted key starting with '-' (parser.go:587).
// ---------------------------------------------------------------------------

// TestValidateKeySegment_SingleDash pins the "unquoted key segment cannot begin
// with '-' unless followed by a digit" error (validateKeySegment line 587) for
// a bare `-` key (no digit follows).
func TestValidateKeySegment_SingleDash(t *testing.T) {
	_, err := parser.Parse(`- = x`)
	if err == nil {
		t.Fatal("expected error for single dash key, got nil")
	}
	if !strings.Contains(err.Error(), "unquoted key segment cannot begin with '-'") {
		t.Errorf("expected dash-start error, got: %v", err)
	}
}

// TestValidateKeySegment_DashLetter pins the same error when the character
// after '-' is a letter (not a digit). Covers the `len(s) >= 2` branch that
// formats the quoted rune in the error message.
func TestValidateKeySegment_DashLetter(t *testing.T) {
	_, err := parser.Parse(`-a = x`)
	if err == nil {
		t.Fatal("expected error for -a key, got nil")
	}
	if !strings.Contains(err.Error(), "unquoted key segment cannot begin with '-'") {
		t.Errorf("expected dash-start error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `'a'`) {
		t.Errorf("expected the offending char in error, got: %v", err)
	}
}
