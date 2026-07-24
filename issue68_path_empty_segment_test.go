// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// xx.hocon#68 — two spec-compliance gaps closed together, both about what a
// parser must REFUSE rather than what it must accept.
//
// S11.7 (HOCON.md L515-519): "If a path element is an empty string, it must
// always be quoted. That is, `a."".b` is a valid path with three elements, and
// the middle element is an empty string. But `a..b` is invalid and should
// generate an error. Following the same rule, a path that starts or ends with
// a `.` is invalid and should generate an error."
// go.hocon already rejected the trailing-dot key form and every substitution
// path form (`${?a..b}`, `${?.a}`, `${?a.}` — enforced by the lexer's
// parseSubstBody state machine), but the KEY path parser silently dropped
// empty pieces when splitting a token on '.', so `a..b: 3` collapsed to
// {"a":{"b":3}} and `.a: 3` to {"a":3}.
//
// S8.1 (HOCON.md L245-247): backtick is a forbidden character in unquoted
// strings. Every other member of that set was already rejected; only '`'
// leaked through, so `a = ` + "`t`" + ` parsed as a string value and
// "`k` = 1" as a key. Parens `(` `)` are deliberately NOT in the forbidden
// set (xx.hocon#34) and are untouched here.
//
// Fixtures: testdata/hocon/path-empty-segment/pe01-pe08 and
// testdata/hocon/unquoted-forbidden/uf01-uf04 (xx.hocon SHA e4e9d64).
// Expected: testdata/expected/{path-empty-segment,unquoted-forbidden}/,
// where `<stem>.error` means "must fail" and `<stem>-expected.json` means
// "must succeed with this value".

package hocon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

const (
	peConfDir     = "testdata/hocon/path-empty-segment"
	peExpectedDir = "testdata/expected/path-empty-segment"
	ufConfDir     = "testdata/hocon/unquoted-forbidden"
	ufExpectedDir = "testdata/expected/unquoted-forbidden"
)

// runSidecarFixtures drives every .conf in confDir against its sidecar in
// expectedDir: `<stem>.error` demands a parse error, `<stem>-expected.json`
// demands a successful parse whose value matches the JSON. A fixture with
// neither sidecar is a fixture-author error or a stale `make testdata`.
func runSidecarFixtures(t *testing.T, label, confDir, expectedDir string) {
	t.Helper()

	if _, err := os.Stat(confDir); os.IsNotExist(err) {
		t.Skipf("%s fixtures missing at %s; run `make testdata`", label, confDir)
	}
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Skipf("%s expected dir missing at %s; run `make testdata`", label, expectedDir)
	}

	entries, err := os.ReadDir(confDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", confDir, err)
	}

	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		stem := strings.TrimSuffix(name, ".conf")
		confPath := filepath.Join(confDir, name)
		errorSidecar := filepath.Join(expectedDir, stem+".error")
		jsonSidecar := filepath.Join(expectedDir, stem+"-expected.json")

		switch {
		case fileExists(errorSidecar):
			ran++
			t.Run(stem+"/must-error", func(t *testing.T) {
				if _, parseErr := hocon.ParseFile(confPath); parseErr == nil {
					t.Errorf("%s: expected a parse error (per %s), got success", stem, errorSidecar)
				}
			})
		case fileExists(jsonSidecar):
			ran++
			t.Run(stem+"/must-parse-ok", func(t *testing.T) {
				cfg, parseErr := hocon.ParseFile(confPath)
				if parseErr != nil {
					t.Fatalf("ParseFile(%s): %v (fixture has %s, so it must parse)", confPath, parseErr, jsonSidecar)
				}

				expectedData, readErr := os.ReadFile(jsonSidecar)
				if readErr != nil {
					t.Fatalf("ReadFile(%s): %v", jsonSidecar, readErr)
				}
				var want any
				if jsonErr := json.Unmarshal(expectedData, &want); jsonErr != nil {
					t.Fatalf("Unmarshal expected JSON (%s): %v", jsonSidecar, jsonErr)
				}

				got := make(map[string]any)
				if unmarshalErr := cfg.Unmarshal(&got); unmarshalErr != nil {
					t.Fatalf("cfg.Unmarshal: %v", unmarshalErr)
				}
				if !jsonEqual(got, want) {
					gotPretty, _ := json.MarshalIndent(got, "", "  ")
					t.Errorf("%s: result mismatch\ngot:\n%s\nwant:\n%s", stem, gotPretty, expectedData)
				}
			})
		default:
			t.Errorf("%s: no sidecar for %s in %s (fixture-author error or stale testdata fetch)", label, stem, expectedDir)
		}
	}

	if ran == 0 {
		t.Errorf("no %s fixtures were tested; check %s and %s", label, confDir, expectedDir)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestS11_7_PathEmptySegmentFixtures drives pe01–pe08.
func TestS11_7_PathEmptySegmentFixtures(t *testing.T) {
	runSidecarFixtures(t, "path-empty-segment", peConfDir, peExpectedDir)
}

// TestS8_1_UnquotedForbiddenFixtures drives uf01–uf04.
func TestS8_1_UnquotedForbiddenFixtures(t *testing.T) {
	runSidecarFixtures(t, "unquoted-forbidden", ufConfDir, ufExpectedDir)
}

// TestIssue68_KeyPathEmptySegmentRejected covers the gap: adjacent, leading and
// repeated dots in KEY position must be BadPath, at top level and nested.
func TestIssue68_KeyPathEmptySegmentRejected(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"adjacent-dots", "a..b: 3\n"},
		{"leading-dot", ".a: 3\n"},
		{"triple-dots", "a...c: 4\n"},
		{"adjacent-dots-nested", "o { a..b: 3 }\n"},
		{"triple-dots-then-quoted-empty", "a...c.\"\": 4\n"},
		{"quoted-then-adjacent-dots", "\"a\"..b = 1\n"},
		{"adjacent-dots-around-whitespace", "a .. b = 1\n"},
		{"numeric-lead-adjacent-dots", "123..abc = 1\n"},
		{"leading-dot-nested", "o { .a: 3 }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := hocon.ParseString(tc.src); err == nil {
				t.Errorf("ParseString(%q): expected an empty-path-element error (HOCON.md L515-519), got success", tc.src)
			}
		})
	}
}

// TestIssue68_QuotedEmptySegmentStillValid pins S11.6: a quoted "" IS a legal
// path element, so `a."".b` must keep producing a three-level nesting. This is
// the boundary the S11.7 rejection must not cross.
func TestIssue68_QuotedEmptySegmentStillValid(t *testing.T) {
	cfg, err := hocon.ParseString("a.\"\".b: 3\n")
	if err != nil {
		t.Fatalf("ParseString: %v (a.\"\".b is a valid 3-element path per HOCON.md L515-517)", err)
	}
	if !cfg.Has("a") {
		t.Fatalf("missing key a; keys=%v", cfg.Keys())
	}
	if got := cfg.GetInt64("a.\"\".b"); got != 3 {
		t.Errorf(`a."".b = %d, want 3`, got)
	}
}

// TestIssue68_LeadingQuotedEmptySegmentStillValid: an empty FIRST element is
// legal too as long as it is quoted — the leading-dot rejection keys off the
// bare '.', not off emptiness per se.
func TestIssue68_LeadingQuotedEmptySegmentStillValid(t *testing.T) {
	cfg, err := hocon.ParseString("\"\".b: 3\n")
	if err != nil {
		t.Fatalf("ParseString: %v (a quoted empty leading element is valid)", err)
	}
	if got := cfg.GetInt64("\"\".b"); got != 3 {
		t.Errorf(`"".b = %d, want 3`, got)
	}
}

// TestIssue68_TrailingDotStillRejected is a regression guard: pe03 already
// passed before the fix via the post-loop trailingDot check, and the new
// empty-element rejection must not displace that error path.
func TestIssue68_TrailingDotStillRejected(t *testing.T) {
	for _, src := range []string{"a.: 3\n", "a b. = 1\n"} {
		if _, err := hocon.ParseString(src); err == nil {
			t.Errorf("ParseString(%q): expected a trailing-period error, got success", src)
		}
	}
}

// TestIssue68_SubstitutionPathStillRejected is a regression guard for pe08 and
// friends: the lexer's substitution-path state machine already enforced S11.7
// and must keep doing so.
func TestIssue68_SubstitutionPathStillRejected(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"adjacent-dots", "x = 1\ny = ${?a..b}\n"},
		{"leading-dot", "x = 1\ny = ${?.a}\n"},
		{"trailing-dot", "x = 1\ny = ${?a.}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := hocon.ParseString(tc.src); err == nil {
				t.Errorf("ParseString(%q): expected an empty-segment-in-path error, got success", tc.src)
			}
		})
	}
}

// TestIssue68_BacktickRejectedInUnquoted covers S8.1: '`' is in the HOCON.md
// L245-247 forbidden set, in value, key and mid-token position alike.
func TestIssue68_BacktickRejectedInUnquoted(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"value", "a = `t`\n"},
		{"key", "`k` = 1\n"},
		{"mid-token", "a = x`y\n"},
		{"value-in-array", "a = [ `t` ]\n"},
		{"value-in-object", "a { b = `t` }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := hocon.ParseString(tc.src); err == nil {
				t.Errorf("ParseString(%q): expected a forbidden-character error for '`' (HOCON.md L245-247), got success", tc.src)
			}
		})
	}
}

// TestIssue68_BacktickInQuotedStringStillOrdinary pins uf04: the forbidden set
// applies to UNQUOTED strings only — inside quotes a backtick is content.
func TestIssue68_BacktickInQuotedStringStillOrdinary(t *testing.T) {
	cfg, err := hocon.ParseString("a = \"x`y\"\nb = \"\"\"p`q\"\"\"\n\"`k`\" = 1\n")
	if err != nil {
		t.Fatalf("ParseString: %v (backtick inside quotes is ordinary content)", err)
	}
	if got := cfg.GetString("a"); got != "x`y" {
		t.Errorf("a = %q, want %q", got, "x`y")
	}
	if got := cfg.GetString("b"); got != "p`q" {
		t.Errorf("b = %q, want %q", got, "p`q")
	}
	if got := cfg.GetInt64("\"`k`\""); got != 1 {
		t.Errorf("`k` = %d, want 1", got)
	}
}

// TestIssue68_ParensStillAllowed is a regression guard for xx.hocon#34: adding
// '`' to the forbidden set must not drag '(' / ')' in with it.
func TestIssue68_ParensStillAllowed(t *testing.T) {
	cfg, err := hocon.ParseString("a = hello (world)\n")
	if err != nil {
		t.Fatalf("ParseString: %v (parens are not in the HOCON.md L245-247 forbidden set — xx.hocon#34)", err)
	}
	if got := cfg.GetString("a"); got != "hello (world)" {
		t.Errorf("a = %q, want %q", got, "hello (world)")
	}
}
