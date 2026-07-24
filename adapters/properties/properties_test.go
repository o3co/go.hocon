// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package properties_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/properties"
)

func parse(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := properties.Parse([]byte(src), "test.properties")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return cfg
}

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

func TestDottedKeysNest(t *testing.T) {
	cfg := parse(t, "db.host = db.internal\ndb.port = 5432\n")
	wantString(t, cfg, "db.host", "db.internal")
	wantString(t, cfg, "db.port", "5432")
}

func TestSeparators(t *testing.T) {
	cfg := parse(t, "a:1\nb 2\nc = 3\nd\t=\t4\ne=\n")
	for path, want := range map[string]string{"a": "1", "b": "2", "c": "3", "d": "4", "e": ""} {
		wantString(t, cfg, path, want)
	}
}

func TestComments(t *testing.T) {
	cfg := parse(t, "# hash comment\n! bang comment\n  # indented\n\na = 1\n")
	wantString(t, cfg, "a", "1")
	if n := len(cfg.Keys()); n != 1 {
		t.Errorf("got %d top-level keys, want 1", n)
	}
}

func TestLineContinuation(t *testing.T) {
	cfg := parse(t, "a = one\\\n      two\n")
	wantString(t, cfg, "a", "onetwo")
}

// An even number of trailing backslashes is an escaped backslash, not a
// continuation marker.
func TestEscapedTrailingBackslashIsNotContinuation(t *testing.T) {
	cfg := parse(t, "a = end\\\\\nb = 2\n")
	wantString(t, cfg, "a", `end\`)
	wantString(t, cfg, "b", "2")
}

func TestEscapes(t *testing.T) {
	cfg := parse(t, "a = x\\ty\nb = \\u00e9\nc = q\\zr\nd = colon\\:in\\=key\n")
	wantString(t, cfg, "a", "x\ty")
	wantString(t, cfg, "b", "é")
	wantString(t, cfg, "c", "qzr") // unknown escape: backslash dropped, as in Java
	wantString(t, cfg, "d", "colon:in=key")
}

func TestEscapedSeparatorInKey(t *testing.T) {
	cfg := parse(t, "a\\:b = 1\n")
	wantString(t, cfg, "a:b", "1")
}

func TestSurrogatePair(t *testing.T) {
	cfg := parse(t, "a = \\ud83d\\ude00\n")
	wantString(t, cfg, "a", "\U0001F600")
}

func TestUnpairedSurrogateIsError(t *testing.T) {
	if _, err := properties.Parse([]byte("a = \\ud83d\n"), "test.properties"); err == nil {
		t.Fatal("unpaired surrogate accepted, want error")
	}
}

func TestTruncatedUnicodeEscapeIsError(t *testing.T) {
	if _, err := properties.Parse([]byte("a = \\u12\n"), "test.properties"); err == nil {
		t.Fatal("truncated \\u accepted, want error")
	}
}

// F2.5: `a` is dropped because `a.b` makes it a parent.
func TestObjectsWin(t *testing.T) {
	cfg := parse(t, "a = 1\na.b = 2\n")
	wantString(t, cfg, "a.b", "2")
	if cfg.GetStringOption("a").IsSome() {
		t.Error("scalar at `a` survived, want it dropped in favour of the object")
	}
}

// F0.2: the file belongs to another program, so ${...} is data, not a reference.
func TestSubstitutionSyntaxStaysLiteral(t *testing.T) {
	cfg := parse(t, "a = ${foo.bar}\n")
	wantString(t, cfg, "a", "${foo.bar}")

	// Still literal after a resolve pass, where a real substitution would fail
	// on the undefined path `foo.bar`.
	resolved, err := cfg.Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantString(t, resolved, "a", "${foo.bar}")
}

func TestQuotedPathSegmentRejected(t *testing.T) {
	_, err := properties.Parse([]byte(`foo."bar.baz" = 1`), "test.properties")
	if err == nil {
		t.Fatal("quoted path segment accepted, want error")
	}
	if !strings.Contains(err.Error(), "F2.7") {
		t.Errorf("error %q does not cite the spec item F2.7", err)
	}
}

func TestInvalidUTF8Rejected(t *testing.T) {
	if _, err := properties.Parse([]byte{'a', '=', 0xff}, "test.properties"); err == nil {
		t.Fatal("invalid UTF-8 accepted, want error")
	}
}

func TestEmptyKeyRejected(t *testing.T) {
	if _, err := properties.Parse([]byte("= value\n"), "test.properties"); err == nil {
		t.Fatal("empty key accepted, want error")
	}
}

func TestCRLFAndCR(t *testing.T) {
	cfg := parse(t, "a = 1\r\nb = 2\rc = 3\n")
	for path, want := range map[string]string{"a": "1", "b": "2", "c": "3"} {
		wantString(t, cfg, path, want)
	}
}

func TestEmptyInput(t *testing.T) {
	cfg := parse(t, "")
	if n := len(cfg.Keys()); n != 0 {
		t.Errorf("got %d keys, want 0", n)
	}
}

// The reason the package exists: a HOCON document referencing values that live
// in a Properties file owned by some other program.
func TestUseAsSubstitutionSourceUnderHOCON(t *testing.T) {
	base, err := properties.Parse([]byte("db.host = db.internal\ndb.port = 5432\n"), "service.properties")
	if err != nil {
		t.Fatalf("Parse: %v", err)
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
