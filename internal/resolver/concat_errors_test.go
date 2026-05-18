// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package resolver_test — S10 concat-errors conformance tests (Phase 6 #3b).
// Drives xx.hocon fixtures from testdata/hocon/concat-errors/ against the
// resolver and asserts error/success per testdata/expected/concat-errors/.
//
// Convention:
//   - testdata/expected/concat-errors/<name>.error  → fixture must resolve-error
//   - testdata/expected/concat-errors/<name>-expected.json → fixture must succeed;
//     not validated for value equality here (that is covered by lightbend_test.go
//     TestLightbendExpected patterns); we only assert no error.
//   - ce05: no expected file present in xx.hocon — skipped by fixture-driven test;
//     covered by TestSpecS10_13_ObjectPlusScalarErrors unit test.
package resolver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/resolver"
)

const (
	concatErrorsConfDir     = "../../testdata/hocon/concat-errors"
	concatErrorsExpectedDir = "../../testdata/expected/concat-errors"
)

// resolveErrFromFile parses and resolves the given .conf file path, returning
// the result and any error.
func resolveErrFromFile(t *testing.T, confPath string) (*resolver.Result, error) {
	t.Helper()
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", confPath, err)
	}
	ast, err := parser.Parse(string(data))
	if err != nil {
		return nil, err
	}
	return resolver.Resolve(ast, resolver.Options{})
}

// TestConcatErrorsFixtures drives all ce01-ce15 fixtures and asserts:
//   - fixtures with a .error sidecar in expected/concat-errors/ must produce a
//     resolve or parse error.
//   - fixtures with a -expected.json sidecar must resolve without error.
//   - fixtures with no sidecar at all are skipped (currently only ce05).
func TestConcatErrorsFixtures(t *testing.T) {
	if _, err := os.Stat(concatErrorsExpectedDir); os.IsNotExist(err) {
		t.Skipf("concat-errors expected dir not found — run `make testdata` first: %s", concatErrorsExpectedDir)
		return
	}
	if _, err := os.Stat(concatErrorsConfDir); os.IsNotExist(err) {
		t.Skipf("concat-errors conf dir not found at %s", concatErrorsConfDir)
		return
	}

	entries, err := os.ReadDir(concatErrorsConfDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", concatErrorsConfDir, err)
	}

	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		stem := strings.TrimSuffix(name, ".conf")
		confPath := filepath.Join(concatErrorsConfDir, name)

		// Determine expected outcome from sidecar files.
		errorSidecar := filepath.Join(concatErrorsExpectedDir, stem+".error")
		jsonSidecar := filepath.Join(concatErrorsExpectedDir, stem+"-expected.json")

		_, hasError := os.Stat(errorSidecar)
		_, hasJSON := os.Stat(jsonSidecar)

		switch {
		case hasError == nil:
			// Fixture must error.
			ran++
			stem := stem // capture for closure
			t.Run(stem+"/must-error", func(t *testing.T) {
				_, err := resolveErrFromFile(t, confPath)
				if err == nil {
					t.Errorf("%s: expected resolve error (spec S10.4/S10.13/S10.19), got nil", stem)
				}
			})
		case hasJSON == nil:
			// Fixture must succeed.
			ran++
			stem := stem
			t.Run(stem+"/must-succeed", func(t *testing.T) {
				if _, err := resolveErrFromFile(t, confPath); err != nil {
					t.Errorf("%s: expected success, got error: %v", stem, err)
				}
			})
		default:
			// No sidecar — skip.
			t.Run(stem+"/no-sidecar-skip", func(t *testing.T) {
				t.Skipf("%s: no expected sidecar in %s; skipping fixture-driven test", stem, concatErrorsExpectedDir)
			})
		}
	}

	if ran == 0 {
		t.Errorf("no concat-errors fixtures were tested; check %s and %s", concatErrorsConfDir, concatErrorsExpectedDir)
	}
}
