// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// xx.hocon#130 — S1.2.4: a raw C0 control character (U+0000–U+001F) inside a
// single-quoted string is a parse error, as in JSON and Lightbend. The lexer
// rejected only LF; tab, CR, U+0001, U+001F and NUL were accepted in value,
// key, substitution-path and include position. Triple-quoted (S9.1) and
// unquoted (S8.8) strings keep controls verbatim, and DEL / C1 are legal.
//
// The emitter half (E18 decision 5) is in render_hocon_test.go.
//
// Fixtures: testdata/hocon/quoted-control/qc01-qc12 (xx.hocon#130).

package hocon_test

import (
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

const (
	qcConfDir     = "testdata/hocon/quoted-control"
	qcExpectedDir = "testdata/expected/quoted-control"
)

// TestS1_2_4_QuotedControlFixtures drives qc01–qc12.
func TestS1_2_4_QuotedControlFixtures(t *testing.T) {
	runSidecarFixtures(t, "quoted-control", qcConfDir, qcExpectedDir)
}

// TestS1_2_4_RawControlInQuotedStringRejected sweeps the whole C0 range in
// value position and pins the other positions that share the lexer path.
func TestS1_2_4_RawControlInQuotedStringRejected(t *testing.T) {
	for r := rune(0); r < 0x20; r++ {
		if r == '\n' {
			continue // reported as an unterminated string, pinned elsewhere
		}
		src := "a = \"x" + string(r) + "y\"\n"
		if _, err := hocon.ParseString(src); err == nil {
			t.Errorf("U+%04X in a quoted value: expected a parse error, got success", r)
		} else if !strings.Contains(err.Error(), "control character") {
			t.Errorf("U+%04X: error %q does not name the control character", r, err)
		}
	}
	positions := map[string]string{
		"key":            "\"a\tb\" = 1\n",
		"path-segment":   "a.\"b\x01\" = 1\n",
		"array-element":  "a = [\"x\x00\"]\n",
		"subst-path":     "x = 1\na = ${\"x\t\"}\n",
		"optional-subst": "a = ${?\"x\x1f\"}\n",
		"include":        "include \"x\ty\"\n",
		"concat":         "a = foo \"x\x01\" bar\n",
	}
	for name, src := range positions {
		t.Run(name, func(t *testing.T) {
			if _, err := hocon.ParseString(src); err == nil {
				t.Errorf("ParseString(%q): expected a parse error, got success", src)
			}
		})
	}
}

// TestS1_2_4_ControlErrorPosition: the error points at the control
// character, not at the opening quote.
func TestS1_2_4_ControlErrorPosition(t *testing.T) {
	_, err := hocon.ParseString("k = 1\na = \"xy\tz\"\n")
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if !strings.Contains(err.Error(), "line 2, col 8") {
		t.Errorf("error %q does not point at the tab (line 2, col 8)", err)
	}
}

// TestS1_2_4_IncludeArgumentReportsLexerError: in include position the
// lexer's error is surfaced, not mistaken for `include` used as a key.
func TestS1_2_4_IncludeArgumentReportsLexerError(t *testing.T) {
	for _, src := range []string{
		"include \"x\ty\"\n",
		"include required(\"x\ty\")\n",
		"include file(\"x\ty\")\n",
	} {
		_, err := hocon.ParseString(src)
		if err == nil {
			t.Errorf("ParseString(%q): expected a parse error, got success", src)
			continue
		}
		if !strings.Contains(err.Error(), "control character U+0009") {
			t.Errorf("ParseString(%q): error %q does not name the control character", src, err)
		}
	}
}

// TestS1_2_4_ControlCharsLegalOutsideSingleQuotes pins the boundary the
// rejection must not cross.
func TestS1_2_4_ControlCharsLegalOutsideSingleQuotes(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"escaped", "a = \"x\\u0000\\u0001\\t\\u001fy\"\n", "x\x00\x01\t\x1fy"},
		{"del-c1", "a = \"x\x7f\u0085\u009fy\"\n", "x\x7f\u0085\u009fy"},
		{"triple-quoted", "a = \"\"\"x\x00\x01\t\r\x1fy\"\"\"\n", "x\x00\x01\t\r\x1fy"},
		{"unquoted", "a = x\x01y\x00z\n", "x\x01y\x00z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := hocon.ParseString(tc.src)
			if err != nil {
				t.Fatalf("ParseString(%q): %v", tc.src, err)
			}
			if got := cfg.GetString("a"); got != tc.want {
				t.Errorf("a = %q, want %q", got, tc.want)
			}
		})
	}
}
