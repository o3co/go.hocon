// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package jsonc_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/jsonc"
)

func parse(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := jsonc.Parse([]byte(src), "test.jsonc")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return cfg
}

func TestCommentsAndTrailingCommas(t *testing.T) {
	cfg := parse(t, `{
	  // a line comment
	  "a": 1, /* a block
	              comment spanning lines */
	  "b": [1, 2, ],
	  "c": { "d": true, },
	}`)

	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
	if got := cfg.GetInt64Slice("b"); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("b = %#v, want [1 2]", got)
	}
	if !cfg.GetBool("c.d") {
		t.Error("c.d = false, want true")
	}
}

// Comment markers inside string literals are data, not syntax.
func TestCommentMarkersInsideStringsSurvive(t *testing.T) {
	cfg := parse(t, `{
	  "url": "https://example.com/a//b",
	  "block": "a /* not a comment */ b",
	  "comma": "trailing, inside"
	}`)

	for path, want := range map[string]string{
		"url":   "https://example.com/a//b",
		"block": "a /* not a comment */ b",
		"comma": "trailing, inside",
	} {
		if got := cfg.GetString(path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestEscapedQuoteDoesNotEndString(t *testing.T) {
	cfg := parse(t, `{"a": "he said \"//\" loudly", "b": 1}`)
	if got := cfg.GetString("a"); got != `he said "//" loudly` {
		t.Errorf("a = %q", got)
	}
	if got := cfg.GetInt64("b"); got != 1 {
		t.Errorf("b = %d, want 1", got)
	}
}

// F0.5: an integer stays an integer instead of round-tripping through float64.
func TestLargeIntegerKeepsPrecision(t *testing.T) {
	cfg := parse(t, `{"big": 9007199254740993}`)
	if got := cfg.GetInt64("big"); got != 9007199254740993 {
		t.Errorf("big = %d, want 9007199254740993 (float64 would give ...92)", got)
	}
}

func TestIntegerOverflowIsError(t *testing.T) {
	_, err := jsonc.Parse([]byte(`{"big": 99999999999999999999}`), "test.jsonc")
	if err == nil {
		t.Fatal("out-of-range integer accepted, want error")
	}
	if !strings.Contains(err.Error(), "F0.5") {
		t.Errorf("error %q does not cite the spec item F0.5", err)
	}
}

func TestFloatsAndScientificNotation(t *testing.T) {
	cfg := parse(t, `{"ratio": 0.5, "sci": 1e3}`)
	if got := cfg.GetString("ratio"); got != "0.5" {
		t.Errorf("ratio rendered as %q, want 0.5", got)
	}
	if got := cfg.GetInt64("sci"); got != 1000 {
		t.Errorf("sci = %d, want 1000", got)
	}
}

// F0.3: a config root has to be an object.
func TestArrayRootRejected(t *testing.T) {
	_, err := jsonc.Parse([]byte(`[1, 2]`), "test.jsonc")
	if err == nil {
		t.Fatal("array root accepted, want error")
	}
	if !strings.Contains(err.Error(), "F0.3") {
		t.Errorf("error %q does not cite the spec item F0.3", err)
	}
}

func TestSyntaxErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unterminated block comment": `{"a": 1 /* oops`,
		"unterminated string":        `{"a": "oops}`,
		"trailing data":              `{"a": 1} {"b": 2}`,
		"not json at all":            `nonsense`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := jsonc.Parse([]byte(src), "test.jsonc"); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
		})
	}
}

// F0.2: a ${...} in foreign data is text, not a reference.
func TestSubstitutionSyntaxStaysLiteral(t *testing.T) {
	cfg := parse(t, `{"a": "${foo.bar}"}`)
	resolved, err := cfg.Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != "${foo.bar}" {
		t.Errorf("a = %q, want the literal ${foo.bar}", got)
	}
}

func TestUseAsSubstitutionSourceUnderHOCON(t *testing.T) {
	base, err := jsonc.Parse([]byte(`{
	  // owned by the frontend build
	  "compilerOptions": { "outDir": "dist" },
	}`), "tsconfig.json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg, err := hocon.ParseStringWithOptions(
		`artifacts = "./"${compilerOptions.outDir}"/bundle.js"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := merged.GetString("artifacts"); got != "./dist/bundle.js" {
		t.Errorf("artifacts = %q, want ./dist/bundle.js", got)
	}
}
