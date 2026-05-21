// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S8.1 / S8.8 — parens `(` `)` are not in HOCON.md L274 forbidden set and
// must be accepted as ordinary unquoted-string content outside include
// resource syntax (`file(...)` / `required(...)` / `classpath(...)` /
// `url(...)`). ts.hocon and rs.hocon already match this reading; go.hocon
// previously emitted `TokenLParen`/`TokenRParen` as standalone tokens
// unconditionally and rejected `a = hello (world)` style values.
//
// Fixtures: testdata/hocon/unquoted-parens/up01-up06 (xx.hocon SHA 5b9c1ba).
// Expected: testdata/expected/unquoted-parens/up0N-expected.json (Lightbend
// typesafe-config 1.4.3 ground truth).
//
// Background: xx.hocon#34 external report from @cgordon; impl tracking
// go.hocon#100. Cross-impl spec PR: xx.hocon#35.

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
	upConfDir     = "testdata/hocon/unquoted-parens"
	upExpectedDir = "testdata/expected/unquoted-parens"
)

// TestS8_UnquotedParens drives all up01–up06 fixtures and asserts each parses
// without error AND the resolved value matches the Lightbend-faithful expected
// JSON byte-for-byte (after normalization).
func TestS8_UnquotedParens(t *testing.T) {
	if _, err := os.Stat(upConfDir); os.IsNotExist(err) {
		t.Skipf("unquoted-parens fixtures missing at %s; run `make testdata`", upConfDir)
	}
	if _, err := os.Stat(upExpectedDir); os.IsNotExist(err) {
		t.Skipf("unquoted-parens expected dir missing at %s; run `make testdata`", upExpectedDir)
	}

	entries, err := os.ReadDir(upConfDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", upConfDir, err)
	}

	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		stem := strings.TrimSuffix(name, ".conf")
		confPath := filepath.Join(upConfDir, name)
		jsonPath := filepath.Join(upExpectedDir, stem+"-expected.json")

		if _, statErr := os.Stat(jsonPath); statErr != nil {
			t.Errorf("%s: expected JSON sidecar missing at %s (xx.hocon ships success fixtures with -expected.json; this is a fixture-author error or stale testdata fetch)", stem, jsonPath)
			continue
		}

		ran++
		t.Run(stem, func(t *testing.T) {
			cfg, parseErr := hocon.ParseFile(confPath)
			if parseErr != nil {
				t.Fatalf("ParseFile(%s): %v (parens in unquoted strings should be accepted per HOCON.md L274 — `(` and `)` are not in the forbidden set)", confPath, parseErr)
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
		t.Errorf("no unquoted-parens fixtures were tested; check %s and %s", upConfDir, upExpectedDir)
	}
}
