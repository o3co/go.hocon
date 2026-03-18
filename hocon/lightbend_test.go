// Copyright 2026 o3co Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.lib/hocon"
)

// TestLightbendEquiv tests the equiv01-05 directories from the Lightbend test suite.
// Each directory contains multiple .conf files that should all parse to produce
// the same result as original.json in that directory.
func TestLightbendEquiv(t *testing.T) {
	baseDir := "testdata/hocon"

	// Known skip reasons for specific test files
	skipFiles := map[string]string{
		// equiv03/includes.conf uses include with .properties files and
		// relative include paths that require classpath-like resolution
		"equiv03/includes.conf": "uses include of .properties and relative sub-includes",
	}

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
				if reason, ok := skipFiles[relPath]; ok {
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
