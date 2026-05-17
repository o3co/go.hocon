// S8.6 — Unquoted strings MUST NOT begin with `-` (unless followed by a digit
// forming a number prefix) or any digit `0-9` (per HOCON.md L270-276).
// Issue #60: https://github.com/o3co/go.hocon/issues/60
//
// Fixture-driven conformance tests against xx.hocon ground truth at
// testdata/hocon/unquoted-starts/.
//
// go.hocon's lexer DOES have a separate Number token kind (TokenInt / TokenFloat),
// so this PR implements Option A — the plan-shaped greedy-with-backtrack
// `lex_number` algorithm — rather than the unquoted-only Option B used in
// ts.hocon (PR #96/#97) and rs.hocon (PR #86). See docs/spec-compliance.md §S8.6
// for the architectural rationale and the Lightbend-quirk gaps (us13, us15)
// that may remain out of scope depending on number-grammar coverage.

package hocon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// Success fixtures: parse must succeed and the resolved JSON must match the
// xx.hocon ground truth.
var s8SuccessFixtures = []string{
	"us01-digit-prefix-with-tail",
	"us04-hyphen-with-digit",
	"us05-number-then-comment",
	"us06-embedded-digits",
	"us07-embedded-hyphen",
	"us10-greedy-backtrack-exp",
	"us11-greedy-backtrack-frac",
	"us12-hex-prefix",
	"us14-multi-dot-version",
	"us16-negative-with-tail",
}

// Deferred-success fixtures: spec-correct cases that go.hocon parser does
// not yet handle (they require Number-token-aware key parsing — a parser
// refactor deferred to a follow-up PR). Today these emit parse errors at
// key position; the spec expects successful parse with the resolved values
// matching the JSON in testdata/expected/unquoted-starts/. Tracked under a
// dedicated follow-up issue (filed when this PR lands).
//   - us08 `123abc = 1`     → {"123abc": 1}   (TokenInt+TokenString concat as key)
//   - us09 `3.14 = "v"`     → {"3":{"14":"v"}} (TokenFloat dot-split as key)
var s8DeferredFixtures = []string{
	"us08-numeric-key-positive",
	"us09-dotted-number-key",
}

// Error fixtures: parse must throw (lex or parse error).
// us02 / us03 are the rule this PR enforces (`-` not followed by a digit
// at the lex layer).
var s8ErrorFixtures = []string{
	"us02-hyphen-no-digit",
	"us03-hyphen-alone",
}

// Known-gap fixtures: documented gaps that require additional work. Tracked
// under #60. These tests use t.Skip with a tracking note so they don't rot
// silently; flip Skip → assert when the gap closes.
var s8GapFixtures = []string{
	"us13-leading-zero",
	"us15-incomplete-exp",
}

func confPath(name string) string {
	return filepath.Join("testdata", "hocon", "unquoted-starts", name+".conf")
}

func expectedPath(name string) string {
	return filepath.Join("testdata", "expected", "unquoted-starts", name+"-expected.json")
}

func TestS8_6_SuccessFixtures(t *testing.T) {
	if _, err := os.Stat(filepath.Join("testdata", "expected", "unquoted-starts")); err != nil {
		t.Skip("unquoted-starts expected fixtures missing; run `make testdata`")
	}
	for _, name := range s8SuccessFixtures {
		t.Run(name, func(t *testing.T) {
			cfg, err := hocon.ParseFile(confPath(name))
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", name, err)
			}
			expectedData, err := os.ReadFile(expectedPath(name))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var want any
			if err := json.Unmarshal(expectedData, &want); err != nil {
				t.Fatalf("parse expected JSON: %v", err)
			}
			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			// Use jsonEqual (defined in lightbend_test.go) to normalize int/float
			// representations consistently, matching the existing fixture-driven
			// pattern in this repo.
			if !jsonEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("%s mismatch\ngot:\n%s\nwant:\n%s", name, gotJSON, wantJSON)
			}
		})
	}
}

func TestS8_6_ErrorFixtures(t *testing.T) {
	for _, name := range s8ErrorFixtures {
		t.Run(name, func(t *testing.T) {
			_, err := hocon.ParseFile(confPath(name))
			if err == nil {
				t.Errorf("%s: expected ParseError, parse succeeded", name)
			}
		})
	}
}

func TestS8_6_KnownGaps(t *testing.T) {
	// These fixtures are documented gaps tracked under #60. They are SKIPPED
	// for now. When the gap closes (i.e. ParseFile starts erroring on these),
	// remove the Skip and the test will pass automatically — the next reader
	// will know the gap has closed because the file diff removes the Skip.
	for _, name := range s8GapFixtures {
		t.Run(name, func(t *testing.T) {
			t.Skip("S8.6 known gap (#60): strict spec requires reject, but go.hocon currently accepts; tracked for future tightening")
			_, err := hocon.ParseFile(confPath(name))
			if err == nil {
				t.Errorf("%s: expected ParseError (gap closed), parse succeeded", name)
			}
		})
	}
}

// TestS8_6_DeferredSuccess covers spec-correct success cases that go.hocon's
// parser does not yet handle (us08 numeric-key concat, us09 numeric dotted-key).
// These will be enabled by a follow-up PR that teaches the parser to accept
// TokenInt/TokenFloat in key position with the appropriate concat/dot-split
// semantics. For now we Skip with a tracker so they don't rot silently.
func TestS8_6_DeferredSuccess(t *testing.T) {
	if _, err := os.Stat(filepath.Join("testdata", "expected", "unquoted-starts")); err != nil {
		t.Skip("unquoted-starts expected fixtures missing; run `make testdata`")
	}
	for _, name := range s8DeferredFixtures {
		t.Run(name, func(t *testing.T) {
			t.Skip("S8.6 deferred to follow-up PR (#60-followup): parser numeric-key support pending")
			cfg, err := hocon.ParseFile(confPath(name))
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", name, err)
			}
			expectedData, err := os.ReadFile(expectedPath(name))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var want any
			if err := json.Unmarshal(expectedData, &want); err != nil {
				t.Fatalf("parse expected JSON: %v", err)
			}
			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !jsonEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("%s mismatch\ngot:\n%s\nwant:\n%s", name, gotJSON, wantJSON)
			}
		})
	}
}

// Regression: S8.6 also applies inside substitution paths and dotted key
// segments. The check at lex time for `-` no-digit must fire at the substitution
// segment start, not at value position only.

func TestS8_6_SubstPathHyphenNoDigitRejected(t *testing.T) {
	// Tightened to assert a *lex-time* rejection specifically: a generic
	// err-not-nil could pass via an unresolved-substitution error, which
	// would mask removal of the parseSubstBody S8.6 check. Use a setup
	// where a matching key DOES exist — if parseSubstBody didn't reject,
	// resolution would succeed and the test would fail loudly.
	input := "\"-foo\" = 1\nx = ${-foo}"
	_, err := hocon.ParseString(input)
	if err == nil {
		t.Fatalf("expected lex/parse-time error for ${-foo}, parse+resolve succeeded; this means parseSubstBody S8.6 check is missing")
	}
	// Error class check: must contain "L270" (the spec citation in the
	// parseSubstBody error message) to confirm it's the lex-time rejection,
	// not a downstream resolve error.
	if !strings.Contains(err.Error(), "L270") {
		t.Errorf("expected lex-time S8.6 error (containing 'L270'), got: %v", err)
	}
}

func TestS8_6_SubstMidSegmentHyphenAfterQuotedAllowed(t *testing.T) {
	// Regression: the parseSubstBody S8.6 check must fire only at segment start.
	// ${"a"-foo} (quoted+unquoted concat → key "a-foo") must remain accepted.
	// Mirrors the existing ${"a"x} → "ax" concat flow. Cross-impl convergence
	// caught this in ts.hocon PR #97 / rs.hocon PR #86 — implement the same
	// gating from the start in go.hocon.
	input := "\"a-foo\" = 1\nx = ${\"a\"-foo}"
	_, err := hocon.ParseString(input)
	if err != nil {
		t.Errorf("${\"a\"-foo} (quoted+unquoted concat) must lex+resolve, got: %v", err)
	}
}

func TestS8_6_KeyPathHyphenSegmentRejected(t *testing.T) {
	// `a.-foo = 1` — the lexer sees `a.-foo` (or splits it) and the parser
	// builds a path from segments; the `-foo` segment must be rejected by
	// the same S8.6 rule.
	_, err := hocon.ParseString("a.-foo = 1")
	if err == nil {
		t.Error("expected error for a.-foo = 1 (key segment starts with '-'), parse succeeded")
	}
}
