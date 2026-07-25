// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"strings"
	"testing"
)

// roundTrip renders a config to HOCON, parses it back, and returns the
// re-parsed config's canonical JSON alongside the original's. Equal JSON means
// the emit → parse round trip preserved the value tree, which is the emitter's
// correctness contract.
func roundTrip(t *testing.T, values map[string]any) (before, after string, text string) {
	t.Helper()
	cfg, err := FromMap(values, "test")
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	before, err = cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest(before): %v", err)
	}
	text, err = cfg.RenderHOCON()
	if err != nil {
		t.Fatalf("RenderHOCON: %v", err)
	}
	reparsed, err := ParseString(text)
	if err != nil {
		t.Fatalf("ParseString of emitted HOCON failed: %v\n--- emitted ---\n%s", err, text)
	}
	after, err = reparsed.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest(after): %v", err)
	}
	return before, after, text
}

func assertRoundTrip(t *testing.T, name string, values map[string]any) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		before, after, text := roundTrip(t, values)
		if before != after {
			t.Errorf("round trip changed the tree\n  before: %s\n  after:  %s\n--- emitted ---\n%s", before, after, text)
		}
	})
}

func TestRenderHOCONRoundTrip(t *testing.T) {
	assertRoundTrip(t, "scalars", map[string]any{
		"s": "hello", "n": 8080, "f": 1.5, "b": true, "z": false, "nul": nil,
	})
	assertRoundTrip(t, "nested-objects", map[string]any{
		"db": map[string]any{"host": "localhost", "port": 5432,
			"opts": map[string]any{"ssl": true}},
	})
	assertRoundTrip(t, "arrays", map[string]any{
		"tags":    []any{"a", "b", "c"},
		"nums":    []any{1, 2, 3},
		"objs":    []any{map[string]any{"id": 1}, map[string]any{"id": 2}},
		"nested":  []any{[]any{1, 2}, []any{3, 4}},
		"empty-a": []any{},
	})
	// Strings that would re-parse as another type MUST stay strings.
	assertRoundTrip(t, "ambiguous-strings", map[string]any{
		"looks-num":   "8080",
		"looks-float": "1.5",
		"looks-bool":  "true",
		"looks-null":  "null",
		"norway":      "no",
		"neg":         "-5",
	})
	// Strings needing quoting for their content.
	assertRoundTrip(t, "special-strings", map[string]any{
		"spaces":   "hello world",
		"empty":    "",
		"reserved": "a:b=c,d",
		"leading":  "  padded  ",
		"url":      "https://example.com/a?b=1",
		"substish": "${foo.bar}",
	})
	assertRoundTrip(t, "multiline", map[string]any{
		"block": "line1\nline2\nline3",
		"crlf":  "a\r\nb",
		"tab":   "a\tb",
	})
	// Keys that cannot be bare.
	assertRoundTrip(t, "awkward-keys", map[string]any{
		"a.b":       "dotted key",
		"has space": 1,
		"":          "empty key",
		"a=b":       true,
		"123":       "numeric key ok",
	})
	assertRoundTrip(t, "empty-object", map[string]any{
		"outer": map[string]any{"inner": map[string]any{}},
	})
	assertRoundTrip(t, "unicode", map[string]any{
		"jp": "こんにちは", "emoji": "😀", "mixed": "a😀b",
	})
	// Strings that defeat triple-quoting (embedded """, a trailing ") must fall
	// through to escaped double quotes and still round-trip.
	assertRoundTrip(t, "quote-heavy", map[string]any{
		"embedded-triple": "a\"\"\"b\nc",
		"trailing-quote":  "ends\"",
		"lone-quote":      "a\"b",
		"multiline-quote": "x\ny\"",
		"backslash":       "a\\b\\\\c",
	})
	// Empty object as an array element and as a direct value exercise the
	// value-position empty-object branch (distinct from a nested key).
	assertRoundTrip(t, "empty-object-positions", map[string]any{
		"in-array": []any{map[string]any{}, map[string]any{"a": 1}},
		"direct":   map[string]any{},
	})
}

// An unresolved config has no textual round trip through a value tree, so
// RenderHOCON must refuse it rather than emit a broken document. The
// placeholders sit at three depths so the error propagates through the nested
// object and array paths, not only the top level.
func TestRenderHOCONRejectsUnresolved(t *testing.T) {
	for name, src := range map[string]string{
		"top-level":  "a = 1\nb = ${a}\n",
		"nested":     "a = 1\nb { c = ${a} }\n",
		"in-array":   "a = 1\nb = [1, ${a}, 3]\n",
		"obj-in-arr": "a = 1\nb = [{ c = ${a} }]\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := ParseStringWithOptions(
				src, DefaultParseOptions().WithResolveSubstitutions(false),
			)
			if err != nil {
				t.Fatalf("ParseStringWithOptions: %v", err)
			}
			if _, err := cfg.RenderHOCON(); err == nil {
				t.Fatalf("RenderHOCON on unresolved %q succeeded, want error", name)
			}
		})
	}
}

// The emitted text should be idiomatic where it is safe: a plain identifier
// value and key stay bare, a number is not quoted.
func TestRenderHOCONIdiomatic(t *testing.T) {
	cfg, err := FromMap(map[string]any{
		"name": "svc", "port": 8080, "enabled": true,
	}, "")
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	got, err := cfg.RenderHOCON()
	if err != nil {
		t.Fatalf("RenderHOCON: %v", err)
	}
	for _, want := range []string{"name = svc\n", "port = 8080\n", "enabled = true\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("emitted HOCON missing %q\n--- got ---\n%s", want, got)
		}
	}
}

// A parsed HOCON document (not from FromMap) also round-trips once resolved.
func TestRenderHOCONFromParsedDocument(t *testing.T) {
	src := `
a = 1
b { c = "x", d = [1, 2, "three"] }
e = ${a}
`
	cfg, err := ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	text, err := cfg.RenderHOCON()
	if err != nil {
		t.Fatalf("RenderHOCON: %v", err)
	}
	reparsed, err := ParseString(text)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, text)
	}
	before, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest(before): %v", err)
	}
	after, err := reparsed.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest(after): %v", err)
	}
	if before != after {
		t.Errorf("parsed-doc round trip diverged\n  before: %s\n  after:  %s\n%s", before, after, text)
	}
}
