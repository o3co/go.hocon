// S8.6 — Unquoted strings MUST NOT begin with `-` (unless followed by a digit
// forming a number prefix) or any digit `0-9` (per HOCON.md L270-276).
// Issue #60: https://github.com/o3co/go.hocon/issues/60
// Issue #81 (key numeric support): folded back into s8SuccessFixtures below.
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
// xx.hocon ground truth. us08/us09 require parseKey to accept TokenInt+
// TokenString key-concat and TokenFloat dot-split (cluster-3c follow-up #81).
var s8SuccessFixtures = []string{
	"us01-digit-prefix-with-tail",
	"us04-hyphen-with-digit",
	"us05-number-then-comment",
	"us06-embedded-digits",
	"us07-embedded-hyphen",
	"us08-numeric-key-positive",
	"us09-dotted-number-key",
	"us10-greedy-backtrack-exp",
	"us11-greedy-backtrack-frac",
	"us12-hex-prefix",
	"us14-multi-dot-version",
	"us16-negative-with-tail",
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

// TestS8_6_NumericKeyConcat covers the parser-level adjacent-token concat
// added in #81-followup, beyond what us08/us09 fixtures exercise. These pin
// the merge+resplit semantics for TokenFloat / TokenInt + unquoted tail.
func TestS8_6_NumericKeyConcat_FloatPlusTail(t *testing.T) {
	// 3.14abc = 1 → segments ["3","14"] then concat "abc" → re-split → ["3","14abc"]
	cfg, err := hocon.ParseString("3.14abc = 1")
	if err != nil {
		t.Fatalf("expected parse success for 3.14abc = 1, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := got["3"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level key \"3\" → object, got %T (%v)", got["3"], got)
	}
	if v, ok2 := inner["14abc"]; !ok2 || !jsonEqual(v, 1) {
		t.Errorf("expected 3.14abc = 1, got inner=%v", inner)
	}
}

func TestS8_6_NumericKeyConcat_IntPlusDottedTail(t *testing.T) {
	// 123abc.foo = 1 → concat "123" + "abc.foo" → re-split → ["123abc", "foo"]
	cfg, err := hocon.ParseString("123abc.foo = 1")
	if err != nil {
		t.Fatalf("expected parse success for 123abc.foo = 1, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := got["123abc"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level key \"123abc\" → object, got %T (%v)", got["123abc"], got)
	}
	if v, ok2 := inner["foo"]; !ok2 || !jsonEqual(v, 1) {
		t.Errorf("expected 123abc.foo = 1, got inner=%v", inner)
	}
}

// TestS8_6_NumericKeyConcat_KeywordTail covers the round-2 review finding
// (Codex): the concat-tail predicate must also accept TokenBool / TokenNull
// (and TokenInclude) so that `123true = 1`, `123null = 1` etc. behave
// symmetrically with `123true.foo = 1` (which the lexer already produces
// as a single TokenString and therefore already worked). Without keyword
// support, the path expression was inconsistently rejected depending on
// whether a `.` followed the keyword.
func TestS8_6_NumericKeyConcat_KeywordTail(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`123true = 1`, "123true"},
		{`123false = 1`, "123false"},
		{`123null = 1`, "123null"},
		{`123include = 1`, "123include"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			cfg, err := hocon.ParseString(c.src)
			if err != nil {
				t.Fatalf("expected parse success for %s, got: %v", c.src, err)
			}
			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if v, ok := got[c.want]; !ok || !jsonEqual(v, 1) {
				t.Errorf("expected key %q → 1, got %v", c.want, got)
			}
		})
	}
}

// TestS8_6_KeyPathHyphenDigitSegmentAllowed pins the positive arm of the
// S8.6 per-segment rule: `a.-1foo = 1` has segment "-1foo" which begins
// with '-' but is immediately followed by a digit, so it must be accepted.
// Complements TestS8_6_KeyPathHyphenSegmentRejected (which covers the
// "no digit after `-`" arm).
func TestS8_6_KeyPathHyphenDigitSegmentAllowed(t *testing.T) {
	cfg, err := hocon.ParseString("a.-1foo = 1")
	if err != nil {
		t.Fatalf("expected parse success for a.-1foo = 1, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := got["a"].(map[string]any)
	if !ok {
		t.Fatalf("expected key \"a\" → object, got %T (%v)", got["a"], got)
	}
	if v, ok2 := inner["-1foo"]; !ok2 || !jsonEqual(v, 1) {
		t.Errorf("expected a.-1foo = 1, got inner=%v", inner)
	}
}

// TestS8_6_NumericKeyConcat_ResplitValidatesHyphenSegments covers the
// concat-resplit path's S8.6 enforcement: after merging a numeric leading
// token with an unquoted tail, the merged value is re-split on '.' and each
// resulting segment must pass validateKeySegment. `123abc.-foo = 1` merges
// to "123abc.-foo", splits to ["123abc","-foo"], and the "-foo" segment
// must be rejected by the same rule as the initial-split path.
func TestS8_6_NumericKeyConcat_ResplitValidatesHyphenSegments(t *testing.T) {
	_, err := hocon.ParseString("123abc.-foo = 1")
	if err == nil {
		t.Error("expected parse error for 123abc.-foo = 1 (post-merge segment starts with '-' no-digit), parse succeeded")
	}
}

// TestS8_6_NumericKeyConcat_ResplitAcceptsHyphenDigit pins the positive arm
// of the same post-merge rule: `123abc.-1foo = 1` has post-merge segments
// ["123abc","-1foo"] and "-1foo" is valid (hyphen followed by digit).
func TestS8_6_NumericKeyConcat_ResplitAcceptsHyphenDigit(t *testing.T) {
	cfg, err := hocon.ParseString("123abc.-1foo = 1")
	if err != nil {
		t.Fatalf("expected parse success for 123abc.-1foo = 1, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := got["123abc"].(map[string]any)
	if !ok {
		t.Fatalf("expected key \"123abc\" → object, got %T (%v)", got["123abc"], got)
	}
	if v, ok2 := inner["-1foo"]; !ok2 || !jsonEqual(v, 1) {
		t.Errorf("expected 123abc.-1foo = 1, got inner=%v", inner)
	}
}

// TestS8_6_QuotedKeyConcatNotResplit guards against the regression Codex
// caught in #81-followup review: the numeric-key concat branch must NOT run
// after a quoted key segment, because a literal `.` inside the quoted part
// must not be reinterpreted as a path separator. Before the gating fix,
// `"a.b"c = 1` was silently accepted as path ["a", "bc"]. We pin the pre-PR
// behavior (parse error) until cross-impl convention for quoted+unquoted key
// concat is settled and a separate spec item lands.
func TestS8_6_QuotedKeyConcatNotResplit(t *testing.T) {
	_, err := hocon.ParseString(`"a.b"c = 1`)
	if err == nil {
		t.Error(`expected parse error for "a.b"c = 1 (quoted+unquoted key concat must not silently re-split quoted dots); got success`)
	}
}
