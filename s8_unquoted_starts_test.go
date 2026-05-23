// S8.6 — Unquoted strings at value-position MAY begin with `-` (treated as
// unquoted text when not followed by a digit) or with digits (greedy Java
// numeric semantics — `readNumber` consumes a JSON-shaped int frac? exp?
// run, with frac/exp parts each backtracking independently at lex time
// when not fully consumed, e.g. `1ex` → TokenInt(1) + TokenString("ex")
// for value-concat). Unlike ts/rs, go.hocon has separate TokenInt /
// TokenFloat kinds, so a successfully-lexed numeric token then validates
// via strconv.ParseInt / ParseFloat in the parser — failure there is an
// error, not a string fallback (that case is covered by the lex-time
// backtrack leaving a suffix for the next-token pass). Concat-
// continuation positions accept any unquoted-permissible character except
// `+` as a continuation of the existing unquoted run.
//
// This reading was established by the E8 amendment in
// xx.hocon/docs/extra-spec-conventions.md (rewritten 2026-05-20 as
// xx.hocon#32 / commit dd102e8, driven by external issue xx.hocon#31). It
// adopts Lightbend's pragmatic reading of HOCON.md L270-276 — "begin" =
// value-position begin (first component of a concatenation), not
// token-position begin at any lexer offset.
//
// Subst-body path expressions (${-foo}) and key-path segments (a.-foo = 1)
// keep their existing strict checks — those rules are about path-element
// composition, not value-position unquoted strings, and remain out of E8
// scope.
//
// Fixture-driven conformance tests against xx.hocon ground truth at
// testdata/hocon/unquoted-starts/.

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
// xx.hocon ground truth. us02/us03/us13 joined this list as part of the E8
// amendment (previously in s8ErrorFixtures / s8GapFixtures under the strict
// reading). us17-us30 are new concat-continuation fixtures from probe groups
// A/B/D/E.
var s8SuccessFixtures = []string{
	"us01-digit-prefix-with-tail",
	"us02-hyphen-no-digit",
	"us03-hyphen-alone",
	"us04-hyphen-with-digit",
	"us05-number-then-comment",
	"us06-embedded-digits",
	"us07-embedded-hyphen",
	"us08-numeric-key-positive",
	"us09-dotted-number-key",
	"us10-greedy-backtrack-exp",
	"us11-greedy-backtrack-frac",
	"us12-hex-prefix",
	"us13-leading-zero",
	"us14-multi-dot-version",
	"us16-negative-with-tail",
	"us17-concat-subst-dash-text",
	"us18-concat-subst-dash-only",
	"us19-concat-subst-double-dash",
	"us20-concat-subst-dash-digit",
	"us21-concat-subst-digit-text",
	"us22-concat-subst-dot-text",
	"us23-concat-subst-underscore",
	"us24-concat-quoted-dash-text",
	"us25-concat-quoted-dot-text",
	"us26-concat-quoted-digit-text",
	"us27-concat-subst-dash-subst",
	"us28-concat-subst-dash-subst-other",
	"us29-concat-unquoted-dash-subst",
	"us30-concat-quoted-dash-subst",
}

// Error fixtures: parse must throw (lex or parse error). us15 (`a = 1e+x`)
// joined this list as part of the E8 amendment — the `+` reservation enforced
// at value-start and concat-continuation positions also catches the bare `+`
// that backtracks out of the exponent in `1e+x`, so the lex-time error fires
// at the `+` token regardless of Lightbend's value-parser-layer message.
var s8ErrorFixtures = []string{
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
			// Sidecar pre-check with actionable message (mirrors the pattern
			// requested by Copilot review on the rs.hocon E8 PR — surfacing
			// missing fixtures/sidecars with the "run `make testdata`" hint
			// instead of a low-signal os.ReadFile error).
			fp := confPath(name)
			jp := expectedPath(name)
			if _, err := os.Stat(fp); err != nil {
				t.Fatalf("fixture missing: %s — run `make testdata` to sync from xx.hocon (%v)", fp, err)
			}
			if _, err := os.Stat(jp); err != nil {
				t.Fatalf("expected JSON missing: %s — run `make testdata` to sync from xx.hocon (%v)", jp, err)
			}
			cfg, err := hocon.ParseFile(fp)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", name, err)
			}
			expectedData, err := os.ReadFile(jp)
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

// ── E8 amendment explicit value-position tests ───────────────────────────────

func TestE8_ValueStartHyphenNoDigit_LexesAsUnquoted(t *testing.T) {
	// RFC 8259 JSON-number requires a digit after `-`; bare `-foo` therefore
	// falls outside L270's disallow scope. Lightbend produces `{"a":"-foo"}`.
	cfg, err := hocon.ParseString("a = -foo")
	if err != nil {
		t.Fatalf("E8: `a = -foo` must lex+resolve as unquoted string, got: %v", err)
	}
	if got := cfg.GetString("a"); got != "-foo" {
		t.Errorf("E8: expected GetString(a)=\"-foo\", got %q", got)
	}
}

func TestE8_ValueStartHyphenAlone_LexesAsUnquoted(t *testing.T) {
	cfg, err := hocon.ParseString("a = -")
	if err != nil {
		t.Fatalf("E8: `a = -` must lex+resolve as unquoted string, got: %v", err)
	}
	if got := cfg.GetString("a"); got != "-" {
		t.Errorf("E8: expected GetString(a)=\"-\", got %q", got)
	}
}

func TestE8_ValueStartLeadingZero_NormalizesToNumber(t *testing.T) {
	// F3 BREAKING: `a = 01` was previously TokenInt{Value: "01"} (raw preserved
	// by readNumber); E8 normalizes via strconv.ParseInt → TokenInt{Value: "1"}.
	// JSON serialization is unchanged (Unmarshal already produced 1), but
	// GetString now returns "1" (was "01") — that's the breaking surface.
	cfg, err := hocon.ParseString("a = 01")
	if err != nil {
		t.Fatalf("E8: `a = 01` must parse, got: %v", err)
	}
	if got := cfg.GetString("a"); got != "1" {
		t.Errorf("E8: expected GetString(a)=\"1\" (normalized), got %q", got)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("E8: expected GetInt64(a)=1, got %d", got)
	}
}

func TestE8_ValueStartNegativeZero_NormalizesToZero(t *testing.T) {
	// `-0` parses as int 0; canonical form drops the sign (no negative zero in
	// integer arithmetic). Lightbend's parseLong does the same.
	cfg, err := hocon.ParseString("a = -0")
	if err != nil {
		t.Fatalf("E8: `a = -0` must parse, got: %v", err)
	}
	if got := cfg.GetString("a"); got != "0" {
		t.Errorf("E8: expected GetString(a)=\"0\" (canonicalized), got %q", got)
	}
}

func TestE8_KeyPosition_LeadingZeroPreserved(t *testing.T) {
	// Regression for Codex review finding on PR (not yet opened): E8's
	// leading-zero normalization is value-position only. `parseKey` reads
	// the same TokenInt.Value upstream, so the lexer must NOT canonicalize
	// — keys like `01 = x` must remain under key `"01"`, not silently
	// renamed to `"1"`. The fix moves normalization out of readNumber and
	// into parseSingleValue's TokenInt case.
	cfg, err := hocon.ParseString("01 = x")
	if err != nil {
		t.Fatalf("E8: `01 = x` must parse, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := got["01"]; !ok {
		t.Errorf("E8: expected key \"01\" (verbatim), got map: %v", got)
	}
}

func TestE8_KeyPosition_LeadingZeroWithTextPreserved(t *testing.T) {
	// Symmetric regression: numeric-key concat (TokenInt + TokenString → key
	// segment) must also preserve the verbatim TokenInt text.
	cfg, err := hocon.ParseString("01abc = x")
	if err != nil {
		t.Fatalf("E8: `01abc = x` must parse, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := got["01abc"]; !ok {
		t.Errorf("E8: expected key \"01abc\" (verbatim), got map: %v", got)
	}
}

func TestE8_KeyPosition_PlusReservation(t *testing.T) {
	// The bare-`+` dispatch reject also fires at key position (`+c = 1`),
	// which is a BREAKING change vs pre-E8 go.hocon. Documented in CHANGELOG.
	_, err := hocon.ParseString("+c = 1")
	if err == nil {
		t.Error("E8: `+c = 1` must reject (+ reservation per HOCON.md), parse succeeded")
	}
}

func TestE8_PlusReservation_ValueStart(t *testing.T) {
	// E8 closes the pre-existing go.hocon gap: bare `+` at value-start was
	// previously accepted as the start of an unquoted run (`+foo` → "+foo"),
	// but Lightbend rejects per the `+=` operator reservation. go.hocon now
	// matches by emitting TokenError from the bare-`+` dispatch branch.
	_, err := hocon.ParseString("a = +foo")
	if err == nil {
		t.Error("E8: `a = +foo` must reject (+ reservation per HOCON.md), parse succeeded")
	}
}

// ── E8 concat-continuation explicit tests ────────────────────────────────────

func TestE8_ConcatContinuation_SubstDashText(t *testing.T) {
	cfg, err := hocon.ParseString("a = foo\nb = ${a}-bar")
	if err != nil {
		t.Fatalf("E8 concat: ${a}-bar must lex+resolve, got: %v", err)
	}
	if got := cfg.GetString("b"); got != "foo-bar" {
		t.Errorf("E8 concat: expected b=\"foo-bar\", got %q", got)
	}
}

func TestE8_ConcatContinuation_QuotedDashText(t *testing.T) {
	cfg, err := hocon.ParseString(`b = "foo"-bar`)
	if err != nil {
		t.Fatalf("E8 concat: \"foo\"-bar must lex+resolve, got: %v", err)
	}
	if got := cfg.GetString("b"); got != "foo-bar" {
		t.Errorf("E8 concat: expected b=\"foo-bar\", got %q", got)
	}
}

func TestE8_ConcatContinuation_SubstDigitText(t *testing.T) {
	cfg, err := hocon.ParseString("a = foo\nb = ${a}1bar")
	if err != nil {
		t.Fatalf("E8 concat: ${a}1bar must lex+resolve, got: %v", err)
	}
	if got := cfg.GetString("b"); got != "foo1bar" {
		t.Errorf("E8 concat: expected b=\"foo1bar\", got %q", got)
	}
}

func TestE8_ConcatContinuation_SubstDotText(t *testing.T) {
	cfg, err := hocon.ParseString("a = foo\nb = ${a}.bar")
	if err != nil {
		t.Fatalf("E8 concat: ${a}.bar must lex+resolve, got: %v", err)
	}
	if got := cfg.GetString("b"); got != "foo.bar" {
		t.Errorf("E8 concat: expected b=\"foo.bar\", got %q", got)
	}
}

func TestE8_PlusReservation_ConcatContinuation(t *testing.T) {
	// Symmetric with value-start: bare `+` after a value-token must still
	// reject. Was previously accepted as the start of an unquoted continuation
	// (`${a}+bar` → "foo+bar"), now rejected via the same bare-`+` dispatch.
	_, err := hocon.ParseString("a = foo\nb = ${a}+bar")
	if err == nil {
		t.Error("E8: ${a}+bar must reject (+ reservation per HOCON.md), parse succeeded")
	}
}

// ── Out-of-E8-scope strict checks (unchanged) ────────────────────────────────
//
// The following rules apply to path-element composition (substitution body
// paths and dotted key segments), not to value-position unquoted strings.
// E8 amendment did not touch these — the strict rule is preserved.

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

// E13 (xx.hocon#42): S8.6 is NOT enforced on key path segments. The rule is
// value-position lexer-disambiguation; key paths are governed by path-element
// parsing rules. Lightbend accepts `a.-foo = 1` verbatim per the xx.hocon
// kh07 fixture. Inverted from the prior strict-reject test.
func TestE13_KeyPathHyphenSegment_Accepted(t *testing.T) {
	cfg, err := hocon.ParseString("a.-foo = 1")
	if err != nil {
		t.Fatalf("E13: a.-foo = 1 must parse (S8.6 not enforced on key paths), got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := got["a"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level 'a' → object, got %T", got["a"])
	}
	if v, ok2 := inner["-foo"]; !ok2 || !jsonEqual(v, 1) {
		t.Errorf("expected a.-foo = 1, got inner=%v", inner)
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

// E13: the concat-resplit path no longer enforces S8.6 on segments. Inverted
// from the prior strict-reject test (xx.hocon#42).
func TestE13_NumericKeyConcat_Resplit_HyphenSegment_Accepted(t *testing.T) {
	cfg, err := hocon.ParseString("123abc.-foo = 1")
	if err != nil {
		t.Fatalf("E13: 123abc.-foo = 1 must parse, got: %v", err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	inner, ok := got["123abc"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level '123abc' → object, got %T", got["123abc"])
	}
	if v, ok2 := inner["-foo"]; !ok2 || !jsonEqual(v, 1) {
		t.Errorf("expected 123abc.-foo = 1, got inner=%v", inner)
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
