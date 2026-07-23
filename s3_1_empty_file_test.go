// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S3.1 — an empty document is valid HOCON and parses to the empty object {}.
// Spec authority: HOCON.md §Omit root braces L134-136 (a file that does not
// begin with `[` or `{` is parsed as if enclosed in `{}`; an empty document
// vacuously qualifies). L130-132 ("Empty files are invalid documents") is the
// JSON baseline, not HOCON-normative — the former reject-posture was revoked
// 2026-07-23 (xx.hocon E10). Confirmed by the Lightbend reference
// implementation, whose "Empty document" error is ConfigSyntax.JSON-only.
// Fixtures: testdata/hocon/empty-file/ef01-ef06 (from xx.hocon); the {}
// -expected.json sidecars are normative — no per-impl override applies.
//
// Positive guards: "{}", "a = 1", and "# comment\na = 1" must also succeed.

package hocon_test

import (
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// s3_1EmptyFixtures lists the names of empty-file fixtures (without extension).
var s3_1EmptyFixtures = []string{
	"ef01-empty",
	"ef02-whitespace-only",
	"ef03-newlines-only",
	"ef04-comment-only",
	"ef05-bom-only",
	"ef06-mixed-ws-comment",
}

// TestS3_1_EmptyFile_ParsesToEmptyObject asserts that each empty-file fixture
// parses successfully to an empty config, matching the {} expected sidecars.
func TestS3_1_EmptyFile_ParsesToEmptyObject(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "hocon", "empty-file")
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Skipf("empty-file fixtures missing at %s; run `make testdata`", fixtureDir)
	}

	for _, name := range s3_1EmptyFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			confPath := filepath.Join(fixtureDir, name+".conf")
			cfg, err := hocon.ParseFile(confPath)
			if err != nil {
				t.Fatalf("ParseFile(%s): expected empty config per corrected S3.1, got error: %v", confPath, err)
			}
			if keys := cfg.Keys(); len(keys) != 0 {
				t.Errorf("ParseFile(%s): expected empty config, got keys %v", confPath, keys)
			}
		})
	}
}

// TestS3_1_EmptyDocument_DeferredParse pins the deferred-resolution path
// (WithResolveSubstitutions(false)): an empty document parses to an empty
// unresolved Config the same way the default path does — the parser is the
// single gate for the corrected S3.1 rule.
func TestS3_1_EmptyDocument_DeferredParse(t *testing.T) {
	cfg, err := hocon.ParseStringWithOptions("", hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("deferred ParseStringWithOptions(\"\"): expected empty config, got error: %v", err)
	}
	if keys := cfg.Keys(); len(keys) != 0 {
		t.Errorf("deferred parse of empty document: expected no keys, got %v", keys)
	}
}

// TestS3_1_BlockCommentOnlyTopLevelIsRejected: HOCON recognises only `#` and
// `//` comments. A `/* ... */`-only document is a syntax error, not an empty
// document — the S3.1 empty-parses-to-{} rule must not mask malformed files.
// (The include-path variant is pinned in issue105_test.go.)
func TestS3_1_BlockCommentOnlyTopLevelIsRejected(t *testing.T) {
	if _, err := hocon.ParseString("/* multi\nline\ncomment */\n"); err == nil {
		t.Error("ParseString on block-comment-only input succeeded; expected syntax error")
	}
}

// TestS3_1_NonEmpty_Accepted are positive guards: these must parse without error.
func TestS3_1_NonEmpty_Accepted(t *testing.T) {
	cases := []struct {
		label string
		src   string
	}{
		{"explicit-empty-object", "{}"},
		{"single-field", "a = 1"},
		{"comment-then-field", "# a comment\na = 1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			if _, err := hocon.ParseString(tc.src); err != nil {
				t.Errorf("ParseString(%q): expected success, got error: %v", tc.src, err)
			}
		})
	}
}
