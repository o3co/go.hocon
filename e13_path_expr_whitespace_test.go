// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// E13 path-expression whitespace preservation (xx.hocon#42) — pw01–pw07.
//
// Lightbend preserves literal whitespace adjacent to dots in path expressions:
//   a b. c = 1   →  {"a b":{" c":1}}     // leading space on " c" preserved
//   a b.\tc = 1  →  {"a b":{"\tc":1}}    // tab preserved (HOCON_WS includes tab)
// go.hocon previously stripped leading whitespace from post-dot segments.
// See xx.hocon docs/extra-spec-conventions.md E13.
//
// 6 success fixtures + 1 error fixture (pw06: trailing-dot still BadPath —
// loosening does NOT cascade into empty path segments).
//
// Fixtures: testdata/hocon/path-expr-whitespace/pw01-pw07.conf
// Expected: testdata/expected/path-expr-whitespace/{pw*-expected.json, pw06-*.error}

package hocon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

const (
	pwConfDir     = "testdata/hocon/path-expr-whitespace"
	pwExpectedDir = "testdata/expected/path-expr-whitespace"
)

func TestE13_PathExprWhitespace(t *testing.T) {
	if _, err := os.Stat(pwConfDir); os.IsNotExist(err) {
		t.Skipf("path-expr-whitespace fixtures missing at %s; run `make testdata`", pwConfDir)
	}
	if _, err := os.Stat(pwExpectedDir); os.IsNotExist(err) {
		t.Skipf("path-expr-whitespace expected dir missing at %s; run `make testdata`", pwExpectedDir)
	}

	entries, err := os.ReadDir(pwConfDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", pwConfDir, err)
	}

	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		stem := strings.TrimSuffix(name, ".conf")
		confPath := filepath.Join(pwConfDir, name)
		jsonPath := filepath.Join(pwExpectedDir, stem+"-expected.json")
		errPath := filepath.Join(pwExpectedDir, stem+".error")

		_, jsonStat := os.Stat(jsonPath)
		_, errStat := os.Stat(errPath)

		isError := errStat == nil
		isSuccess := jsonStat == nil

		if !isError && !isSuccess {
			t.Errorf("%s: neither -expected.json nor .error sidecar found at %s", stem, pwExpectedDir)
			continue
		}

		ran++
		t.Run(stem, func(t *testing.T) {
			if isError {
				// Sidecar existence is the signal (per xx.hocon docs/fixture-conventions.md);
				// message content is not asserted across impls.
				_, parseErr := hocon.ParseFile(confPath)
				if parseErr == nil {
					t.Errorf("%s: expected parse error per .error sidecar, parse succeeded", stem)
				}
				return
			}

			// success path
			cfg, parseErr := hocon.ParseFile(confPath)
			if parseErr != nil {
				t.Fatalf("ParseFile(%s): %v\n  go.hocon must preserve literal whitespace adjacent to dots per E13 (xx.hocon#42).", confPath, parseErr)
			}

			expectedData, readErr := os.ReadFile(jsonPath)
			if readErr != nil {
				t.Fatalf("ReadFile(%s): %v", jsonPath, readErr)
			}

			var want any
			if jsonErr := json.Unmarshal(expectedData, &want); jsonErr != nil {
				t.Fatalf("Unmarshal expected JSON (%s): %v", jsonPath, jsonErr)
			}

			got := make(map[string]any)
			if unmarshalErr := cfg.Unmarshal(&got); unmarshalErr != nil {
				t.Fatalf("cfg.Unmarshal: %v", unmarshalErr)
			}

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			if string(gotJSON) != string(wantJSON) {
				gotPretty, _ := json.MarshalIndent(got, "", "  ")
				t.Errorf("%s: result mismatch\ngot:\n%s\nwant:\n%s", stem, gotPretty, expectedData)
			}
		})
	}

	if ran == 0 {
		t.Errorf("no pw* fixtures executed; check %s and %s", pwConfDir, pwExpectedDir)
	}
}
