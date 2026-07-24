// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package yaml_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/yaml"
)

func parse(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := yaml.Parse([]byte(src), "test.yaml")
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

func TestScalarsMappingsAndSequences(t *testing.T) {
	cfg := parse(t, `
name: svc
port: 8080
ratio: 0.5
debug: true
tags: [a, b]
db:
  host: localhost
  replicas:
    - id: 1
    - id: 2
`)
	wantString(t, cfg, "name", "svc")
	if got := cfg.GetInt64("port"); got != 8080 {
		t.Errorf("port = %d, want 8080", got)
	}
	if !cfg.GetBool("debug") {
		t.Error("debug = false, want true")
	}
	if got := cfg.GetStringSlice("tags"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("tags = %#v, want [a b]", got)
	}
	wantString(t, cfg, "db.host", "localhost")
	if r := cfg.GetConfigSlice("db.replicas"); len(r) != 2 || r[1].GetInt64("id") != 2 {
		t.Errorf("db.replicas did not become a list of objects: %#v", r)
	}
}

// F5.1 — the Norway problem. Under the YAML 1.2 core schema only true/false
// are booleans, so a country code stays a country code.
func TestNorwayProblemDoesNotArise(t *testing.T) {
	cfg := parse(t, "no: no\nyes: yes\non: on\noff: off\ny: y\nn: n\nreal: true\n")
	for _, path := range []string{"no", "yes", "on", "off", "y", "n"} {
		got, ok := cfg.GetStringOption(path).Get()
		if !ok {
			t.Errorf("path %q missing", path)
			continue
		}
		if got != path {
			t.Errorf("path %q = %q, want the literal string %q", path, got, path)
		}
	}
	if !cfg.GetBool("real") {
		t.Error("real = false, want true — true/false are still booleans")
	}
}

// F5.2 — anchors, aliases and merge keys are resolved before mapping.
func TestAnchorsAliasesAndMergeKeys(t *testing.T) {
	cfg := parse(t, `
defaults: &d
  host: localhost
  port: 5432
primary:
  <<: *d
  port: 6432
copy: *d
`)
	wantString(t, cfg, "primary.host", "localhost")
	if got := cfg.GetInt64("primary.port"); got != 6432 {
		t.Errorf("primary.port = %d, want 6432 (the merge key must not win over an explicit field)", got)
	}
	if got := cfg.GetInt64("copy.port"); got != 5432 {
		t.Errorf("copy.port = %d, want 5432", got)
	}
}

// F5.3 — non-string scalar keys become their string forms.
func TestNonStringKeysBecomeStrings(t *testing.T) {
	cfg := parse(t, "1: one\n2.5: half\ntrue: yes-key\n")
	for path, want := range map[string]string{"1": "one", "2.5": "half", "true": "yes-key"} {
		got, ok := cfg.GetStringOption(`"` + path + `"`).Get()
		if !ok {
			t.Errorf("key %q missing", path)
			continue
		}
		if got != want {
			t.Errorf("key %q = %q, want %q", path, got, want)
		}
	}
}

// F5.4 — a timestamp stays a string; there is no datetime type to unwrap.
func TestTimestampStaysString(t *testing.T) {
	cfg := parse(t, "at: 2001-12-14t21:59:43.10-05:00\nday: 2002-12-14\n")
	wantString(t, cfg, "at", "2001-12-14t21:59:43.10-05:00")
	wantString(t, cfg, "day", "2002-12-14")
}

// F5.5 — !!binary keeps the base64 text the source carried.
func TestBinaryBecomesBase64String(t *testing.T) {
	cfg := parse(t, "blob: !!binary aGk=\n")
	wantString(t, cfg, "blob", "aGk=")
}

// F5.6 — HOCON's number model cannot hold these.
func TestNaNAndInfinityRejected(t *testing.T) {
	for _, src := range []string{"a: .nan", "a: .inf", "a: -.inf"} {
		t.Run(src, func(t *testing.T) {
			_, err := yaml.Parse([]byte(src), "test.yaml")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
			if !strings.Contains(err.Error(), "F0.6") {
				t.Errorf("error %q does not cite the spec item F0.6", err)
			}
		})
	}
}

// F5.7 — the reason this adapter does not just call Unmarshal: that would
// return document one and discard the rest without saying so.
func TestMultiDocumentRejected(t *testing.T) {
	_, err := yaml.Parse([]byte("a: 1\n---\nb: 2\n"), "test.yaml")
	if err == nil {
		t.Fatal("multi-document stream accepted, want error")
	}
	if !strings.Contains(err.Error(), "F5.7") {
		t.Errorf("error %q does not cite the spec item F5.7", err)
	}
}

// F5.8 — a duplicate key in hand-written YAML is a mistake, not an override.
func TestDuplicateKeyRejected(t *testing.T) {
	if _, err := yaml.Parse([]byte("a: 1\na: 2\n"), "test.yaml"); err == nil {
		t.Fatal("duplicate key accepted, want error")
	}
}

// F5.9 — an empty document is the empty object, as in HOCON itself (S3.1).
func TestEmptyDocumentIsEmptyObject(t *testing.T) {
	for _, src := range []string{"", "\n", "# just a comment\n"} {
		cfg, err := yaml.Parse([]byte(src), "test.yaml")
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if n := len(cfg.Keys()); n != 0 {
			t.Errorf("Parse(%q) produced %d keys, want 0", src, n)
		}
	}
}

// F0.3 — a config root has to be a mapping.
func TestSequenceRootRejected(t *testing.T) {
	_, err := yaml.Parse([]byte("- 1\n- 2\n"), "test.yaml")
	if err == nil {
		t.Fatal("sequence root accepted, want error")
	}
	if !strings.Contains(err.Error(), "F0.3") {
		t.Errorf("error %q does not cite the spec item F0.3", err)
	}
}

// F0.5 — an integer past int64 is an error, not a silent widening.
func TestIntegerOverflowRejected(t *testing.T) {
	_, err := yaml.Parse([]byte("a: 18446744073709551615\n"), "test.yaml")
	if err == nil {
		t.Fatal("out-of-range integer accepted, want error")
	}
	if !strings.Contains(err.Error(), "F0.5") {
		t.Errorf("error %q does not cite the spec item F0.5", err)
	}
}

// Measured behaviour of the default library, pinned so a change in it is
// visible rather than silent. Not a portability contract: scalar resolution
// belongs to the library (spec F5 "Scope"), and a config that depends on
// these forms should quote them instead.
func TestInheritedDecoderQuirks(t *testing.T) {
	cfg := parse(t, "octal: 010\nunderscored: 1_000\nhuge: 99999999999999999999999\n")
	if got := cfg.GetInt64("octal"); got != 8 {
		t.Errorf("octal = %d, want 8 — a leading zero is octal (YAML 1.1 legacy)", got)
	}
	if got := cfg.GetInt64("underscored"); got != 1000 {
		t.Errorf("underscored = %d, want 1000", got)
	}
	// Past 64 bits the decoder hands back a string, so F0.5's overflow rule
	// never gets a chance to fire.
	wantString(t, cfg, "huge", "99999999999999999999999")
}

// F0.2 — a ${...} in foreign data is text, not a reference.
func TestSubstitutionSyntaxStaysLiteral(t *testing.T) {
	cfg := parse(t, `a: "${foo.bar}"`)
	resolved, err := cfg.Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantString(t, resolved, "a", "${foo.bar}")
}

// The reason the package exists: a HOCON document reading values out of a
// compose file some other tool owns.
func TestUseAsSubstitutionSourceUnderHOCON(t *testing.T) {
	base, err := yaml.Parse([]byte("services:\n  db:\n    image: postgres:16\n"), "docker-compose.yml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg, err := hocon.ParseStringWithOptions(
		`image = ${services.db.image}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantString(t, merged, "image", "postgres:16")
}

// FromValue is the boundary this package owns: the caller decodes with
// whatever library and settings they chose, and hands the tree over. These
// trees imitate the shapes other Go YAML libraries produce.
func TestFromValueAcceptsOtherLibrariesShapes(t *testing.T) {
	// yaml.v2-era shape: map[any]any with non-string scalar keys, int leaves.
	doc := map[any]any{
		"db": map[any]any{"port": int(5432)},
		1:    "one",
		true: "yes-key",
	}
	cfg, err := yaml.FromValue(doc, "injected")
	if err != nil {
		t.Fatalf("FromValue: %v", err)
	}
	if got := cfg.GetInt64("db.port"); got != 5432 {
		t.Errorf("db.port = %d, want 5432", got)
	}
	wantString(t, cfg, `"1"`, "one")
	wantString(t, cfg, `"true"`, "yes-key")
}

// go.yaml.in resolves timestamps to time.Time; the tree rule maps it to its
// RFC 3339 text, as a TOML date is (F4.2's reasoning).
func TestFromValueTimeBecomesRFC3339String(t *testing.T) {
	at := time.Date(2002, 12, 14, 21, 59, 43, 0, time.UTC)
	cfg, err := yaml.FromValue(map[string]any{"at": at}, "injected")
	if err != nil {
		t.Fatalf("FromValue: %v", err)
	}
	wantString(t, cfg, "at", "2002-12-14T21:59:43Z")
}

// A collection key has no string form and is refused (F5.3).
func TestFromValueCollectionKeyRejected(t *testing.T) {
	_, err := yaml.FromValue(map[any]any{[2]any{1, 2}: "pair"}, "injected")
	if err == nil {
		t.Fatal("collection key accepted, want error")
	}
	if !strings.Contains(err.Error(), "F5.3") {
		t.Errorf("error %q does not cite the spec item F5.3", err)
	}
}

// nil is the empty document, whatever produced it (F5.9).
func TestFromValueNilIsEmpty(t *testing.T) {
	cfg, err := yaml.FromValue(nil, "injected")
	if err != nil {
		t.Fatalf("FromValue: %v", err)
	}
	if n := len(cfg.Keys()); n != 0 {
		t.Errorf("got %d keys, want 0", n)
	}
}
