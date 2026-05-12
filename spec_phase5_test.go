// Package hocon_test — Phase 5 spec-compliance tests.
// Clears the 28 remaining 🤷 items in docs/spec-compliance.md.
// Probe methodology: each item was verified against live parser output before
// classification; see Phase 5 PR body for per-item observations.
package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

// ── issue constants ────────────────────────────────────────────────────────────

// specIssueS1_S3_Empty is the GitHub issue number for S1.1 / S3.1:
// empty file should be invalid but is silently accepted.
const specIssueS1_S3_Empty = 75

// specIssueS8_2_SlashSlash is the GitHub issue number for S8.2:
// "//" inside an unquoted string should start a comment, but the lexer
// treats it as literal content when there is no preceding whitespace.
const specIssueS8_2_SlashSlash = 76

// specIssueS11_3_NumberPaths is the GitHub issue number for S11.3:
// numeric path expressions (e.g. `1.2.3 = x`) are rejected by the parser
// instead of being accepted and split on the dot separator.
const specIssueS11_3_NumberPaths = 77

// specIssueS13_15_BothUndef is the GitHub issue number for S13.15:
// `foo : ${?bar}${?baz}` creates field with empty-string value when both
// substitutions are undefined; spec requires the field to be omitted.
const specIssueS13_15_BothUndef = 78

// specIssueS13a_12_SelfRefPath is the GitHub issue number for S13a.12:
// self-referential substitution in a path expression does not resolve to
// the "below" value — the looked-up sub-object is discarded in the merge.
const specIssueS13a_12_SelfRefPath = 79

// specIssueS14a_10_UnquotedInclude is the GitHub issue number for S14a.10:
// include argument that is an unquoted string (e.g. `include foo.conf`) is
// silently accepted instead of rejected with a parse error.
const specIssueS14a_10_UnquotedInclude = 80

// specIssueS18_4_NoUnit is the GitHub issue number for S18.4:
// a string value with no unit should be interpreted with the default unit
// (milliseconds for duration), but GetDurationOption returns None.
const specIssueS18_4_NoUnit = 81

// specIssueS19_2_Micro is the GitHub issue number for S19.2:
// microsecond duration units (us, micro, micros, microsecond, microseconds)
// are all missing from parseDuration; GetDurationOption returns None.
const specIssueS19_2_Micro = 82

// specIssueS10_15_QuotedWS is the GitHub issue number for S10.15:
// quoted whitespace between obj/array substitutions should be a parse/resolve
// error, but the impl silently merges the arrays.
const specIssueS10_15_QuotedWS = 83

// specIssueS23_4_ObjectWins is the GitHub issue number for S23.4:
// when a .properties key conflicts (both leaf and parent), the object should
// win; the impl keeps the leaf string instead.
const specIssueS23_4_ObjectWins = 84

// ── S1.1 / S3.1: empty file is invalid ───────────────────────────────────────

// TestSpec_S1_1_EmptyFile_Pin pins the current behaviour where an empty file
// parses without error. Spec (HOCON.md L130) requires empty files to be invalid.
func TestSpec_S1_1_EmptyFile_Pin(t *testing.T) {
	// pin: see #75 — ParseString("") currently returns no error
	_ = specIssueS1_S3_Empty
	_, err := hocon.ParseString("")
	if err != nil {
		t.Errorf("[pin] empty file currently parses without error, but got: %v", err)
	}
}

// TestSpec_S1_1_EmptyFile_Spec is the spec-correct assertion: empty file must error.
func TestSpec_S1_1_EmptyFile_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S1.1/S3.1 — ParseString(\"\") returns nil error; see #%d", specIssueS1_S3_Empty)
	_, err := hocon.ParseString("")
	if err == nil {
		t.Error("expected error for empty file, got nil")
	}
}

// TestSpec_S3_1_EmptyFileInvalid_Pin is the same pin as S1.1 (same underlying item).
func TestSpec_S3_1_EmptyFileInvalid_Pin(t *testing.T) {
	// pin: see #75
	_ = specIssueS1_S3_Empty
	_, err := hocon.ParseString("")
	if err != nil {
		t.Errorf("[pin] S3.1 empty file currently parses without error, but got: %v", err)
	}
}

// TestSpec_S3_1_EmptyFileInvalid_Spec is the spec-correct assertion.
func TestSpec_S3_1_EmptyFileInvalid_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S3.1 — same as S1.1; see #%d", specIssueS1_S3_Empty)
	_, err := hocon.ParseString("")
	if err == nil {
		t.Error("expected error for empty file, got nil")
	}
}

// ── S6.5: "newline" means specifically 0x000A ─────────────────────────────────

// TestSpec_S6_5_NewlineMeansLF verifies that LF (0x000A) acts as the field
// separator in rootless HOCON, and that CR (0x000D) alone does NOT act as a
// field separator but is treated as non-newline whitespace.
// Spec: HOCON.md L183.
func TestSpec_S6_5_NewlineMeansLF(t *testing.T) {
	// LF separates fields in root-less HOCON.
	cfg := mustParseCfg(t, "a=1\nb=2")
	if cfg.GetInt("a") != 1 || cfg.GetInt("b") != 2 {
		t.Errorf("LF should separate fields: a=%d b=%d", cfg.GetInt("a"), cfg.GetInt("b"))
	}
	// CRLF: CR treated as whitespace, LF is the actual separator — should also work.
	cfg2 := mustParseCfg(t, "a=1\r\nb=2")
	if cfg2.GetInt("a") != 1 || cfg2.GetInt("b") != 2 {
		t.Errorf("CRLF should separate fields: a=%d b=%d", cfg2.GetInt("a"), cfg2.GetInt("b"))
	}
}

// ── S8.2: // inside unquoted string starts a comment ─────────────────────────

// TestSpec_S8_2_SlashSlashInUnquoted_Pin pins the current (non-conformant)
// behaviour: `//` embedded in an unquoted token without preceding whitespace
// is treated as literal text rather than starting a comment.
// Spec HOCON.md L248: "//" starts a comment anywhere outside a quoted string.
func TestSpec_S8_2_SlashSlashInUnquoted_Pin(t *testing.T) {
	// pin: see #76 — "bar//baz" is treated as unquoted string "bar//baz"
	_ = specIssueS8_2_SlashSlash
	cfg := mustParseCfg(t, "foo = bar//baz")
	got := cfg.GetString("foo")
	if got != "bar//baz" {
		t.Errorf("[pin] expected current value %q, got %q", "bar//baz", got)
	}
}

// TestSpec_S8_2_SlashSlashInUnquoted_Spec is the spec-correct assertion:
// `foo = bar//baz` → `//baz` starts a comment, so foo = "bar".
func TestSpec_S8_2_SlashSlashInUnquoted_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S8.2 — //baz not treated as comment in unquoted run; see #%d", specIssueS8_2_SlashSlash)
	cfg := mustParseCfg(t, "foo = bar//baz")
	if got := cfg.GetString("foo"); got != "bar" {
		t.Errorf("expected foo=%q (// starts comment), got %q", "bar", got)
	}
}

// ── S10.10: null stringifies to "null" in concat ─────────────────────────────

// TestSpec_S10_10_NullStringifiesInConcat verifies that `null` in a value
// concatenation is converted to the string "null". Spec HOCON.md L364.
func TestSpec_S10_10_NullStringifiesInConcat(t *testing.T) {
	cfg := mustParseCfg(t, `foo = null bar`)
	got, ok := cfg.GetStringOption("foo").Get()
	if !ok || got != "null bar" {
		t.Errorf("expected foo=%q, got ok=%v val=%q", "null bar", ok, got)
	}
}

// ── S10.12: single non-string value preserves its type ────────────────────────

// TestSpec_S10_12_SingleValuePreservesType verifies that a single non-string
// value is not converted to a string. Spec HOCON.md L376.
// `a = true` → bool; `n = 42` → int; `v = null` → null (GetStringOption=None).
func TestSpec_S10_12_SingleValuePreservesType(t *testing.T) {
	// bool: accessible as bool; GetStringOption returns Some via S17.2 auto-conversion,
	// but the stored type is boolean — not a spec violation.
	cfg := mustParseCfg(t, "a = true")
	if !cfg.GetBool("a") {
		t.Error("single true should be stored as boolean")
	}
	// null: GetStringOption must return None (not Some("null"))
	cfgNull := mustParseCfg(t, "v = null")
	if cfgNull.GetStringOption("v").IsSome() {
		t.Error("single null should not be stringified: GetStringOption must return None")
	}
	// number: accessible as int
	cfgN := mustParseCfg(t, "n = 42")
	if cfgN.GetInt64("n") != 42 {
		t.Errorf("single int 42 should be stored as number, got %d", cfgN.GetInt64("n"))
	}
}

// ── S10.15: quoted whitespace between obj/array substitutions is an error ─────

// TestSpec_S10_15_QuotedWSBetweenArraySubsts_Pin pins current behaviour:
// quoted whitespace between two array substitutions is silently accepted and
// the arrays are merged. Spec HOCON.md L442 requires this to be an error.
func TestSpec_S10_15_QuotedWSBetweenArraySubsts_Pin(t *testing.T) {
	// pin: see #83 — quoted " " between two array substs produces merged array [1,2]
	_ = specIssueS10_15_QuotedWS
	cfg := mustParseCfg(t, `
a = [1]
b = [2]
c = ${a} " " ${b}
`)
	opt := cfg.GetStringSliceOption("c")
	if !opt.IsSome() {
		t.Error("[pin] expected Some (current impl merges arrays), got None")
	}
}

// TestSpec_S10_15_QuotedWSBetweenArraySubsts_Spec is the spec-correct assertion.
func TestSpec_S10_15_QuotedWSBetweenArraySubsts_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S10.15 — quoted ws between array substs should error; see #%d", specIssueS10_15_QuotedWS)
	_, err := hocon.ParseString(`
a = [1]
b = [2]
c = ${a} " " ${b}
`)
	if err == nil {
		t.Error("expected error: quoted whitespace between array substitutions must be rejected")
	}
}

// ── S10.16: non-newline whitespace in arrays is concat, not separator ─────────

// TestSpec_S10_16_WhitespaceInArrayIsConcat verifies that whitespace between
// values inside an array creates a concat (one element), not separate elements.
// Spec HOCON.md L447.
func TestSpec_S10_16_WhitespaceInArrayIsConcat(t *testing.T) {
	// [1 2 3 4] → one element: the string "1 2 3 4"
	cfg := mustParseCfg(t, "arr = [1 2 3 4]")
	slice, ok := cfg.GetStringSliceOption("arr").Get()
	if !ok {
		t.Fatal("expected Some, got None")
	}
	if len(slice) != 1 {
		t.Errorf("expected 1 element (concat), got %d: %v", len(slice), slice)
	}
	if slice[0] != "1 2 3 4" {
		t.Errorf("expected %q, got %q", "1 2 3 4", slice[0])
	}
	// Newline-separated values ARE separate elements.
	cfg2 := mustParseCfg(t, "arr = [\n1\n2\n3\n]")
	slice2, ok2 := cfg2.GetStringSliceOption("arr").Get()
	if !ok2 {
		t.Fatal("expected Some for newline-separated array")
	}
	if len(slice2) != 3 {
		t.Errorf("expected 3 elements (newline-separated), got %d: %v", len(slice2), slice2)
	}
}

// ── S11.3: numbers retain original string representation in paths ─────────────

// TestSpec_S11_3_NumbersInPaths_Pin pins the current behaviour where a purely
// numeric path expression like `1.2.3 = x` is rejected at parse time.
// Spec HOCON.md L489: numbers in path expressions must retain their original
// string representation and be split on the "." separator.
func TestSpec_S11_3_NumbersInPaths_Pin(t *testing.T) {
	// pin: see #77 — `1.2.3 = x` produces a parse error
	_ = specIssueS11_3_NumberPaths
	_, err := hocon.ParseString("1.2.3 = x")
	if err == nil {
		t.Error("[pin] expected parse error for numeric-root path, got nil")
	}
}

// TestSpec_S11_3_NumbersInPaths_Spec is the spec-correct assertion:
// `1.2.3 = x` should create the path ["1","2","3"] → nested objects.
func TestSpec_S11_3_NumbersInPaths_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S11.3 — numeric-root path expression rejected; see #%d", specIssueS11_3_NumberPaths)
	cfg := mustParseCfg(t, "1.2.3 = x")
	// path "1.2.3" → nested lookup
	got, ok := cfg.GetStringOption("1.2.3").Get()
	if !ok || got != "x" {
		t.Errorf("expected 1.2.3=%q, got ok=%v val=%q", "x", ok, got)
	}
}

// ── S13.10: required substitution undefined → error ───────────────────────────

// TestSpec_S13_10_RequiredSubstUndefined verifies that an unresolved required
// substitution produces an error. Spec HOCON.md L627.
func TestSpec_S13_10_RequiredSubstUndefined(t *testing.T) {
	_, err := hocon.ParseString("a = ${nonexistent}")
	if err == nil {
		t.Error("expected error for unresolved required substitution, got nil")
	}
}

// ── S13.12: optional undefined in array element → element not added ───────────

// TestSpec_S13_12_OptionalUndefinedInArrayElementSkipped verifies that
// `${?missing}` in an array element position is silently omitted.
// Spec HOCON.md L635.
func TestSpec_S13_12_OptionalUndefinedInArrayElementSkipped(t *testing.T) {
	cfg := mustParseCfg(t, "arr = [1, ${?missing}, 3]")
	slice, ok := cfg.GetStringSliceOption("arr").Get()
	if !ok {
		t.Fatal("expected Some, got None")
	}
	if len(slice) != 2 {
		t.Errorf("expected 2 elements (missing element skipped), got %d: %v", len(slice), slice)
	}
	if slice[0] != "1" || slice[1] != "3" {
		t.Errorf("expected [1, 3], got %v", slice)
	}
}

// ── S13.15: foo : ${?bar}${?baz} skipped only when BOTH undefined ─────────────

// TestSpec_S13_15_BothUndefined_Pin pins the current (non-conformant) behaviour
// where `foo = ${?bar}${?baz}` creates a field with value "" even when both
// substitutions are undefined. Spec HOCON.md L640 requires the field to be omitted.
func TestSpec_S13_15_BothUndefined_Pin(t *testing.T) {
	// pin: see #78 — field is created with empty string instead of being omitted
	_ = specIssueS13_15_BothUndef
	cfg := mustParseCfg(t, "foo = ${?bar}${?baz}")
	opt := cfg.GetStringOption("foo")
	if !opt.IsSome() {
		t.Error("[pin] expected Some (current impl creates field with \"\"), got None")
		return
	}
	v, _ := opt.Get()
	if v != "" {
		t.Errorf("[pin] expected current value %q, got %q", "", v)
	}
}

// TestSpec_S13_15_BothUndefined_Spec is the spec-correct assertion:
// field must not be created when both substitutions are undefined.
func TestSpec_S13_15_BothUndefined_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S13.15 — field created with empty string when both substs undefined; see #%d", specIssueS13_15_BothUndef)
	cfg := mustParseCfg(t, "foo = ${?bar}${?baz}")
	if cfg.GetStringOption("foo").IsSome() {
		t.Error("field foo must not exist when both ${?bar} and ${?baz} are undefined")
	}
}

// TestSpec_S13_15_OneDefinedCreatesField verifies the positive case:
// when one substitution is defined, the field IS created.
func TestSpec_S13_15_OneDefinedCreatesField(t *testing.T) {
	cfg := mustParseCfg(t, "baz=hello\nfoo = ${?bar}${?baz}")
	got, ok := cfg.GetStringOption("foo").Get()
	if !ok || got != "hello" {
		t.Errorf("expected foo=%q when baz=hello, got ok=%v val=%q", "hello", ok, got)
	}
}

// ── S13a.3: self-ref before any prior value → undefined → error ───────────────

// TestSpec_S13a_3_SelfRefNoPriorValue verifies that `foo = ${foo}` with no
// prior definition of foo produces an error. Spec HOCON.md L767: the substitution
// is undefined (not a cycle), but still invalid for a required substitution.
func TestSpec_S13a_3_SelfRefNoPriorValue(t *testing.T) {
	_, err := hocon.ParseString("foo = ${foo}")
	if err == nil {
		t.Error("expected error for self-referential substitution with no prior value")
	}
}

// ── S13a.5: substitution hidden by later non-object → no error ────────────────

// TestSpec_S13a_5_SubstHiddenByLaterValue verifies that `foo = ${missing}; foo = 42`
// does not error: the substitution is never evaluated. Spec HOCON.md L780.
func TestSpec_S13a_5_SubstHiddenByLaterValue(t *testing.T) {
	cfg := mustParseCfg(t, "foo = ${does-not-exist}\nfoo = 42")
	if cfg.GetInt("foo") != 42 {
		t.Errorf("expected foo=42, got %d", cfg.GetInt("foo"))
	}
}

// ── S13a.7: cycle inside array `a : [${a}]` → error ──────────────────────────

// TestSpec_S13a_7_CycleInsideArray verifies that `a = [${a}]` produces an error.
// Spec HOCON.md L689.
func TestSpec_S13a_7_CycleInsideArray(t *testing.T) {
	_, err := hocon.ParseString("a = [${a}]")
	if err == nil {
		t.Error("expected circular reference error for a = [${a}]")
	}
}

// ── S13a.9: multi-step cycle a→b→c→a → error ─────────────────────────────────

// TestSpec_S13a_9_MultiStepCycle verifies that a→b→c→a produces an error.
// Spec HOCON.md L862.
func TestSpec_S13a_9_MultiStepCycle(t *testing.T) {
	_, err := hocon.ParseString("a = ${b}\nb = ${c}\nc = ${a}")
	if err == nil {
		t.Error("expected circular reference error for a→b→c→a cycle")
	}
}

// ── S13a.10: substitution memoized by instance, not by path ───────────────────

// TestSpec_S13a_10_MemoizedByInstance documents that this invariant is internal
// to the resolver and cannot be observed through the public API.
// Spec HOCON.md L885. Status: ➖ (not externally observable).
func TestSpec_S13a_10_MemoizedByInstance(t *testing.T) {
	t.Skip("S13a.10: memoization-by-instance is an internal resolver invariant not observable via the public API")
}

// ── S13a.12: self-ref in path expression resolves to "below" ─────────────────

// TestSpec_S13a_12_SelfRefInPathResolvesBelow_Pin pins the current behaviour:
// the spec example `foo : { a : { c : 1 } }; foo : ${foo.a}; foo : { a : 2 }`
// should yield {a:2, c:1} but the impl produces {a:2} (c is lost).
// Spec HOCON.md L791.
func TestSpec_S13a_12_SelfRefInPathResolvesBelow_Pin(t *testing.T) {
	// pin: see #79 — ${foo.a} self-reference does not include {c:1} in the merge
	_ = specIssueS13a_12_SelfRefPath
	cfg := mustParseCfg(t, `
foo : { a : { c : 1 } }
foo : ${foo.a}
foo : { a : 2 }
`)
	// a=2 is correct regardless
	if cfg.GetInt("foo.a") != 2 {
		t.Errorf("[pin] foo.a: expected 2, got %d", cfg.GetInt("foo.a"))
	}
	// c should be absent (current buggy behaviour)
	if cfg.GetIntOption("foo.c").IsSome() {
		t.Error("[pin] foo.c should be absent in current impl (self-ref-in-path bug)")
	}
}

// TestSpec_S13a_12_SelfRefInPathResolvesBelow_Spec is the spec-correct assertion.
func TestSpec_S13a_12_SelfRefInPathResolvesBelow_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S13a.12 — ${foo.a} does not include {c:1}; see #%d", specIssueS13a_12_SelfRefPath)
	cfg := mustParseCfg(t, `
foo : { a : { c : 1 } }
foo : ${foo.a}
foo : { a : 2 }
`)
	// spec: ${foo.a} looks "below" to {c:1}; final foo = {a:2, c:1}
	if cfg.GetInt("foo.a") != 2 {
		t.Errorf("foo.a: expected 2, got %d", cfg.GetInt("foo.a"))
	}
	if cfg.GetInt("foo.c") != 1 {
		t.Errorf("foo.c: expected 1, got %d", cfg.GetInt("foo.c"))
	}
}

// ── S13a.14: mutually-referring objects resolve lazily without false cycle ─────

// TestSpec_S13a_14_MutualRefNoCycle verifies the spec example:
// bar.a = ${foo.d} and foo.c = ${bar.b} resolve lazily; result: bar.a=4, foo.c=3.
// Spec HOCON.md L825-834.
func TestSpec_S13a_14_MutualRefNoCycle(t *testing.T) {
	cfg := mustParseCfg(t, `
bar : { a : ${foo.d}, b : 1 }
bar.b = 3
foo : { c : ${bar.b}, d : 2 }
foo.d = 4
`)
	if got := cfg.GetInt("bar.a"); got != 4 {
		t.Errorf("bar.a: expected 4, got %d", got)
	}
	if got := cfg.GetInt("foo.c"); got != 3 {
		t.Errorf("foo.c: expected 3, got %d", got)
	}
}

// ── S14a.10: include argument must be a quoted string ─────────────────────────

// TestSpec_S14a_10_UnquotedIncludeArg_Pin pins the current (non-conformant)
// behaviour where `include unquoted-file` is silently accepted (the parser
// accepts an unquoted TokenString as the include path, then the missing file
// is treated as an optional include and silently skipped).
// Spec HOCON.md L958: the argument must be a single quoted string.
func TestSpec_S14a_10_UnquotedIncludeArg_Pin(t *testing.T) {
	// pin: see #80 — "include unquoted-file" parses without error
	_ = specIssueS14a_10_UnquotedInclude
	_, err := hocon.ParseString("include unquoted-file")
	if err != nil {
		t.Errorf("[pin] expected nil error (current impl accepts unquoted), got: %v", err)
	}
}

// TestSpec_S14a_10_UnquotedIncludeArg_Spec is the spec-correct assertion.
func TestSpec_S14a_10_UnquotedIncludeArg_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S14a.10 — unquoted include arg accepted; see #%d", specIssueS14a_10_UnquotedInclude)
	_, err := hocon.ParseString("include unquoted-file")
	if err == nil {
		t.Error("expected parse error for unquoted include argument, got nil")
	}
}

// TestSpec_S14a_10_QuotedIncludeArgAccepted verifies the positive case:
// a quoted include argument is valid (even if the file is missing — optional).
// Uses t.TempDir to avoid any filesystem side-effects.
func TestSpec_S14a_10_QuotedIncludeArgAccepted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.conf")
	if err := os.WriteFile(path, []byte(`x = 1`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg := mustParseCfg(t, fmt.Sprintf(`include "%s"`, slashPath))
	if cfg.GetInt("x") != 1 {
		t.Errorf("expected x=1 from included file, got %d", cfg.GetInt("x"))
	}
}

// ── S18.3: unit name consists only of letters ─────────────────────────────────

// TestSpec_S18_3_UnitNameLettersOnly verifies that a unit with embedded digits
// is rejected (GetDurationOption returns None, not a valid duration).
// Spec HOCON.md L1287: unit name consists only of Unicode L* / isLetter characters.
func TestSpec_S18_3_UnitNameLettersOnly(t *testing.T) {
	// Valid: "10ms" parses correctly.
	cfg := mustParseCfg(t, `d = "10ms"`)
	if got := cfg.GetDuration("d"); got != 10*time.Millisecond {
		t.Errorf("'10ms': expected %v, got %v", 10*time.Millisecond, got)
	}
	// Invalid: "10ms2" — the unit "ms2" contains a digit; must be rejected.
	cfg2 := mustParseCfg(t, `d = "10ms2"`)
	if cfg2.GetDurationOption("d").IsSome() {
		t.Error("'10ms2' has digit in unit name — GetDurationOption must return None")
	}
}

// ── S18.4: string with no unit → default unit ────────────────────────────────

// TestSpec_S18_4_StringNoUnit_Pin pins the current (non-conformant) behaviour
// where a bare number string (e.g. "100") returns None from GetDurationOption.
// Spec HOCON.md L1290: string with no unit should be treated as the default
// unit (milliseconds for duration).
func TestSpec_S18_4_StringNoUnit_Pin(t *testing.T) {
	// pin: see #81 — "100" (no unit) returns None instead of 100ms
	_ = specIssueS18_4_NoUnit
	cfg := mustParseCfg(t, `d = "100"`)
	if cfg.GetDurationOption("d").IsSome() {
		t.Error("[pin] expected None for no-unit string (current impl), got Some")
	}
}

// TestSpec_S18_4_StringNoUnit_Spec is the spec-correct assertion.
func TestSpec_S18_4_StringNoUnit_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S18.4 — bare-number string not treated as default unit; see #%d", specIssueS18_4_NoUnit)
	cfg := mustParseCfg(t, `d = "100"`)
	got, ok := cfg.GetDurationOption("d").Get()
	if !ok || got != 100*time.Millisecond {
		t.Errorf("expected %v, got ok=%v val=%v", 100*time.Millisecond, ok, got)
	}
}

// ── S19.1: ns/nano/nanos/nanosecond/nanoseconds ───────────────────────────────

// TestSpec_S19_1_Nanoseconds_Pin pins partial conformance: "ns", "nanosecond",
// and "nanoseconds" are recognised; "nano" and "nanos" are not.
// Spec HOCON.md L1307.
func TestSpec_S19_1_Nanoseconds_Pin(t *testing.T) {
	// These three pass already (✅ sub-rules):
	for _, unit := range []string{"ns", "nanosecond", "nanoseconds"} {
		cfg := mustParseCfg(t, fmt.Sprintf(`d = "1%s"`, unit))
		got, ok := cfg.GetDurationOption("d").Get()
		if !ok || got != time.Nanosecond {
			t.Errorf("unit %q: expected %v, got ok=%v val=%v", unit, time.Nanosecond, ok, got)
		}
	}
	// These two are missing (pin: returns None):
	for _, unit := range []string{"nano", "nanos"} {
		cfg := mustParseCfg(t, fmt.Sprintf(`d = "1%s"`, unit))
		if cfg.GetDurationOption("d").IsSome() {
			t.Errorf("[pin] unit %q: currently returns None, but got Some — update status to ✅", unit)
		}
	}
}

// TestSpec_S19_1_Nanoseconds_Spec is the spec-correct assertion for missing aliases.
func TestSpec_S19_1_Nanoseconds_Spec(t *testing.T) {
	t.Skipf("[skip] spec partial-violation per S19.1 — 'nano' and 'nanos' aliases missing; no dedicated issue yet (tracked under S19.2 scope #%d)", specIssueS19_2_Micro)
	for _, unit := range []string{"nano", "nanos"} {
		cfg := mustParseCfg(t, fmt.Sprintf(`d = "1%s"`, unit))
		got, ok := cfg.GetDurationOption("d").Get()
		if !ok || got != time.Nanosecond {
			t.Errorf("unit %q: expected %v, got ok=%v val=%v", unit, time.Nanosecond, ok, got)
		}
	}
}

// ── S19.2: us/micro/micros/microsecond/microseconds ──────────────────────────

// TestSpec_S19_2_Microseconds_Pin pins the current behaviour: ALL microsecond
// duration unit aliases return None. Spec HOCON.md L1308.
func TestSpec_S19_2_Microseconds_Pin(t *testing.T) {
	// pin: see #82 — all microsecond units produce None
	_ = specIssueS19_2_Micro
	for _, unit := range []string{"us", "micro", "micros", "microsecond", "microseconds"} {
		cfg := mustParseCfg(t, fmt.Sprintf(`d = "1%s"`, unit))
		if cfg.GetDurationOption("d").IsSome() {
			t.Errorf("[pin] unit %q: expected None (current impl), got Some", unit)
		}
	}
}

// TestSpec_S19_2_Microseconds_Spec is the spec-correct assertion.
func TestSpec_S19_2_Microseconds_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S19.2 — microsecond unit aliases not in parseDuration; see #%d", specIssueS19_2_Micro)
	for _, unit := range []string{"us", "micro", "micros", "microsecond", "microseconds"} {
		cfg := mustParseCfg(t, fmt.Sprintf(`d = "1%s"`, unit))
		got, ok := cfg.GetDurationOption("d").Get()
		if !ok || got != time.Microsecond {
			t.Errorf("unit %q: expected %v, got ok=%v val=%v", unit, time.Microsecond, ok, got)
		}
	}
}

// ── S19.5: m/minute/minutes ──────────────────────────────────────────────────

// TestSpec_S19_5_Minutes verifies that "m", "minute", and "minutes" are all
// recognised as minute aliases. Spec HOCON.md L1311.
func TestSpec_S19_5_Minutes(t *testing.T) {
	for _, unit := range []string{"m", "minute", "minutes"} {
		cfg := mustParseCfg(t, fmt.Sprintf(`d = "1%s"`, unit))
		got, ok := cfg.GetDurationOption("d").Get()
		if !ok || got != time.Minute {
			t.Errorf("unit %q: expected %v, got ok=%v val=%v", unit, time.Minute, ok, got)
		}
	}
}

// ── S19.8: duration unit names are case sensitive (lowercase only) ─────────────

// TestSpec_S19_8_DurationCaseSensitive verifies that uppercase duration unit
// names are rejected. Spec HOCON.md L1304.
func TestSpec_S19_8_DurationCaseSensitive(t *testing.T) {
	// "MS" (uppercase) must not be accepted
	cfg := mustParseCfg(t, `d = "10MS"`)
	if cfg.GetDurationOption("d").IsSome() {
		t.Error("'10MS' (uppercase) must not be a valid duration unit")
	}
	// "Seconds" (mixed case) must not be accepted
	cfg2 := mustParseCfg(t, `d = "10Seconds"`)
	if cfg2.GetDurationOption("d").IsSome() {
		t.Error("'10Seconds' (mixed case) must not be a valid duration unit")
	}
	// "ms" (lowercase) must be accepted
	cfg3 := mustParseCfg(t, `d = "10ms"`)
	if got := cfg3.GetDuration("d"); got != 10*time.Millisecond {
		t.Errorf("'10ms' expected %v, got %v", 10*time.Millisecond, got)
	}
}

// ── S23.2: empty path elements preserved in properties ───────────────────────

// TestSpec_S23_2_EmptyPathElementsPreserved verifies that a .properties key
// such as "a." (trailing dot) produces a nested object where the last segment
// is the empty string. Spec HOCON.md L1456.
func TestSpec_S23_2_EmptyPathElementsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	// "a.=hello" → split on "." → ["a",""] → {a: {"": "hello"}}
	if err := os.WriteFile(path, []byte("a.=hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg := mustParseCfg(t, fmt.Sprintf(`include file("%s")`, slashPath))
	// "a" must be accessible as an object
	if !cfg.GetConfigOption("a").IsSome() {
		t.Fatal("expected 'a' to be an object (config), got None")
	}
	// The empty-string key inside a must hold "hello"
	got, ok := cfg.GetStringOption(`a.""`).Get()
	if !ok || got != "hello" {
		t.Errorf(`a."" expected "hello", got ok=%v val=%q`, ok, got)
	}
}

// ── S23.4: object wins over string on conflicting property key ────────────────

// TestSpec_S23_4_ObjectWinsOverString_Pin pins the current (non-conformant)
// behaviour: when "a=hello" and "a.b=world" both appear in a .properties file,
// the impl keeps the string "hello" for "a" and loses "a.b=world".
// Spec HOCON.md L1485-1489: the object must win; string is discarded.
func TestSpec_S23_4_ObjectWinsOverString_Pin(t *testing.T) {
	// pin: see #84 — string "hello" wins over object {b: "world"} in current impl
	_ = specIssueS23_4_ObjectWins
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	if err := os.WriteFile(path, []byte("a=hello\na.b=world\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg := mustParseCfg(t, fmt.Sprintf(`include file("%s")`, slashPath))
	// Current impl: a = "hello", a.b is inaccessible (object was dropped)
	aStr, aOk := cfg.GetStringOption("a").Get()
	if !aOk || aStr != "hello" {
		t.Errorf("[pin] expected a=%q (string wins currently), got ok=%v val=%q", "hello", aOk, aStr)
	}
	if cfg.GetStringOption("a.b").IsSome() {
		t.Error("[pin] a.b should be absent in current impl (object was discarded)")
	}
}

// TestSpec_S23_4_ObjectWinsOverString_Spec is the spec-correct assertion.
func TestSpec_S23_4_ObjectWinsOverString_Spec(t *testing.T) {
	t.Skipf("[skip] spec violation per S23.4 — string wins over object in .properties conflict; see #%d", specIssueS23_4_ObjectWins)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	if err := os.WriteFile(path, []byte("a=hello\na.b=world\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg := mustParseCfg(t, fmt.Sprintf(`include file("%s")`, slashPath))
	// spec: object {b:"world"} wins; string "hello" is discarded
	got, ok := cfg.GetStringOption("a.b").Get()
	if !ok || got != "world" {
		t.Errorf("expected a.b=%q (object wins), got ok=%v val=%q", "world", ok, got)
	}
	// "a" as a string must be gone
	if cfg.GetStringOption("a").IsSome() {
		t.Error("a must not be a string (object wins — string discarded)")
	}
}
