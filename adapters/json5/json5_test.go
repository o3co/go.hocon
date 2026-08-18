// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package json5_test

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/json5"
)

func parse(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := json5.Parse([]byte(src), "test.json5")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return cfg
}

func parseErr(t *testing.T, src, wantSubstr string) {
	t.Helper()
	_, err := json5.Parse([]byte(src), "test.json5")
	if err == nil {
		t.Fatalf("Parse(%q): expected error containing %q, got nil", src, wantSubstr)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("Parse(%q): error %q does not contain %q", src, err, wantSubstr)
	}
}

// ---------------------------------------------------------------------------
// The json5.org front-page example, minus Infinity/NaN (spec F0.6 rejects
// those — pinned separately below).
// ---------------------------------------------------------------------------

func TestJSON5FrontPageExample(t *testing.T) {
	cfg := parse(t, `{
	  // comments
	  unquoted: 'and you can quote me on that',
	  singleQuotes: 'I can use "double quotes" here',
	  lineBreaks: "Look, Mom! \
No \\n's!",
	  hexadecimal: 0xdecaf,
	  leadingDecimalPoint: .8675309, andTrailing: 8675309.,
	  positiveSign: +1,
	  trailingComma: 'in objects', andIn: ['arrays',],
	  "backwardsCompatible": "with JSON",
	}`)

	for path, want := range map[string]string{
		"unquoted":            "and you can quote me on that",
		"singleQuotes":        `I can use "double quotes" here`,
		"lineBreaks":          `Look, Mom! No \n's!`,
		"trailingComma":       "in objects",
		"backwardsCompatible": "with JSON",
	} {
		if got := cfg.GetString(path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	if got := cfg.GetInt64("hexadecimal"); got != 0xdecaf {
		t.Errorf("hexadecimal = %d, want %d", got, int64(0xdecaf))
	}
	if got := cfg.GetFloat64("leadingDecimalPoint"); got != 0.8675309 {
		t.Errorf("leadingDecimalPoint = %v, want 0.8675309", got)
	}
	if got := cfg.GetFloat64("andTrailing"); got != 8675309.0 {
		t.Errorf("andTrailing = %v, want 8675309.0", got)
	}
	if got := cfg.GetInt64("positiveSign"); got != 1 {
		t.Errorf("positiveSign = %d, want 1", got)
	}
	if got := cfg.GetStringSlice("andIn"); !reflect.DeepEqual(got, []string{"arrays"}) {
		t.Errorf("andIn = %#v, want [arrays]", got)
	}
}

// ---------------------------------------------------------------------------
// Identifier keys (ES5 IdentifierName)
// ---------------------------------------------------------------------------

func TestIdentifierKeys(t *testing.T) {
	cfg := parse(t, `{a: 1, $b: 2, _c: 3, é: 4, a1: 5}`)
	for path, want := range map[string]int64{"a": 1, "$b": 2, "_c": 3, "é": 4, "a1": 5} {
		if got := cfg.GetInt64(path); got != want {
			t.Errorf("%s = %d, want %d", path, got, want)
		}
	}
}

func TestIdentifierKeyUnicodeEscape(t *testing.T) {
	// \u0061 = 'a'; ES5 allows \u escapes inside IdentifierName.
	cfg := parse(t, `{\u0061\u0062: 7}`)
	if got := cfg.GetInt64("ab"); got != 7 {
		t.Errorf("ab = %d, want 7", got)
	}
}

func TestIdentifierKeyErrors(t *testing.T) {
	parseErr(t, `{1a: 1}`, "expected an object key")
	// \u0031 = '1' — a legal escape, but not a legal identifier START.
	parseErr(t, `{\u0031x: 1}`, "not a valid identifier character")
	parseErr(t, `{\x61: 1}`, `only \uXXXX escapes`)
}

// ---------------------------------------------------------------------------
// Strings
// ---------------------------------------------------------------------------

func TestStringEscapes(t *testing.T) {
	cfg := parse(t, `{
	  hex: "\x41\x42",
	  vtab: "a\vb",
	  nul: "a\0b",
	  self: "\q\'\"",
	  astral: "\uD83D\uDE00",
	}`)
	for path, want := range map[string]string{
		"hex":    "AB",
		"vtab":   "a\vb",
		"nul":    "a\x00b",
		"self":   `q'"`,
		"astral": "😀",
	} {
		if got := cfg.GetString(path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestStringLineContinuations(t *testing.T) {
	// LF, CRLF, and LS continuations all contribute nothing.
	cfg := parse(t, "{a: 'x\\\ny', b: 'x\\\r\ny', c: 'x\\\u2028y'}")
	for _, path := range []string{"a", "b", "c"} {
		if got := cfg.GetString(path); got != "xy" {
			t.Errorf("%s = %q, want %q", path, got, "xy")
		}
	}
}

func TestStringUnescapedSeparatorsAllowed(t *testing.T) {
	// LS/PS are legal unescaped inside JSON5 strings (the ES5 quirk).
	cfg := parse(t, "{a: 'x\u2028y'}")
	if got := cfg.GetString("a"); got != "x\u2028y" {
		t.Errorf("a = %q, want %q", got, "x\u2028y")
	}
}

func TestStringErrors(t *testing.T) {
	parseErr(t, "{a: 'x\ny'}", "unescaped line terminator")
	parseErr(t, `{a: 'oops}`, "unterminated string")
	parseErr(t, `{a: '\01'}`, "octal escape")
	parseErr(t, `{a: '\7'}`, "digits cannot be escaped")
	// F3.5: a lone surrogate is an error, high or low, paired-wrong or alone.
	parseErr(t, `{a: "\uD800"}`, "spec F3.5")
	parseErr(t, `{a: "\uD800\u0041"}`, "spec F3.5")
	parseErr(t, `{a: "\uDE00"}`, "spec F3.5")
}

// ---------------------------------------------------------------------------
// Numbers
// ---------------------------------------------------------------------------

func TestNumbers(t *testing.T) {
	cfg := parse(t, `{
	  hex: 0xFF, hexneg: -0x10, hexplus: +0xA,
	  min: -0x8000000000000000, max: 0x7FFFFFFFFFFFFFFF,
	  lead: .5, trail: 5., plus: +5, exp: 1e3, negzero: -0,
	}`)
	if got := cfg.GetInt64("hex"); got != 255 {
		t.Errorf("hex = %d, want 255", got)
	}
	if got := cfg.GetInt64("hexneg"); got != -16 {
		t.Errorf("hexneg = %d, want -16", got)
	}
	if got := cfg.GetInt64("hexplus"); got != 10 {
		t.Errorf("hexplus = %d, want 10", got)
	}
	if got := cfg.GetInt64("min"); got != math.MinInt64 {
		t.Errorf("min = %d, want MinInt64", got)
	}
	if got := cfg.GetInt64("max"); got != math.MaxInt64 {
		t.Errorf("max = %d, want MaxInt64", got)
	}
	if got := cfg.GetFloat64("lead"); got != 0.5 {
		t.Errorf("lead = %v, want 0.5", got)
	}
	if got := cfg.GetFloat64("trail"); got != 5.0 {
		t.Errorf("trail = %v, want 5.0", got)
	}
	if got := cfg.GetInt64("plus"); got != 5 {
		t.Errorf("plus = %d, want 5", got)
	}
	if got := cfg.GetFloat64("exp"); got != 1000.0 {
		t.Errorf("exp = %v, want 1000.0", got)
	}
	if got := cfg.GetInt64("negzero"); got != 0 {
		t.Errorf("negzero = %d, want 0", got)
	}
}

func TestNumberErrors(t *testing.T) {
	// F0.5: integers that do not fit in int64 are errors, not silent floats.
	parseErr(t, `{a: 0x10000000000000000}`, "spec F0.5")
	parseErr(t, `{a: -0x8000000000000001}`, "spec F0.5")
	parseErr(t, `{a: 9223372036854775808}`, "spec F0.5")
	parseErr(t, `{a: 0x}`, "hex literal needs at least one digit")
	// F0.6: Infinity and NaN in every spelling.
	for _, lit := range []string{"Infinity", "-Infinity", "+Infinity", "NaN", "-NaN", "+NaN"} {
		parseErr(t, `{a: `+lit+`}`, "spec F0.6")
	}
}

// ---------------------------------------------------------------------------
// Comments, whitespace, structure
// ---------------------------------------------------------------------------

func TestCommentsAndWhitespace(t *testing.T) {
	cfg := parse(t, "{\n  // line comment\u2028 a: 1,\n  /* block\n comment */ b: 2,\u00a0c:\u20033\n}")
	for path, want := range map[string]int64{"a": 1, "b": 2, "c": 3} {
		if got := cfg.GetInt64(path); got != want {
			t.Errorf("%s = %d, want %d", path, got, want)
		}
	}
}

func TestCommentAndStructureErrors(t *testing.T) {
	parseErr(t, `{a: 1} /* open`, "unterminated /* comment")
	parseErr(t, `{a: 1} }`, "unexpected content after top-level value")
	parseErr(t, `{a: 1`, "unterminated object")
	parseErr(t, `[1, 2`, "unterminated array")
	parseErr(t, `[,1]`, "unexpected character")
	parseErr(t, `[1,,2]`, "unexpected character")
	parseErr(t, `{a 1}`, "expected ':'")
	// F0.3: the root must be an object.
	parseErr(t, `[1, 2]`, "spec F0.3")
	parseErr(t, `"just a string"`, "spec F0.3")
}

// Trailing whitespace and comments after the value are fine (only content is
// an error).
func TestTrailingTriviaAccepted(t *testing.T) {
	cfg := parse(t, "{a: 1} // done\n/* and a block */\n\n")
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Duplicate keys (spec F0.7)
// ---------------------------------------------------------------------------

func TestDuplicateKeysFollowHoconSemantics(t *testing.T) {
	// Two objects merge…
	cfg := parse(t, `{a: {x: 1, shared: {p: 1}}, a: {y: 2, shared: {q: 2}}}`)
	if got := cfg.GetInt64("a.x"); got != 1 {
		t.Errorf("a.x = %d, want 1", got)
	}
	if got := cfg.GetInt64("a.y"); got != 2 {
		t.Errorf("a.y = %d, want 2", got)
	}
	if got := cfg.GetInt64("a.shared.p"); got != 1 {
		t.Errorf("a.shared.p = %d, want 1", got)
	}
	if got := cfg.GetInt64("a.shared.q"); got != 2 {
		t.Errorf("a.shared.q = %d, want 2", got)
	}

	// …anything else is last-wins.
	cfg = parse(t, `{a: 1, a: 2}`)
	if got := cfg.GetInt64("a"); got != 2 {
		t.Errorf("a = %d, want 2", got)
	}
	cfg = parse(t, `{a: {x: 1}, a: 2}`)
	if got := cfg.GetInt64("a"); got != 2 {
		t.Errorf("a = %d, want 2 (scalar over object)", got)
	}
}

// ---------------------------------------------------------------------------
// BOM and encoding (spec F0.9, S1.1 posture)
// ---------------------------------------------------------------------------

func TestBOM(t *testing.T) {
	cfg := parse(t, "\ufeff{a: \ufeff1}")
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
}

func TestInvalidUTF8Rejected(t *testing.T) {
	_, err := json5.Parse([]byte("{a: \"\xff\"}"), "test.json5")
	if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("expected invalid UTF-8 error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Merge with a HOCON document (the adapter's purpose)
// ---------------------------------------------------------------------------

func TestWithFallbackMerge(t *testing.T) {
	base, err := json5.Parse([]byte(`{db: {host: 'localhost', port: 5432}}`), "base.json5")
	if err != nil {
		t.Fatalf("json5.Parse: %v", err)
	}
	cfg, err := hocon.ParseStringWithOptions("db { host = db.example.com }\nurl = ${db.host}",
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("hocon.ParseStringWithOptions: %v", err)
	}
	merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := merged.GetString("db.host"); got != "db.example.com" {
		t.Errorf("db.host = %q, want db.example.com", got)
	}
	if got := merged.GetInt64("db.port"); got != 5432 {
		t.Errorf("db.port = %d, want 5432", got)
	}
	if got := merged.GetString("url"); got != "db.example.com" {
		t.Errorf("url = %q, want db.example.com", got)
	}
}

func TestParseFileUsesPathAsOrigin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf.json5")
	if err := os.WriteFile(path, []byte(`{a: [}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := json5.ParseFile(path)
	if err == nil || !strings.Contains(err.Error(), "conf.json5") {
		t.Fatalf("expected error naming the file, got %v", err)
	}
}
