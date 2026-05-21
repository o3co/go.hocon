// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package hocon_test — E11 include package(...) conformance tests.
//
// Drives all 14 ipk* fixtures from testdata/hocon/include-package/ and
// asserts expected outcomes (success, parse error, registration error, cycle error).
//
// Per-impl override: ipk03 (registration collision) is go.hocon-specific; the
// implErrors map below marks it as go.hocon-enforced-error (no Lightbend sidecar).
//
// No Lightbend .error sidecars exist for E11 fixtures (Lightbend has no package(...)
// concept). All outcomes are asserted directly in this file.
package hocon_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

const (
	ipkConfDir = "testdata/hocon/include-package"
	ipkPkgDir  = "testdata/hocon/include-package/_packages"
)

// ipkOutcome classifies the expected test outcome.
type ipkOutcome int

const (
	ipkSuccess           ipkOutcome = iota // parse succeeds; optional value check
	ipkParseError                          // parse or resolve error
	ipkRegistrationError                   // error at registration time (before parse)
	ipkCycleError                          // circular include error (subtype of parse error)
)

// ipkFixture describes a single fixture's expected behavior.
type ipkFixture struct {
	name       string
	outcome    ipkOutcome
	setup      func(t *testing.T) // optional registration setup (run before parse)
	wantResult map[string]any    // for ipkSuccess fixtures — nil means don't check
}

// readPkg reads a package fixture file and returns its bytes.
func readPkg(t *testing.T, relPath string) []byte {
	t.Helper()
	full := filepath.Join(ipkPkgDir, relPath)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("readPkg(%s): %v", relPath, err)
	}
	return data
}

// ipkFixtures defines all 14 E11 fixtures with their per-impl setup and expectations.
func ipkFixtures(t *testing.T) []ipkFixture {
	t.Helper()
	return []ipkFixture{
		{
			name:    "ipk01-basic",
			outcome: ipkSuccess,
			setup: func(t *testing.T) {
				t.Helper()
				content := readPkg(t, "github.com_example_lib/reference.conf")
				if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", content); err != nil {
					t.Fatalf("ipk01 setup: %v", err)
				}
			},
			wantResult: map[string]any{
				"host": "example.com",
				"port": float64(8080),
				"app":  map[string]any{"name": "lib"},
			},
		},
		{
			name:    "ipk02-one-arg-rejected",
			outcome: ipkParseError,
		},
		{
			name:    "ipk03-collision",
			outcome: ipkRegistrationError,
			setup: func(t *testing.T) {
				t.Helper()
				contentA := readPkg(t, "_ipk03-pkg-A.conf")
				if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", contentA); err != nil {
					t.Fatalf("ipk03 setup A: %v", err)
				}
				contentB := readPkg(t, "_ipk03-pkg-B.conf")
				err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", contentB)
				if err == nil {
					t.Fatalf("ipk03: expected collision error, got nil")
				}
				var colErr *hocon.PackageCollisionError
				if !errors.As(err, &colErr) {
					t.Fatalf("ipk03: expected *PackageCollisionError, got %T: %v", err, err)
				}
				// Registration error verified. Signal test done via t.Log (parse in fixture is still valid).
				t.Log("ipk03: collision error verified at registration time")
			},
		},
		{
			name:    "ipk04-lookup-miss",
			outcome: ipkParseError,
			// No setup — empty registry. PackageLookup will miss.
		},
		{
			name:    "ipk05-required-miss",
			outcome: ipkParseError,
			// No setup — empty registry.
		},
		{
			name:    "ipk06-byte-exact-id-case",
			outcome: ipkParseError,
			setup: func(t *testing.T) {
				t.Helper()
				// Register with uppercase "Foo/Bar" — fixture uses lowercase "foo/bar" → miss
				content := readPkg(t, "github.com_example_lib_byte/Foo_Bar_x.conf")
				if err := hocon.RegisterPackage("Foo/Bar", "x.conf", content); err != nil {
					t.Fatalf("ipk06 setup: %v", err)
				}
			},
		},
		{
			name:    "ipk07-byte-exact-file-case",
			outcome: ipkParseError,
			setup: func(t *testing.T) {
				t.Helper()
				// Register with uppercase "Reference.conf" — fixture uses lowercase → miss
				content := readPkg(t, "github.com_example_lib_byte/github.com_example_lib_Reference.conf")
				if err := hocon.RegisterPackage("github.com/example/lib", "Reference.conf", content); err != nil {
					t.Fatalf("ipk07 setup: %v", err)
				}
			},
		},
		{
			name:    "ipk08-empty-content",
			outcome: ipkSuccess,
			setup: func(t *testing.T) {
				t.Helper()
				// Register empty content
				emptyContent := readPkg(t, "github.com_example_lib_empty/empty.conf")
				if err := hocon.RegisterPackage("github.com/example/lib", "empty.conf", emptyContent); err != nil {
					t.Fatalf("ipk08 setup: %v", err)
				}
			},
			wantResult: map[string]any{
				"app": "host",
			},
		},
		{
			name:    "ipk09-file-empty",
			outcome: ipkParseError,
		},
		{
			name:    "ipk10-file-absolute",
			outcome: ipkParseError,
		},
		{
			name:    "ipk11-file-traversal",
			outcome: ipkParseError,
		},
		{
			name:    "ipk12-file-backslash",
			outcome: ipkParseError,
		},
		{
			name:    "ipk13-cycle-self",
			outcome: ipkCycleError,
			setup: func(t *testing.T) {
				t.Helper()
				content := readPkg(t, "_cycle/ipk13-self.conf")
				if err := hocon.RegisterPackage("foo", "self.conf", content); err != nil {
					t.Fatalf("ipk13 setup: %v", err)
				}
			},
		},
		{
			name:    "ipk14-cycle-mutual",
			outcome: ipkCycleError,
			setup: func(t *testing.T) {
				t.Helper()
				contentA := readPkg(t, "_cycle/ipk14-a.conf")
				if err := hocon.RegisterPackage("foo", "a.conf", contentA); err != nil {
					t.Fatalf("ipk14 setup a: %v", err)
				}
				contentB := readPkg(t, "_cycle/ipk14-b.conf")
				if err := hocon.RegisterPackage("foo", "b.conf", contentB); err != nil {
					t.Fatalf("ipk14 setup b: %v", err)
				}
			},
		},
	}
}

// TestE11IncludePackageConformance drives all 14 ipk* fixtures.
func TestE11IncludePackageConformance(t *testing.T) {
	if _, err := os.Stat(ipkConfDir); os.IsNotExist(err) {
		t.Skipf("include-package fixtures not found at %s", ipkConfDir)
		return
	}

	fixtures := ipkFixtures(t)

	for _, fix := range fixtures {
		fix := fix
		t.Run(fix.name, func(t *testing.T) {
			// Reset registry before each sub-test for isolation.
			hocon.ResetPackageRegistry()
			t.Cleanup(hocon.ResetPackageRegistry)

			// Run setup (registration).
			if fix.setup != nil {
				fix.setup(t)
			}

			confPath := filepath.Join(ipkConfDir, fix.name+".conf")
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("fixture file not found: %s", confPath)
				return
			}

			switch fix.outcome {
			case ipkRegistrationError:
				// ipk03: registration error is asserted inside setup; nothing more to do.
				// The fixture .conf itself can be parsed or not — we don't assert on it.
				// The test passes because setup already verified the collision error.
				t.Log("registration error verified in setup; fixture parse not asserted")

			case ipkParseError, ipkCycleError:
				cfg, err := hocon.ParseFile(confPath)
				if err == nil {
					t.Errorf("%s: expected error (outcome=%v), got success (cfg=%v)", fix.name, fix.outcome, cfg)
					return
				}
				if fix.outcome == ipkCycleError {
					// Cycle errors should mention "circular" in the message.
					if !strings.Contains(err.Error(), "circular") {
						t.Errorf("%s: expected error mentioning 'circular', got: %v", fix.name, err)
					}
				}

			case ipkSuccess:
				cfg, err := hocon.ParseFile(confPath)
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", fix.name, err)
				}
				if fix.wantResult != nil {
					got := make(map[string]any)
					if err := cfg.Unmarshal(&got); err != nil {
						t.Fatalf("%s: Unmarshal: %v", fix.name, err)
					}
					wantJSON, _ := json.MarshalIndent(fix.wantResult, "", "  ")
					gotJSON, _ := json.MarshalIndent(normalizeForJSON(got), "", "  ")
					wantNorm, _ := json.MarshalIndent(normalizeForJSON(fix.wantResult), "", "  ")
					if string(gotJSON) != string(wantNorm) {
						t.Errorf("%s: result mismatch\ngot:\n%s\nwant:\n%s", fix.name, gotJSON, wantJSON)
					}
				}
			}
		})
	}
}
