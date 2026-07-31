// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package env_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/env"
)

func wantString(t *testing.T, cfg *hocon.Config, path, want string) {
	t.Helper()
	got, ok := cfg.GetStringOption(path).Get()
	if !ok {
		t.Fatalf("path %q missing", path)
	}
	if got != want {
		t.Errorf("path %q = %q, want %q", path, got, want)
	}
}

// F1.2/F1.3: "__" is the path separator, a single "_" stays in the segment,
// and segments are lowercased.
// F1.6 message format, shared with py.hocon and rs.hocon: the mapped path is
// rendered as a HOCON path expression, so a segment holding a literal dot is
// quoted and cannot be mistaken for two segments.
// F1.9(b): a bulk mount is an explicit request for a whole namespace, so an
// entry in it that cannot be decoded is an error. Omitting it silently would
// leave a subtree that looks complete while the operator's setting is missing,
// and a stale default would then win invisibly; admitting the raw bytes is
// worse still, since the key becomes unreachable text.
func TestUndecodableNameInMountIsError(t *testing.T) {
	_, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{"APP_\xffZ=1"}})
	if err == nil {
		t.Fatal("undecodable variable name accepted, want error")
	}
	if !strings.Contains(err.Error(), "F1.9") {
		t.Errorf("error %q does not cite the spec item F1.9", err)
	}
	if !strings.Contains(err.Error(), `\xff`) {
		t.Errorf("error %q does not show the offending name in escaped form", err)
	}
}

func TestUndecodableValueInMountIsError(t *testing.T) {
	_, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{"APP_X=\xff\xfe"}})
	if err == nil {
		t.Fatal("undecodable value accepted, want error")
	}
	if !strings.Contains(err.Error(), "APP_X") {
		t.Errorf("error %q does not name the variable", err)
	}
	if !strings.Contains(err.Error(), "F1.9") {
		t.Errorf("error %q does not cite the spec item F1.9", err)
	}
	// Environment values are where credentials live: the message must not
	// echo one, decodable or not.
	if strings.Contains(err.Error(), "\xff") || strings.Contains(err.Error(), `\xff`) {
		t.Errorf("error %q echoes the value", err)
	}
}

// The prefix filter bounds the rule. An undecodable variable the caller never
// asked for must not break an unrelated mount — that is what keeps this from
// becoming the abort-on-anything bug F1.9 exists to avoid.
func TestUndecodableEntryOutsideThePrefixIsIgnored(t *testing.T) {
	cfg, err := env.Load(env.Options{
		Prefix:  "APP_",
		Environ: []string{"OTHER_\xffZ=junk", "SOMETHING=\xfe", "APP_A=1"},
	})
	if err != nil {
		t.Fatalf("Load: %v — an entry outside the prefix is none of the mount's business", err)
	}
	wantString(t, cfg, "a", "1")
}

// F1.9(c): an entry whose value does not decode still occupies its mapped
// path, so a second name mapping to the same path is still a conflict. Both
// problems are errors here, so the mount fails either way — what must not
// happen is the undecodable entry being dropped and the other value quietly
// mounting as if it were unopposed.
func TestUndecodableValueStillOccupiesItsPath(t *testing.T) {
	_, err := env.Load(env.Options{
		Prefix:  "APP_",
		Environ: []string{"APP_A__B=\xff", "APP_a__b=ok"},
	})
	if err == nil {
		t.Fatal("mount succeeded, want error — one value would have won silently")
	}
	if !strings.Contains(err.Error(), "APP_A__B") {
		t.Errorf("error %q does not name the undecodable entry", err)
	}
}

// A .env file is validated as a whole (its bytes are one document), so this
// path was already covered; pinned so the two stay consistent.
func TestUndecodableDotEnvIsError(t *testing.T) {
	_, err := env.Parse([]byte("A=\xff\n"), env.Options{})
	if err == nil {
		t.Fatal("undecodable .env accepted, want error")
	}
}

func TestCollisionMessageRendersPathAsExpression(t *testing.T) {
	for name, tc := range map[string]struct {
		environ []string
		want    string
	}{
		"double underscore": {
			[]string{"APP_A__B=1", "APP_a__b=2"},
			"APP_A__B and APP_a__b both map to a.b",
		},
		"literal dot": {
			[]string{"APP_FOO.BAR=1", "APP_foo.bar=2"},
			`APP_FOO.BAR and APP_foo.bar both map to "foo.bar"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := env.Load(env.Options{Prefix: "APP_", Environ: tc.environ})
			if err == nil {
				t.Fatal("collision accepted, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// A NUL in a variable name must not fake a path boundary. The collision index
// used to join segments with NUL, so APP_A<NUL>B (one segment) and APP_A__B
// (two) hashed alike and reported a collision that does not exist. Reachable
// through Options.Environ.
func TestNULInNameIsNotAPathSeparator(t *testing.T) {
	cfg, err := env.Load(env.Options{
		Prefix:  "APP_",
		Environ: []string{"APP_A\x00B=one", "APP_A__B=two"},
	})
	if err != nil {
		t.Fatalf("Load: %v — these are different paths, not a collision", err)
	}
	if got := cfg.GetString("a.b"); got != "two" {
		t.Errorf("a.b = %q, want %q", got, "two")
	}
	if got := cfg.GetString("\"a\x00b\""); got != "one" {
		t.Errorf("a<NUL>b = %q, want %q", got, "one")
	}
}

// F1.3: lowercasing is ASCII-only. Go's strings.ToLower applies simple case
// mapping and turns İ (U+0130) into "i", which would collide with I; Python,
// JS and Rust produce "i"+U+0307 and keep the two apart. Folding only A-Z
// makes every implementation agree.
func TestLowercasingIsASCIIOnly(t *testing.T) {
	cfg, err := env.Load(env.Options{
		Prefix:  "APP_",
		Environ: []string{"APP_\u0130=dotted", "APP_I=plain"},
	})
	if err != nil {
		t.Fatalf("Load: %v — U+0130 and I are different keys under ASCII folding", err)
	}
	if got := cfg.GetString("i"); got != "plain" {
		t.Errorf("i = %q, want %q", got, "plain")
	}
	if got := cfg.GetString("\u0130"); got != "dotted" {
		t.Errorf("U+0130 = %q, want %q", got, "dotted")
	}
}

func TestLoadNestsAndLowercases(t *testing.T) {
	cfg, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{
		"APP_DB__HOST=db.internal",
		"APP_DB__MAX_CONN=10",
		"APP_NAME=svc",
		"PATH=/usr/bin",
		"OTHER_DB__HOST=nope",
	}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantString(t, cfg, "db.host", "db.internal")
	wantString(t, cfg, "db.max_conn", "10")
	wantString(t, cfg, "name", "svc")

	for _, absent := range []string{"path", "PATH", "other_db.host"} {
		if cfg.GetStringOption(absent).IsSome() {
			t.Errorf("path %q was mounted despite the prefix filter", absent)
		}
	}
}

// F1.1: mounting the whole environment would pull in unrelated secrets.
func TestLoadRequiresPrefix(t *testing.T) {
	_, err := env.Load(env.Options{Environ: []string{"A=1"}})
	if err == nil {
		t.Fatal("Load without a prefix succeeded, want error")
	}
	if !strings.Contains(err.Error(), "F1.1") {
		t.Errorf("error %q does not cite the spec item F1.1", err)
	}
}

// F1.6: two variables can map to one path and there is no meaningful order to
// break the tie, so it is an error rather than a silent pick.
func TestLoadCollisionIsError(t *testing.T) {
	_, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{
		"APP_A__B=1",
		"APP_a__b=2",
	}})
	if err == nil {
		t.Fatal("colliding variables accepted, want error")
	}
	for _, want := range []string{"APP_A__B", "APP_a__b", "a.b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoadEmptySegmentIsError(t *testing.T) {
	for _, name := range []string{"APP_A____B=1", "APP___A=1", "APP_A__=1", "APP_=1"} {
		if _, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{name}}); err == nil {
			t.Errorf("Load(%q) succeeded, want error", name)
		}
	}
}

// F1.4/F0.2: values are strings and ${...} in them is data, not a reference.
func TestLoadValuesAreLiteralStrings(t *testing.T) {
	cfg, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{"APP_A=${foo.bar}"}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantString(t, cfg, "a", "${foo.bar}")
	resolved, err := cfg.Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantString(t, resolved, "a", "${foo.bar}")
}

func TestLoadValueContainingEquals(t *testing.T) {
	cfg, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{"APP_A=k=v"}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantString(t, cfg, "a", "k=v")
}

func parseDotEnv(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := env.Parse([]byte(src), env.Options{})
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return cfg
}

func TestDotEnvDialect(t *testing.T) {
	cfg := parseDotEnv(t, strings.Join([]string{
		"# a comment",
		"",
		"export FOO=bar",
		"DB__HOST=db.internal",
		`QUOTED="a\nb"`,
		`SINGLE='raw ${x} #hash'`,
		"HASH=#fff",
		"INNER=a#b",
		"SPACED=  trimmed  ",
	}, "\n"))

	for path, want := range map[string]string{
		"foo":     "bar",
		"db.host": "db.internal",
		"quoted":  "a\nb",
		"single":  "raw ${x} #hash",
		"hash":    "#fff",
		"inner":   "a#b",
		"spaced":  "trimmed",
	} {
		wantString(t, cfg, path, want)
	}
}

// F1.7: rather than guess whether " #" opens a comment, say so and let the
// author quote the value.
func TestDotEnvAmbiguousHashIsError(t *testing.T) {
	_, err := env.Parse([]byte("FOO=bar # comment\n"), env.Options{})
	if err == nil {
		t.Fatal("value with a trailing # accepted, want error")
	}
	if !strings.Contains(err.Error(), "quote") {
		t.Errorf("error %q does not suggest quoting", err)
	}
}

func TestDotEnvErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unterminated double quote": `FOO="bar` + "\n",
		"unterminated single quote": "FOO='bar\n",
		"unknown escape":            `FOO="a\qb"` + "\n",
		"text after closing quote":  `FOO="bar" baz` + "\n",
		"missing equals":            "FOO\n",
		"empty name":                "=bar\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := env.Parse([]byte(src), env.Options{}); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
		})
	}
}

func TestDotEnvReportsLineNumber(t *testing.T) {
	_, err := env.Parse([]byte("A=1\nB=2\nC=\"oops\n"), env.Options{Origin: "sample.env"})
	if err == nil {
		t.Fatal("unterminated quote accepted, want error")
	}
	if !strings.Contains(err.Error(), "sample.env:3") {
		t.Errorf("error %q does not point at sample.env:3", err)
	}
}

// A .env file has a definite order, so a repeated name is last-wins (F0.7)
// rather than the collision error the process environment gets.
func TestDotEnvDuplicateLastWins(t *testing.T) {
	cfg := parseDotEnv(t, "A=1\nA=2\n")
	wantString(t, cfg, "a", "2")
}

func TestDotEnvPrefixFilter(t *testing.T) {
	cfg, err := env.Parse([]byte("APP_A=1\nOTHER=2\n"), env.Options{Prefix: "APP_"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	wantString(t, cfg, "a", "1")
	if cfg.GetStringOption("other").IsSome() {
		t.Error("unprefixed variable was mounted")
	}
}

// The reason the package exists: a HOCON document referencing deployment values
// that only exist in the environment.
func TestUseAsSubstitutionSourceUnderHOCON(t *testing.T) {
	base, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{
		"APP_DB__HOST=db.internal",
		"APP_DB__PORT=5432",
	}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Substitutions must stay unresolved until the fallback is attached.
	cfg, err := hocon.ParseStringWithOptions(
		`url = "postgres://"${db.host}":"${db.port}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantString(t, merged, "url", "postgres://db.internal:5432")
}

// F0.9: a leading BOM in a .env file is stripped rather than glued onto the
// first variable name.
func TestLeadingBOMStrippedFromDotEnv(t *testing.T) {
	cfg, err := env.Parse([]byte("\ufeffAPP_A=1\n"), env.Options{Prefix: "APP_"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.GetString("a"); got != "1" {
		t.Errorf("a = %q, want \"1\" — the BOM ended up in the name", got)
	}
}

// A name that maps to a path deeper than the limit is refused (spec F1.2).
//
// Go grows its goroutine stacks, so a deep chain here costs memory rather than
// crashing — py.hocon raised RecursionError at 497 segments and rs.hocon
// aborted the process outright. The limit is still here, at the same 64 the
// three siblings use, because otherwise the same environment mounts in one
// implementation and errors in another.
func TestDeepPathRefused(t *testing.T) {
	name := func(n int) string {
		return "APP_" + strings.Join(repeatSeg("s", n), "__")
	}
	if _, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{name(64) + "=v"}}); err != nil {
		t.Fatalf("64 segments must mount: %v", err)
	}
	for _, n := range []int{65, 10000} {
		_, err := env.Load(env.Options{Prefix: "APP_", Environ: []string{name(n) + "=v"}})
		if err == nil {
			t.Fatalf("%d segments accepted, want an error", n)
		}
		if !strings.Contains(err.Error(), "over the limit of 64") {
			t.Errorf("error %q does not name the limit", err)
		}
	}
}

// The same limit protects Parse, which reads arbitrary .env text.
func TestDeepPathRefusedInDotEnv(t *testing.T) {
	src := strings.Join(repeatSeg("S", 65), "__") + "=v\n"
	_, err := env.Parse([]byte(src), env.Options{})
	if err == nil || !strings.Contains(err.Error(), "over the limit of 64") {
		t.Fatalf("got %v, want a limit error", err)
	}
}

func repeatSeg(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}
