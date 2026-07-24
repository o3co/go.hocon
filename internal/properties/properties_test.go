// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package properties

import "testing"

func wantPairs(t *testing.T, src string, want map[string]string) {
	t.Helper()
	got, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Parse(%q)[%q] = %q, want %q", src, k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("Parse(%q) produced %d keys, want %d: %v", src, len(got), len(want), got)
	}
}

func TestSeparators(t *testing.T) {
	wantPairs(t, "host=localhost\nport=8080", map[string]string{"host": "localhost", "port": "8080"})
	wantPairs(t, "host:localhost", map[string]string{"host": "localhost"})
}

// Whitespace alone separates a key from its value, so `f value = 3` is the key
// `f` holding the value `value = 3`.
func TestWhitespaceIsASeparator(t *testing.T) {
	wantPairs(t, "f value = 3", map[string]string{"f": "value = 3"})
	wantPairs(t, "host localhost", map[string]string{"host": "localhost"})
}

func TestComments(t *testing.T) {
	wantPairs(t, "# comment\n! also\nkey=val", map[string]string{"key": "val"})
}

func TestBlankLines(t *testing.T) {
	wantPairs(t, "\n\nkey=val\n\n", map[string]string{"key": "val"})
}

// Java skips whitespace before a value but not after it, so the trailing run
// survives. The pre-2026-07 parser trimmed both ends.
func TestValueKeepsTrailingWhitespace(t *testing.T) {
	wantPairs(t, "  key  =  value  ", map[string]string{"key": "value  "})
}

func TestLineContinuation(t *testing.T) {
	wantPairs(t, "a = one\\\n      two\n", map[string]string{"a": "onetwo"})
}

// An even number of trailing backslashes is an escaped backslash, not a
// continuation marker.
func TestEscapedTrailingBackslashIsNotContinuation(t *testing.T) {
	wantPairs(t, "a = end\\\\\nb = 2\n", map[string]string{"a": `end\`, "b": "2"})
}

func TestEscapes(t *testing.T) {
	wantPairs(t, "a = x\\ty\nb = \\u00e9\nc = q\\zr\n", map[string]string{
		"a": "x\ty",
		"b": "é",
		"c": "qzr", // unknown escape: the backslash is dropped, as in Java
	})
}

// A separator loses its meaning when escaped, so the key keeps the character.
func TestEscapedSeparatorInKey(t *testing.T) {
	wantPairs(t, "b\\:c = 2\n", map[string]string{"b:c": "2"})
	wantPairs(t, "a\\=b = 1\n", map[string]string{"a=b": "1"})
	wantPairs(t, "a\\ b = 1\n", map[string]string{"a b": "1"})
}

func TestSurrogatePair(t *testing.T) {
	wantPairs(t, "a = \\ud83d\\ude00\n", map[string]string{"a": "\U0001F600"})
}

func TestRepeatedKeyKeepsLast(t *testing.T) {
	wantPairs(t, "a=1\na=2\n", map[string]string{"a": "2"})
}

func TestCRLFAndCR(t *testing.T) {
	wantPairs(t, "a = 1\r\nb = 2\rc = 3\n", map[string]string{"a": "1", "b": "2", "c": "3"})
}

func TestErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unpaired high surrogate": `a = \ud83d`,
		"unpaired low surrogate":  `a = \ude00`,
		"truncated unicode":       `a = \u12`,
		"invalid unicode digits":  `a = \uZZZZ`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(src); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
		})
	}
}

func TestInvalidUTF8(t *testing.T) {
	if _, err := Parse("a=\xff"); err == nil {
		t.Fatal("invalid UTF-8 accepted, want error")
	}
}
