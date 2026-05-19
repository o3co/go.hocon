// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S13a.13 — optional self-referential substitution look-back conformance tests.
// Spec authority: HOCON.md L837–L854 (self-referential substitutions).
// Fixtures: testdata/hocon/self-ref-lookback/sr01-sr11 (.conf files).
// Expected JSON: testdata/expected/self-ref-lookback/sr01-sr11-expected.json.
// Error fixture: testdata/expected/self-ref-lookback/sr05-required-no-prior.error.
//
// The core fix: `a = ${?a}foo` with no prior `a` must resolve to "foo", not
// "foofoo". The self-ref look-back must short-circuit to UNDEFINED when no
// prior value exists — not fall through and resolve the current concat.

package hocon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// s13a13SuccessFixtures lists fixtures that must parse and match expected JSON.
var s13a13SuccessFixtures = []string{
	"sr01-optional-no-prior",
	"sr02-optional-no-prior-leading",
	"sr03-optional-no-prior-both-sides",
	"sr04-optional-with-prior",
	"sr06-required-with-prior",
	"sr07-array-optional-no-prior",
	"sr08-array-optional-with-prior",
	"sr09-nested-no-prior",
	"sr10-nested-with-prior",
	"sr11-mutual-ref-forward",
}

// s13a13ErrorFixtures lists fixtures that must produce a non-nil error.
var s13a13ErrorFixtures = []string{
	"sr05-required-no-prior",
}

func confPath_s13a13(name string) string {
	return filepath.Join("testdata", "hocon", "self-ref-lookback", name+".conf")
}

func expectedPath_s13a13(name string) string {
	return filepath.Join("testdata", "expected", "self-ref-lookback", name+"-expected.json")
}

// TestS13a13_SelfRefLookback_Success asserts that each success fixture parses
// and resolves to the expected JSON value.
func TestS13a13_SelfRefLookback_Success(t *testing.T) {
	if _, err := os.Stat(filepath.Join("testdata", "expected", "self-ref-lookback")); err != nil {
		t.Skip("self-ref-lookback expected fixtures missing; run `make testdata`")
	}

	for _, name := range s13a13SuccessFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			confPath := confPath_s13a13(name)
			expectedPath := expectedPath_s13a13(name)

			cfg, err := hocon.ParseFile(confPath)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", confPath, err)
			}

			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", expectedPath, err)
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

// TestS13a13_SelfRefLookback_Errors asserts that each error fixture produces
// a non-nil error during parse or resolve.
func TestS13a13_SelfRefLookback_Errors(t *testing.T) {
	if _, err := os.Stat(filepath.Join("testdata", "expected", "self-ref-lookback")); err != nil {
		t.Skip("self-ref-lookback expected fixtures missing; run `make testdata`")
	}

	for _, name := range s13a13ErrorFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			confPath := confPath_s13a13(name)
			_, err := hocon.ParseFile(confPath)
			if err == nil {
				t.Errorf("expected error for %s but got success", confPath)
			}
		})
	}
}
