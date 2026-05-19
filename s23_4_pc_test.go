// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S23.4 — .properties dotted-key conflict: object wins over scalar (HOCON.md L1485).
// Fixtures: testdata/hocon/properties-conflict/pc01-pc04.properties (from xx.hocon, SHA 5beedfa).
// Expected: testdata/expected/properties-conflict/pc0N-expected.json.
//
// The fixtures are loaded via ParseString with a temporary inline include directive
// so that propsToObjectVal() is exercised through the real resolver path.

package hocon_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// pcFixture describes a properties-conflict conformance fixture.
type pcFixture struct {
	name string
}

var pcFixtures = []pcFixture{
	{"pc01-forward"},
	{"pc02-reverse"},
	{"pc03-deep-forward"},
	{"pc04-deep-reverse"},
}

func confPathPC(name string) string {
	return filepath.Join("testdata", "hocon", "properties-conflict", name+".properties")
}

func expectedPathPC(name string) string {
	return filepath.Join("testdata", "expected", "properties-conflict", name+"-expected.json")
}

// TestS23_4_PC_ObjectWins asserts that each properties-conflict fixture resolves to
// its expected JSON sidecar, where conflicting dotted keys yield objects, not scalars.
func TestS23_4_PC_ObjectWins(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "hocon", "properties-conflict")
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Skipf("properties-conflict fixtures missing at %s; run `make testdata`", fixtureDir)
	}
	expectedDir := filepath.Join("testdata", "expected", "properties-conflict")
	if _, err := os.Stat(expectedDir); err != nil {
		t.Skipf("properties-conflict expected JSON missing at %s; run `make testdata`", expectedDir)
	}

	for _, fx := range pcFixtures {
		fx := fx
		t.Run(fx.name, func(t *testing.T) {
			propsPath := confPathPC(fx.name)
			if _, err := os.Stat(propsPath); err != nil {
				t.Skipf("fixture %s not found: %v", propsPath, err)
			}
			expectedPath := expectedPathPC(fx.name)

			// Load the .properties file via an inline include directive.
			absPath, err := filepath.Abs(propsPath)
			if err != nil {
				t.Fatalf("Abs(%s): %v", propsPath, err)
			}
			slashPath := filepath.ToSlash(absPath)
			src := fmt.Sprintf(`include file("%s")`, slashPath)

			cfg, err := hocon.ParseString(src)
			if err != nil {
				t.Fatalf("ParseString(include %s): %v", propsPath, err)
			}

			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", expectedPath, err)
			}
			var want any
			if err := json.Unmarshal(expectedData, &want); err != nil {
				t.Fatalf("parse expected JSON %s: %v", expectedPath, err)
			}

			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if !jsonEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("mismatch\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
			}
		})
	}
}

// TestS23_4_InlineForwardOrder verifies the object-wins rule inline (forward order).
// "a=hello\na.b=world" → a should be an object {b: "world"}, not the string "hello".
func TestS23_4_InlineForwardOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	if err := os.WriteFile(path, []byte("a=hello\na.b=world\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg, err := hocon.ParseString(fmt.Sprintf(`include file("%s")`, slashPath))
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, ok := cfg.GetStringOption("a.b").Get()
	if !ok || got != "world" {
		t.Errorf("a.b = %q (ok=%v), want %q (object wins)", got, ok, "world")
	}
	if cfg.GetStringOption("a").IsSome() {
		t.Error("a must not be a plain string — object {b: world} wins over scalar 'hello'")
	}
}

// TestS23_4_InlineReverseOrder verifies that sort-based processing makes reverse-order
// identical to forward-order (both must produce {a: {b: "world"}}).
func TestS23_4_InlineReverseOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	if err := os.WriteFile(path, []byte("a.b=world\na=hello\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg, err := hocon.ParseString(fmt.Sprintf(`include file("%s")`, slashPath))
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, ok := cfg.GetStringOption("a.b").Get()
	if !ok || got != "world" {
		t.Errorf("a.b = %q (ok=%v), want %q (reverse order, sort makes it same)", got, ok, "world")
	}
}

// TestS23_4_DeepNest verifies the object-wins rule at deeper nesting levels.
// "a.b.c=v1\na.b=v2" → a.b should be an object {c: "v1"}, not the string "v2".
func TestS23_4_DeepNest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.properties")
	if err := os.WriteFile(path, []byte("a.b.c=v1\na.b=v2\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	slashPath := filepath.ToSlash(path)
	cfg, err := hocon.ParseString(fmt.Sprintf(`include file("%s")`, slashPath))
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, ok := cfg.GetStringOption("a.b.c").Get()
	if !ok || got != "v1" {
		t.Errorf("a.b.c = %q (ok=%v), want %q (object wins at depth 2)", got, ok, "v1")
	}
}
