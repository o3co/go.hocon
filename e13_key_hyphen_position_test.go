// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// E13 key-position S8.6 conformance (xx.hocon#42) — kh01–kh08 fixtures.
//
// HOCON.md L270-276 (S8.6) forbids unquoted strings from BEGINNING with `-`
// (unless followed by a digit). That rule is value-position only: Lightbend's
// path parser accepts hyphen-start segments verbatim in field-key position.
// go.hocon previously over-enforced S8.6 on every dot-split key segment.
// See xx.hocon docs/extra-spec-conventions.md E13.
//
// Fixtures: testdata/hocon/key-hyphen-position/kh01-kh08.conf
// Expected: testdata/expected/key-hyphen-position/kh*-expected.json

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
	khConfDir     = "testdata/hocon/key-hyphen-position"
	khExpectedDir = "testdata/expected/key-hyphen-position"
)

func TestE13_KeyHyphenPosition(t *testing.T) {
	if _, err := os.Stat(khConfDir); os.IsNotExist(err) {
		t.Skipf("key-hyphen-position fixtures missing at %s; run `make testdata`", khConfDir)
	}
	if _, err := os.Stat(khExpectedDir); os.IsNotExist(err) {
		t.Skipf("key-hyphen-position expected dir missing at %s; run `make testdata`", khExpectedDir)
	}

	entries, err := os.ReadDir(khConfDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", khConfDir, err)
	}

	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		stem := strings.TrimSuffix(name, ".conf")
		confPath := filepath.Join(khConfDir, name)
		jsonPath := filepath.Join(khExpectedDir, stem+"-expected.json")

		if _, statErr := os.Stat(jsonPath); statErr != nil {
			t.Errorf("%s: expected JSON sidecar missing at %s (E13 ships kh* as success fixtures with -expected.json)", stem, jsonPath)
			continue
		}

		ran++
		t.Run(stem, func(t *testing.T) {
			cfg, parseErr := hocon.ParseFile(confPath)
			if parseErr != nil {
				t.Fatalf("ParseFile(%s): %v\n  go.hocon must accept hyphen-start segments in key position per E13 (xx.hocon#42). HOCON.md L270-276 S8.6 is value-position only.", confPath, parseErr)
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
		t.Errorf("no kh* fixtures executed; check %s and %s", khConfDir, khExpectedDir)
	}
}
