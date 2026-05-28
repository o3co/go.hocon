// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon"
)

// Cross-impl regression tests for go.hocon#132 (S10.5 inner whitespace in
// value concatenation) and go.hocon#133 (S10.11 numbers stringify "as written
// in the source file"). Both were collapsed/canonicalized pre-fix:
//
//   #132 — parseValue inserted a single hardcoded " " separator, collapsing
//          every multi-space run to one space.
//   #133 — parseSingleValue canonicalized integer lexemes via strconv.FormatInt
//          (`05` → "5", `00_example` → "0_example"), losing the source spelling
//          when the value is stringified in a concat.

func TestS10_5_ValueConcatWhitespace(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unquoted multi-space", "a = foo   bar\n", "foo   bar"},
		{"quoted multi-space", `a = "foo"   "bar"` + "\n", "foo   bar"},
		{"single space unchanged", "a = foo bar\n", "foo bar"},
		{"defined subst multi-space", "x = mid\na = \"left\"  ${x}  \"right\"\n", "left  mid  right"},
		{"tab run preserved", "a = foo \t bar\n", "foo \t bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := hocon.ParseString(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := cfg.GetString("a"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestS10_5_UndefinedOptionalKeepsBothRuns pins the go.hocon#132 canonical
// repro: env unset → the optional substitution contributes nothing, but the
// 2 + 2 whitespace runs around it must remain (→ 4 spaces).
func TestS10_5_UndefinedOptionalKeepsBothRuns(t *testing.T) {
	// Deferred parse + hermetic resolve (UseSystemEnvironment=false) so the
	// optional ${?GO132_UNSET} is undefined regardless of the host env.
	cfg, err := hocon.ParseStringWithOptions(
		"a = \"left\"  ${?GO132_UNSET}  \"right\"\n",
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != "left    right" {
		t.Errorf("got %q, want %q", got, "left    right")
	}
}

func TestS10_11_NumericLexemePreserved(t *testing.T) {
	// version concat keeps the leading zero of `minor = 05`.
	cfg, err := hocon.ParseString("major = 26\nminor = 05\nversion = ${major}.${minor}\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("version"); got != "26.05" {
		t.Errorf("version: got %q, want %q", got, "26.05")
	}
	// standalone numeric reads semantically (05 → 5).
	if got := cfg.GetInt64("minor"); got != 5 {
		t.Errorf("minor GetInt64: got %d, want 5", got)
	}
}

// TestS10_11_NumericPrefixUnquotedKeepsLexeme pins the `00_example` case from
// go.hocon#133: the numeric prefix is lexed as TokenInt(`00`) then concatenated
// with the unquoted `_example`; the canonicalization dropped the leading zero
// (`0_example`). With the lexeme preserved it stays `00_example`.
func TestS10_11_NumericPrefixUnquotedKeepsLexeme(t *testing.T) {
	cfg, err := hocon.ParseString("name = 00_example\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("name"); got != "00_example" {
		t.Errorf("name: got %q, want %q", got, "00_example")
	}
}
