// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S13c — env-var list expansion: ${X[]} / ${?X[]} HOCON syntax.
// Spec authority: HOCON.md L893–L917 (list values from environment variables).
// Extra-spec conventions: E6 (config-defined wins), E7 (whitespace before []).
// Fixtures: testdata/hocon/env-var-list/ev01-ev11 (conf + env sidecars).
// Expected JSON: testdata/expected/env-var-list/ev01-ev11-expected.json.
//
// NOTE: no t.Parallel() in this file — t.Setenv mutates the process environment,
// which is incompatible with parallel test execution.

package hocon_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// s13cSuccessFixtures: parse must succeed and resolved JSON must match expected.
//
// Note on ev08-self-append: the spec originally classified this as a tripwire
// pending cluster 3f (S13a.13 self-ref-lookback), but multi-impl probe (ts/rs/go
// all pass naturally) confirmed the assumption was wrong — ev08's `x = ["x"]; x =
// ${?x} ${?LIST[]}` has a clear prior value for `x`, so it does not exercise the
// S13a.13 "no prior value" lookback gap. ev08 lives in SUCCESS in all 3 impls.
//
// ev12c-include-config-defined-wins: E6 cross-source fixture (xx.hocon#22).
// ${S13C_EV12C_X[]} in an included file; S13C_EV12C_X is defined in the parent
// config. No .env sidecar — routes through native Lightbend 1.4.6 path (config
// value wins before env-var list expansion is ever consulted).
var s13cSuccessFixtures = []string{
	"ev01-basic",
	"ev02-stops-at-gap",
	"ev04-optional-no-elements",
	"ev05-config-defined-wins",
	"ev06-concat-prepend",
	"ev07-concat-append",
	"ev08-self-append",
	"ev09-whitespace-before-suffix",
	"ev10-empty-string-element",
	"ev11-include-context",
	"ev12c-include-config-defined-wins",
}

// s13cErrorFixtures: parse/resolve must return a non-nil error.
var s13cErrorFixtures = []string{
	"ev03-required-no-elements",
}

// confPath_s13c returns the path to a fixture .conf file.
func confPath_s13c(name string) string {
	return filepath.Join("testdata", "hocon", "env-var-list", name+".conf")
}

// expectedPath_s13c returns the path to a fixture expected JSON file.
func expectedPath_s13c(name string) string {
	return filepath.Join("testdata", "expected", "env-var-list", name+"-expected.json")
}

// parseEnvSidecar reads a KEY=VALUE sidecar file and returns the key-value pairs.
// Rules:
//   - Lines are trimmed of leading/trailing whitespace.
//   - Empty lines and lines starting with '#' are skipped.
//   - Each line must contain '=' (else: error); the key is everything before the
//     first '=', the value is everything after (may be empty, may contain '=').
//   - Keys and values are used verbatim (no shell quoting / expansion).
func parseEnvSidecar(path string) (result map[string]string, err error) {
	f, err := os.Open(path)
	if err != nil {
		// A missing .env sidecar means "no env vars for this fixture" — treat as empty.
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	result = make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return nil, fmt.Errorf("malformed env sidecar %s line %d: missing '=': %q", path, lineNo, line)
		}
		key := line[:idx]
		val := line[idx+1:]
		result[key] = val
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, scanErr
	}
	return result, nil
}

// injectEnvSidecar loads and injects the .env sidecar for a fixture via t.Setenv.
// t.Setenv automatically restores each variable after the test completes.
func injectEnvSidecar(t *testing.T, name string) {
	t.Helper()
	sidecarPath := filepath.Join("testdata", "hocon", "env-var-list", name+".env")
	pairs, err := parseEnvSidecar(sidecarPath)
	if err != nil {
		t.Fatalf("parseEnvSidecar(%s): %v", sidecarPath, err)
	}
	for k, v := range pairs {
		t.Setenv(k, v)
	}
}

// TestS13c_SuccessFixtures runs all success fixtures end-to-end.
func TestS13c_SuccessFixtures(t *testing.T) {
	if _, err := os.Stat(filepath.Join("testdata", "expected", "env-var-list")); err != nil {
		t.Skip("env-var-list expected fixtures missing; run `make testdata`")
	}
	for _, name := range s13cSuccessFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			injectEnvSidecar(t, name)

			cfg, err := hocon.ParseFile(confPath_s13c(name))
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", name, err)
			}

			expectedData, err := os.ReadFile(expectedPath_s13c(name))
			if err != nil {
				t.Fatalf("ReadFile(expected, %s): %v", name, err)
			}
			var want any
			if err := json.Unmarshal(expectedData, &want); err != nil {
				t.Fatalf("parse expected JSON (%s): %v", name, err)
			}

			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", name, err)
			}

			if !jsonEqual(got, want) {
				gotJSON, _ := json.MarshalIndent(got, "", "  ")
				wantJSON, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("%s mismatch\ngot:\n%s\nwant:\n%s", name, gotJSON, wantJSON)
			}
		})
	}
}

// TestS13c_ErrorFixtures runs all error fixtures — parse/resolve must fail.
func TestS13c_ErrorFixtures(t *testing.T) {
	for _, name := range s13cErrorFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			injectEnvSidecar(t, name)

			_, err := hocon.ParseFile(confPath_s13c(name))
			if err == nil {
				t.Errorf("%s: expected parse/resolve error, got success", name)
			}
		})
	}
}
