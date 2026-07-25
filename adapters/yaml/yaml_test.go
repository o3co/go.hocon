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

// F0.9 — a leading BOM is stripped rather than glued onto the first key.
func TestLeadingBOMStripped(t *testing.T) {
	cfg := parse(t, "\ufeffa: 1\n")
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a = %d, want 1 — the BOM ended up in the key", got)
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

// F5.3 on the Parse path. goccy's own duplicate-key detection compares the
// key *text*, so it catches 1: against "1": but not a key that only resolves
// to the same string — 1.0, 0x10, 01, +1, ~ all stringify onto another key's
// text, and the loser used to vanish without a word.
func TestParseCollidingKeyFormsRejected(t *testing.T) {
	for name, src := range map[string]string{
		"float and quoted int": "1.0: a\n\"1\": b\n",
		"hex and quoted int":   "0x10: a\n\"16\": b\n",
		"octal and quoted int": "01: a\n\"1\": b\n",
		"signed and quoted":    "+1: a\n\"1\": b\n",
		"null and quoted null": "~: a\n\"null\": b\n",
		"int and float":        "1: a\n1.0: b\n",
		"case of true":         "true: a\nTrue: b\n",
		"signed zero":          "0: a\n-0: b\n",
		"octal and decimal":    "8: a\n0o10: b\n",
		"hex and decimal":      "1: a\n0x1: b\n",
		"inside a sequence":    "l:\n  - 1.0: a\n    \"1\": b\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := yaml.Parse([]byte(src), "test.yaml")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error — a value is being dropped", src)
			}
			if !strings.Contains(err.Error(), "F5.3") {
				t.Errorf("error %q does not cite the spec item F5.3", err)
			}
		})
	}
}

// A collision below the root names the path to it, so it does not read like a
// top-level one.
func TestParseCollisionErrorNamesThePath(t *testing.T) {
	_, err := yaml.Parse([]byte("db:\n  ports:\n    1.0: a\n    \"1\": b\n"), "test.yaml")
	if err == nil {
		t.Fatal("colliding key forms accepted, want error")
	}
	if !strings.Contains(err.Error(), "db.ports.1") {
		t.Errorf("error %q does not locate the collision at db.ports.1", err)
	}
	// Both spellings, with their lines, so the reader can go straight to them.
	for _, want := range []string{"1.0 (line 3)", `"1" (line 4)`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the colliding key %s", err, want)
		}
	}
}

// Measured goccy behaviour, pinned so a change in it is visible: identical key
// *text* is the library's own error, and it fires before the adapter sees the
// document. If goccy ever stops reporting it, the adapter's check above still
// covers the case — but the message would change, and this test says so.
func TestInheritedDuplicateKeyDetection(t *testing.T) {
	for name, src := range map[string]string{
		"same text twice":     "a: 1\na: 2\n",
		"plain versus quoted": "1: a\n\"1\": b\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := yaml.Parse([]byte(src), "test.yaml")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
			if !strings.Contains(err.Error(), "already defined") {
				t.Errorf("error %q is not goccy's duplicate-key report; "+
					"if the adapter now catches this first, update this pin", err)
			}
		})
	}
}

// Ordinary documents must still round-trip once the decoder is asked for
// ordered maps: merge keys stay resolved (F5.2), sequences of mappings keep
// their shape, and binary still becomes base64 text (F5.5).
func TestOrderedDecodeKeepsDocumentSemantics(t *testing.T) {
	cfg := parse(t, `
base: &b
  a: 1
child:
  <<: *b
  c: 2
list:
  - k: 1
  - k: 2
blob: !!binary aGk=
`)
	if got := cfg.GetInt64("child.a"); got != 1 {
		t.Errorf("child.a = %d, want 1 — merge key not resolved (F5.2)", got)
	}
	if got := cfg.GetInt64("child.c"); got != 2 {
		t.Errorf("child.c = %d, want 2", got)
	}
	if l := cfg.GetConfigSlice("list"); len(l) != 2 || l[1].GetInt64("k") != 2 {
		t.Errorf("list did not stay a sequence of mappings: %#v", l)
	}
	wantString(t, cfg, "blob", "aGk=")
}

// F5.3: two sibling keys whose string forms coincide are an error, not a
// last-writer-wins race under Go's randomized map iteration. Parse gets this
// from the ordered decode below; the injected-tree path has to enforce it on
// whatever shape the caller hands over.
func TestFromValueCollidingKeyFormsRejected(t *testing.T) {
	for name, doc := range map[string]any{
		"int 1 and string 1": map[any]any{1: "from-int", "1": "from-string"},
		"bool and string true": map[any]any{
			true: "from-bool", "true": "from-string",
		},
		"nested collision": map[string]any{
			"m": map[any]any{1: "a", "1": "b"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := yaml.FromValue(doc, "injected")
			if err == nil {
				t.Fatal("colliding key forms accepted, want error")
			}
			if !strings.Contains(err.Error(), "F5.3") {
				t.Errorf("error %q does not cite the spec item F5.3", err)
			}
		})
	}
}

// The collision error names both source keys, so the author can tell which
// two lines of the tree are fighting.
func TestCollidingKeyErrorNamesBothForms(t *testing.T) {
	_, err := yaml.FromValue(map[any]any{1: "a", "1": "b"}, "injected")
	if err == nil {
		t.Fatal("colliding key forms accepted, want error")
	}
	for _, want := range []string{"1 (int)", `"1"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name the colliding key %s", err, want)
		}
	}
}

// Distinct string forms keep working, including the non-string scalars that
// merely stringify (F5.3's normal case).
func TestDistinctKeyFormsUnaffected(t *testing.T) {
	cfg, err := yaml.FromValue(map[any]any{1: "one", 2: "two", "three": "3"}, "injected")
	if err != nil {
		t.Fatalf("FromValue: %v", err)
	}
	wantString(t, cfg, `"1"`, "one")
	wantString(t, cfg, `"2"`, "two")
	wantString(t, cfg, "three", "3")
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
