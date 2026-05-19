// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S3.1 — empty file is an invalid HOCON document conformance tests.
// Spec authority: HOCON.md L130 ("Empty files are invalid documents").
// Fixtures: testdata/hocon/empty-file/ef01-ef06 (from xx.hocon, SHA 5beedfa).
//
// All six fixtures must produce a non-nil parse error; no expected JSON sidecar
// is needed for error cases. The test gate checks for the .conf fixtures directly.
//
// Positive guards: "{}", "a = 1", and "# comment\na = 1" must succeed.

package hocon_test

import (
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// s3_1ErrorFixtures lists the names of empty-file error fixtures (without extension).
var s3_1ErrorFixtures = []string{
	"ef01-empty",
	"ef02-whitespace-only",
	"ef03-newlines-only",
	"ef04-comment-only",
	"ef05-bom-only",
	"ef06-mixed-ws-comment",
}

// TestS3_1_EmptyFile_Error asserts that each empty-file fixture produces a non-nil
// parse or resolve error (HOCON.md L130: empty files are invalid documents).
func TestS3_1_EmptyFile_Error(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "hocon", "empty-file")
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Skipf("empty-file fixtures missing at %s; run `make testdata`", fixtureDir)
	}

	for _, name := range s3_1ErrorFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			confPath := filepath.Join(fixtureDir, name+".conf")
			_, err := hocon.ParseFile(confPath)
			if err == nil {
				t.Errorf("ParseFile(%s): expected error for empty/comment-only input, got nil", confPath)
			}
		})
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
