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
