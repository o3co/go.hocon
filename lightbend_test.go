// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// TestLightbendEquiv tests the equiv01-05 directories from the Lightbend test suite.
// Each directory contains multiple .conf files that should all parse to produce
// the same result as original.json in that directory.
func TestLightbendEquiv(t *testing.T) {
	baseDir := "testdata/hocon"

	// Known skip reasons for specific test files
	skipFiles := map[string]string{}

	for i := 1; i <= 5; i++ {
		dir := filepath.Join(baseDir, fmt.Sprintf("equiv%02d", i))
		jsonPath := filepath.Join(dir, "original.json")

		jsonData, err := os.ReadFile(jsonPath)
		if err != nil {
			t.Logf("skipping equiv%02d: no original.json found", i)
			continue
		}

		var want any
		if err := json.Unmarshal(jsonData, &want); err != nil {
			t.Fatalf("equiv%02d: failed to parse original.json: %v", i, err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("equiv%02d: cannot read directory: %v", i, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}

			relPath := filepath.Join(fmt.Sprintf("equiv%02d", i), e.Name())
			confPath := filepath.Join(dir, e.Name())

			t.Run(relPath, func(t *testing.T) {
				if reason, ok := skipFiles[filepath.ToSlash(relPath)]; ok {
					t.Skipf("skipped: %s", reason)
				}

				cfg, err := hocon.ParseFile(confPath)
				if err != nil {
					t.Fatalf("ParseFile(%s): %v", confPath, err)
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
}

// TestLightbendSuite tests individual .conf files that have matching .json expected outputs
// at the top level of the test data directory.
func TestLightbendSuite(t *testing.T) {
	dir := "testdata/hocon"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("lightbend test data not found at %s: %v", dir, err)
	}

	// Files where the .json is NOT an expected output but rather an include source
	// (test01.json has different keys from test01.conf — it's used as JSON include data)
	excludeFromPairing := map[string]bool{
		"test01.conf": true,
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		if excludeFromPairing[e.Name()] {
			continue
		}

		confPath := filepath.Join(dir, e.Name())
		jsonPath := strings.TrimSuffix(confPath, ".conf") + ".json"
		if _, err2 := os.Stat(jsonPath); os.IsNotExist(err2) {
			continue // no expected output — skip
		}

		t.Run(e.Name(), func(t *testing.T) {
			cfg, err3 := hocon.ParseFile(confPath)
			if err3 != nil {
				t.Fatalf("ParseFile: %v", err3)
			}
			expected, err4 := os.ReadFile(jsonPath)
			if err4 != nil {
				t.Fatalf("ReadFile expected: %v", err4)
			}
			var want map[string]any
			if err5 := json.Unmarshal(expected, &want); err5 != nil {
				t.Skipf("cannot parse expected JSON: %v", err5)
			}
			got := make(map[string]any)
			if err6 := cfg.Unmarshal(&got); err6 != nil {
				t.Fatalf("Unmarshal: %v", err6)
			}
			if !jsonEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				t.Errorf("mismatch\ngot:\n%s\nwant:\n%s", gotJSON, expected)
			}
		})
	}
}

// jsonEqual compares two values by serializing to JSON.
// This normalizes number types (int64 vs float64) for comparison.
func jsonEqual(a, b any) bool {
	aj, _ := json.Marshal(normalizeForJSON(a))
	bj, _ := json.Marshal(normalizeForJSON(b))
	return string(aj) == string(bj)
}

// TestLightbendExpected auto-discovers expected JSON files from xx.hocon
// and compares parsed .conf output against them.
func TestLightbendExpected(t *testing.T) {
	confDir := "testdata/hocon"
	expectedDir := "testdata/expected"

	entries, err := os.ReadDir(expectedDir)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("expected JSON dir not found at %s in CI — fetch testdata before running tests: %v", expectedDir, err)
		}
		t.Skipf("expected JSON dir not found at %s — run `make testdata` first: %v", expectedDir, err)
		return
	}

	// Known failures — skip tests that cannot pass yet
	skip := map[string]string{
		"test01-expected.json": "system section contains environment-dependent values",
		"test02-expected.json": "unresolved substitution for empty-key path",
	}

	for _, e := range entries {
		name := e.Name()

		if !strings.HasSuffix(name, "-expected.json") || strings.Contains(name, "-expected-error") {
			continue
		}
		if reason, ok := skip[name]; ok {
			t.Run(name, func(t *testing.T) {
				t.Skipf("known failure: %s", reason)
			})
			continue
		}

		confName := strings.Replace(name, "-expected.json", ".conf", 1)
		confPath := filepath.Join(confDir, confName)
		expectedPath := filepath.Join(expectedDir, name)

		t.Run(confName, func(t *testing.T) {
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("conf not found: %s", confPath)
				return
			}

			cfg, err := hocon.ParseFile(confPath)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}

			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
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
				t.Errorf("mismatch\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
			}
		})
	}
}

// TestLightbendExpectedErrors auto-discovers expected error files.
func TestLightbendExpectedErrors(t *testing.T) {
	confDir := "testdata/hocon"
	expectedDir := "testdata/expected"

	entries, err := os.ReadDir(expectedDir)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("expected JSON dir not found at %s in CI — fetch testdata before running tests: %v", expectedDir, err)
		}
		t.Skipf("expected dir not found at %s — run `make testdata` first: %v", expectedDir, err)
		return
	}

	tested := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "-expected-error.json") {
			continue
		}

		confName := strings.Replace(name, "-expected-error.json", ".conf", 1)
		confPath := filepath.Join(confDir, confName)

		t.Run(confName+" should error", func(t *testing.T) {
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("conf not found: %s", confPath)
				return
			}

			_, err := hocon.ParseFile(confPath)
			if err == nil {
				t.Errorf("expected error for %s but got success", confPath)
			}
		})
		tested++
	}

	if tested == 0 {
		t.Error("No expected error tests were run. Check testdata/expected/")
	}
}

// TestSubstTokenizeSuccess auto-discovers subst-tokenize success fixtures.
func TestSubstTokenizeSuccess(t *testing.T) {
	expectedDir := filepath.Join("testdata", "expected", "subst-tokenize")
	if _, err := os.Stat(expectedDir); err != nil {
		t.Skip("subst-tokenize fixtures missing; run `make testdata`")
	}

	entries, err := os.ReadDir(expectedDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "-expected.json") {
			continue
		}
		confName := strings.Replace(name, "-expected.json", ".conf", 1)
		ran++
		t.Run(confName, func(t *testing.T) {
			confPath := filepath.Join("testdata", "hocon", "subst-tokenize", confName)
			expectedPath := filepath.Join(expectedDir, name)

			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("conf not found: %s", confPath)
				return
			}

			cfg, err := hocon.ParseFile(confPath)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", confPath, err)
			}

			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
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
				t.Errorf("mismatch\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no subst-tokenize success fixtures found — run `make testdata`")
	}
}

// TestSubstTokenizeErrors auto-discovers subst-tokenize error fixtures.
// Spec Goal 2: error position must fall within the offending ${...} body.
// All fixtures are single-line, so we assert Line == 1 and that Col falls
// within the [substStart, substEnd] byte span of the ${...} token.
func TestSubstTokenizeErrors(t *testing.T) {
	expectedDir := filepath.Join("testdata", "expected", "subst-tokenize")
	entries, err := os.ReadDir(expectedDir)
	if err != nil {
		t.Fatal("subst-tokenize error fixtures missing — run `make testdata`")
	}
	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "-expected-error.json") {
			continue
		}
		confName := strings.Replace(name, "-expected-error.json", ".conf", 1)
		ran++
		t.Run(confName+"_should_error", func(t *testing.T) {
			confPath := filepath.Join("testdata", "hocon", "subst-tokenize", confName)
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("conf not found: %s", confPath)
				return
			}
			data, err := os.ReadFile(confPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = hocon.ParseString(string(data))
			if err == nil {
				t.Fatalf("expected error for %s", confName)
			}
			// Spec Goal 2: error position must fall within the offending ${...} body.
			// All fixtures are single-line. Compute the ${...} span from fixture data
			// and assert pe.Col falls within [minCol, maxCol] (1-based columns).
			var pe *hocon.ParseError
			if errors.As(err, &pe) {
				if pe.Line != 1 {
					t.Errorf("%s: expected error at line 1, got line %d (err: %v)", confName, pe.Line, err)
				}
				// Find the ${ in the fixture.
				substStart := strings.Index(string(data), "${")
				if substStart < 0 {
					t.Fatalf("%s: no ${ in fixture", confName)
				}
				// Find the matching } (if missing, treat EOF as end for unterminated subst).
				substEnd := -1
				if idx := strings.Index(string(data[substStart:]), "}"); idx >= 0 {
					substEnd = substStart + idx
				} else {
					substEnd = len(data) - 1
				}
				// Convert to 1-based columns (on line 1, col == byte offset + 1).
				minCol := substStart + 1
				maxCol := substEnd + 1
				if pe.Col < minCol || pe.Col > maxCol {
					t.Errorf("%s: error col %d not in ${...} range [%d, %d] (err: %v)",
						confName, pe.Col, minCol, maxCol, err)
				}
			} else {
				// Non-ParseError: check the message contains "line 1" for position evidence.
				msg := err.Error()
				if !strings.Contains(msg, "line 1") {
					t.Errorf("%s: error message does not mention line 1: %v", confName, err)
				}
			}
		})
	}
	if ran == 0 {
		t.Fatal("no subst-tokenize error fixtures found — run `make testdata`")
	}
}

// normalizeForJSON converts int64 values to float64 to match encoding/json's
// default number type when unmarshaling into any.
func normalizeForJSON(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(vv))
		for k, val := range vv {
			m[k] = normalizeForJSON(val)
		}
		return m
	case []any:
		s := make([]any, len(vv))
		for i, val := range vv {
			s[i] = normalizeForJSON(val)
		}
		return s
	case int64:
		return float64(vv)
	case int:
		return float64(vv)
	default:
		return vv
	}
}
