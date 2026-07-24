package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon"
)

// Issue #158: a value concatenation consisting solely of undefined optional
// substitutions SEPARATED BY WHITESPACE must materialize the separator
// whitespace as a string, matching the reference implementation
// (`cf = ${?vv} ${?vv}` → "cf": " ").
//
// HOCON.md §Substitutions: an undefined ${?foo} "should become an empty
// string" when part of a value concatenation with another string — the
// inter-token whitespace is that other string. The spec's field-drop example
// (`foo : ${?bar}${?baz}`) has no whitespace between the substitutions and
// keeps dropping the field (covered below as a guard).

func TestIssue158_WhitespaceSeparatedUndefinedOptionalsKeepField(t *testing.T) {
	cfg, err := hocon.ParseString("cf = ${?vv} ${?vv}\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Has("cf") {
		t.Fatalf(`cf dropped; want cf = " " (reference behaviour)`)
	}
	if got := cfg.GetString("cf"); got != " " {
		t.Errorf("cf = %q, want %q", got, " ")
	}
}

func TestIssue158_ThreeUndefinedOptionalsTwoSeparators(t *testing.T) {
	cfg, err := hocon.ParseString("cf = ${?a} ${?b} ${?c}\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("cf"); got != "  " {
		t.Errorf("cf = %q, want two spaces", got)
	}
}

// Guard: the spec's own example — adjacent undefined optionals with NO
// whitespace still drop the field (all five parsers agree here).
func TestIssue158_AdjacentUndefinedOptionalsStillDropField(t *testing.T) {
	cfg, err := hocon.ParseString("x = ${?a}${?b}\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Has("x") {
		t.Errorf("x = %q; want field dropped (HOCON.md §Substitutions example)", cfg.GetString("x"))
	}
}

// Array-element context takes the same concat path: pre-fix `[${?a} ${?b}]`
// resolved to []; the materialized separator now yields [" "].
func TestIssue158_ArrayElementContext(t *testing.T) {
	cfg, err := hocon.ParseString("arr = [${?a} ${?b}]\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := cfg.GetStringSlice("arr")
	if len(got) != 1 || got[0] != " " {
		t.Errorf("arr = %q, want [\" \"]", got)
	}
}

// Boundary between drop and materialize: an adjacent undefined pair plus a
// whitespace-separated third still materializes the one separator.
func TestIssue158_AdjacentPairPlusSeparatedThird(t *testing.T) {
	cfg, err := hocon.ParseString("cf = ${?a}${?b} ${?c}\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("cf"); got != " " {
		t.Errorf("cf = %q, want %q", got, " ")
	}
}

// Guard: partial-vanish concat keeps working (leading separator preserved).
func TestIssue158_PartialVanishKeepsSeparator(t *testing.T) {
	cfg, err := hocon.ParseString("y = ${?vv} 1\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("y"); got != " 1" {
		t.Errorf("y = %q, want %q", got, " 1")
	}
}
