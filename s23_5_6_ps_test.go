// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S23.5 / S23.6 — .properties full syntax: backslash continuations and unicode
// escapes, plus the separator and value-whitespace rules that come with them.
// Both items were globally out-of-scope until 2026-07-24.
//
// Fixtures: testdata/hocon/properties-syntax/ps01-ps05.properties (from xx.hocon).
// Expected: testdata/expected/properties-syntax/psNN-expected.json, generated
// from Lightbend, which reads the file with java.util.Properties.
//
// The fixtures are loaded through a temporary inline include so that the real
// resolver path is what gets checked, matching s23_4_pc_test.go.

package hocon_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

var psFixtures = []string{
	"ps01-continuation",
	"ps02-escapes",
	"ps03-separators",
	"ps04-value-whitespace",
	"ps05-astral",
}

func TestS23_5_6_PropertiesSyntax(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "hocon", "properties-syntax")
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Skipf("properties-syntax fixtures missing at %s; run `make testdata`", fixtureDir)
	}
	expectedDir := filepath.Join("testdata", "expected", "properties-syntax")
	if _, err := os.Stat(expectedDir); err != nil {
		t.Skipf("properties-syntax expected JSON missing at %s; run `make testdata`", expectedDir)
	}

	for _, name := range psFixtures {
		t.Run(name, func(t *testing.T) {
			propsPath := filepath.Join(fixtureDir, name+".properties")
			if _, err := os.Stat(propsPath); err != nil {
				t.Skipf("fixture %s not found: %v", propsPath, err)
			}
			absPath, err := filepath.Abs(propsPath)
			if err != nil {
				t.Fatalf("Abs(%s): %v", propsPath, err)
			}
			src := fmt.Sprintf(`include file(%q)`, filepath.ToSlash(absPath))

			cfg, err := hocon.ParseString(src)
			if err != nil {
				t.Fatalf("ParseString(include %s): %v", propsPath, err)
			}

			expectedData, err := os.ReadFile(filepath.Join(expectedDir, name+"-expected.json"))
			if err != nil {
				t.Fatalf("ReadFile expected: %v", err)
			}
			var want any
			if err := json.Unmarshal(expectedData, &want); err != nil {
				t.Fatalf("parse expected JSON: %v", err)
			}

			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if !jsonEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("mismatch against the Lightbend oracle\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
			}
		})
	}
}
