// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package parser_test — S12.5 include-reservation conformance tests (Phase 6 #3e).
// Drives xx.hocon fixtures from testdata/hocon/include-reservation/ against the
// parser and asserts error/success per testdata/expected/include-reservation/.
//
// Convention:
//   - testdata/expected/include-reservation/<name>.error  → fixture must parse-error
//   - testdata/expected/include-reservation/<name>-expected.json → fixture must parse OK
//     (value equality is exercised by resolver-layer lightbend tests, not here)
//   - implErrors: fixtures where xx.hocon has no sidecar (Lightbend silently accepts)
//     but go.hocon enforces a parse error per strict HOCON.md L570 posture.
//   - fixtures with no sidecar and not in implErrors → skipped.
package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
)

const (
	irConfDir     = "../../testdata/hocon/include-reservation"
	irExpectedDir = "../../testdata/expected/include-reservation"
)

// implErrors lists fixtures that xx.hocon classifies as Lightbend-quirk
// silent-accept (no .error sidecar) but go.hocon enforces as parse errors per
// HOCON.md L570 strict-spec posture.
var implErrors = map[string]bool{
	"ir03-include-dot-foo-equals": true,
	"ir04-include-nested-object":  true,
}

// TestIncludeReservationFixtures drives all ir01–ir14 fixtures and asserts:
//   - fixtures with a .error sidecar (or in implErrors) must produce a parse error.
//   - fixtures with a -expected.json sidecar must parse without error.
//   - fixtures with no sidecar and not in implErrors are skipped.
func TestIncludeReservationFixtures(t *testing.T) {
	if _, err := os.Stat(irExpectedDir); os.IsNotExist(err) {
		t.Skipf("include-reservation expected dir not found — run `make testdata` first: %s", irExpectedDir)
		return
	}
	if _, err := os.Stat(irConfDir); os.IsNotExist(err) {
		t.Skipf("include-reservation conf dir not found at %s", irConfDir)
		return
	}

	entries, err := os.ReadDir(irConfDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", irConfDir, err)
	}

	ran := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".conf") {
			continue
		}
		stem := strings.TrimSuffix(name, ".conf")
		confPath := filepath.Join(irConfDir, name)

		// Skip inner files (included by ir05/ir09 include statements).
		if strings.HasSuffix(stem, "-inner") {
			continue
		}

		// Determine expected outcome.
		errorSidecar := filepath.Join(irExpectedDir, stem+".error")
		jsonSidecar := filepath.Join(irExpectedDir, stem+"-expected.json")

		_, hasErrorSidecar := os.Stat(errorSidecar)
		_, hasJSON := os.Stat(jsonSidecar)

		isImplError := implErrors[stem]

		switch {
		case hasErrorSidecar == nil || isImplError:
			// Fixture must produce a parse error.
			ran++
			stem := stem
			confPath := confPath
			t.Run(stem+"/must-parse-error", func(t *testing.T) {
				data, readErr := os.ReadFile(confPath)
				if readErr != nil {
					t.Fatalf("ReadFile(%s): %v", confPath, readErr)
				}
				_, parseErr := parser.Parse(string(data))
				if parseErr == nil {
					if isImplError {
						t.Errorf("%s: expected parse error (go.hocon strict S12.5, HOCON.md L570), got nil", stem)
					} else {
						t.Errorf("%s: expected parse error (per .error sidecar), got nil", stem)
					}
				}
			})
		case hasJSON == nil:
			// Fixture must parse without error.
			// For ir05/ir09 (include statements), we only assert no parse error;
			// file resolution requires the resolver layer tested elsewhere.
			ran++
			stem := stem
			confPath := confPath
			t.Run(stem+"/must-parse-ok", func(t *testing.T) {
				data, readErr := os.ReadFile(confPath)
				if readErr != nil {
					t.Fatalf("ReadFile(%s): %v", confPath, readErr)
				}
				if _, parseErr := parser.Parse(string(data)); parseErr != nil {
					t.Errorf("%s: expected no parse error, got: %v", stem, parseErr)
				}
			})
		default:
			// No sidecar and not an implError — skip.
			stem := stem
			t.Run(stem+"/no-sidecar-skip", func(t *testing.T) {
				t.Skipf("%s: no expected sidecar in %s; skipping fixture-driven test", stem, irExpectedDir)
			})
		}
	}

	if ran == 0 {
		t.Errorf("no include-reservation fixtures were tested; check %s and %s", irConfDir, irExpectedDir)
	}
}
