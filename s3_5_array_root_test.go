// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S3.5 — an array-root document ([1,2]) is syntactically valid HOCON
// (HOCON.md L989-991: "both JSON and HOCON allow arrays as root values in a
// document"), but the object-rooted Config API rejects it with a TYPE error
// after a successful syntax parse. Reference: Lightbend parses the document,
// then Parseable.forceParsedToObject throws ConfigException.WrongType "has
// type LIST rather than object at file root". The former behavior — a parse
// error "expected key" — was the right net outcome (reject) as the wrong kind
// of error at the wrong layer.
//
// S14b.1 (HOCON.md L993-994): an INCLUDED file with an array root is invalid;
// the error names the included file.
//
// Fixtures: xx.hocon array-root/ar01-ar03 with .error sidecars (see
// TestS3_5_ArrayRoot_Conformance for the fixture loop).

package hocon_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// requireConfigError asserts err is a *hocon.ConfigError (the type-mismatch
// class — Lightbend WrongType analog) and NOT a *hocon.ParseError.
func requireConfigError(t *testing.T, err error, label string) *hocon.ConfigError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected ConfigError, got nil", label)
	}
	var pe *hocon.ParseError
	if errors.As(err, &pe) {
		t.Fatalf("%s: got ParseError %q; array-root documents are valid syntax — expected the type-error class", label, pe.Message)
	}
	var ce *hocon.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("%s: expected *hocon.ConfigError, got %T: %v", label, err, err)
	}
	if !strings.Contains(ce.Message, "array rather than object at file root") {
		t.Errorf("%s: message must name the array-at-file-root condition, got: %s", label, ce.Message)
	}
	return ce
}

func TestS3_5_ArrayRoot_TopLevelIsTypeError(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"basic", "[1,2]"},
		{"multiline-objects", "[\n  { a : 1 },\n  { b : 2 }\n]\n"},
		{"empty-array", "[]"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := hocon.ParseString(tc.src)
			_ = requireConfigError(t, err, tc.name)
		})
	}
}

func TestS3_5_ArrayRoot_ErrorCarriesPosition(t *testing.T) {
	_, err := hocon.ParseString("\n  [1,2]")
	ce := requireConfigError(t, err, "position")
	if !strings.Contains(ce.Message, "2:3") {
		t.Errorf("message must carry the opening bracket position 2:3, got: %s", ce.Message)
	}
}

func TestS3_5_ArrayRoot_DeferredLifecycle(t *testing.T) {
	_, err := hocon.ParseStringWithOptions("[1,2]", hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	_ = requireConfigError(t, err, "deferred")
}

func TestS3_5_MalformedArraysStaySyntaxErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"unterminated", "[1,2"},
		{"trailing-content", "[1,2]\na = 1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := hocon.ParseString(tc.src)
			var pe *hocon.ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("%s: expected *hocon.ParseError (syntax), got %T: %v", tc.name, err, err)
			}
		})
	}
}

func TestS3_5_IncludeOfArrayRootNamesIncludedFile(t *testing.T) {
	dir := t.TempDir()
	arrFile := filepath.Join(dir, "arr.conf")
	if err := os.WriteFile(arrFile, []byte("[1,2]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashArr := strings.ReplaceAll(arrFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashArr)), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := hocon.ParseFile(mainFile)
	if err == nil {
		t.Fatal("expected error for include of array-root file (HOCON.md L993-994), got nil")
	}
	var re *hocon.ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *hocon.ResolveError, got %T: %v", err, err)
	}
	if !strings.Contains(re.Message, "array at file root") {
		t.Errorf("message must name the array-at-file-root condition, got: %s", re.Message)
	}
	// The included-source identity is carried in FilePath (rendered by
	// Error()), not embedded in Message.
	if !strings.Contains(re.FilePath, "arr.conf") {
		t.Errorf("FilePath must name the included file, got: %q", re.FilePath)
	}
	if !strings.Contains(err.Error(), "arr.conf") {
		t.Errorf("rendered error must name the included file, got: %v", err)
	}
}

func TestS3_5_PackageIncludeOfArrayRootIsError(t *testing.T) {
	// Global registry isolation (same idiom as e11_include_package_test.go).
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)
	if err := hocon.RegisterPackage("test/s3-5-array-root", "ref.conf", []byte("[1,2]\n")); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := hocon.ParseString("include package(\"test/s3-5-array-root\", \"ref.conf\")\na = 1")
	if err == nil {
		t.Fatal("expected error for package include of array-root content, got nil")
	}
	var re *hocon.ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *hocon.ResolveError, got %T: %v", err, err)
	}
	if !strings.Contains(re.Message, "array at file root") {
		t.Errorf("message must name the array-at-file-root condition, got: %s", re.Message)
	}
}

func TestS3_5_NonRootArraysUnaffected(t *testing.T) {
	cfg, err := hocon.ParseString("a = [1,2]")
	if err != nil {
		t.Fatalf("field-value array: %v", err)
	}
	if got := cfg.GetIntSlice("a"); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("a = %v, want [1 2]", got)
	}
	if _, err := hocon.ParseString("{ a = [1,2] }"); err != nil {
		t.Errorf("braced root with array field: %v", err)
	}
}

// TestS3_5_ArrayRoot_Conformance loops the xx.hocon array-root fixtures
// (ar01-ar03, .error sidecars — Lightbend WrongType ground truth).
// ar03-inner.conf is a sibling include-target, not a standalone fixture.
func TestS3_5_ArrayRoot_Conformance(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "hocon", "array-root")
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Skipf("array-root fixtures missing at %s; run `make testdata` (requires xx.hocon#64)", fixtureDir)
	}
	entries, err := filepath.Glob(filepath.Join(fixtureDir, "ar*.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, conf := range entries {
		conf := conf
		name := filepath.Base(conf)
		if strings.HasSuffix(name, "-inner.conf") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			_, err := hocon.ParseFile(conf)
			if err == nil {
				t.Fatalf("%s: expected array-at-file-root error, got nil", name)
			}
			if !strings.Contains(err.Error(), "array") || !strings.Contains(err.Error(), "file root") {
				t.Errorf("%s: error must name the array-at-file-root condition, got: %v", name, err)
			}
		})
	}
}

// TestS3_5_NestedIncludeNamesInnermostFile pins that a nested include chain
// (parent -> mid -> arr) names the innermost file that actually has the array
// root — not an intermediate file. Regression guard for the conversion living
// at the ParseBytes site rather than in loadIncludeFile (where errors.Is
// re-fired at each level and re-wrapped with the outer file's path).
func TestS3_5_NestedIncludeNamesInnermostFile(t *testing.T) {
	dir := t.TempDir()
	arrFile := filepath.Join(dir, "arr.conf")
	if err := os.WriteFile(arrFile, []byte("[1,2]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	midFile := filepath.Join(dir, "mid.conf")
	if err := os.WriteFile(midFile, []byte("include \"arr.conf\"\nb = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	if err := os.WriteFile(mainFile, []byte("include \"mid.conf\"\na = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := hocon.ParseFile(mainFile)
	if err == nil {
		t.Fatal("expected array-at-file-root error through the nested chain, got nil")
	}
	var re *hocon.ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *hocon.ResolveError, got %T: %v", err, err)
	}
	if !strings.Contains(re.FilePath, "arr.conf") {
		t.Errorf("FilePath must name the innermost file arr.conf, got: %q", re.FilePath)
	}
	if strings.Contains(re.FilePath, "mid.conf") || strings.Contains(re.Message, "mid.conf") {
		t.Errorf("error must not accuse the intermediate file mid.conf, got: %v", re)
	}
}

// TestS3_5_FileIncludesPackageWithArrayRoot pins the file -> package nesting
// direction: the error names the package virtual path, not the including file.
func TestS3_5_FileIncludesPackageWithArrayRoot(t *testing.T) {
	// Global registry isolation (same idiom as e11_include_package_test.go).
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)
	if err := hocon.RegisterPackage("test/s3-5-nested-pkg", "ref.conf", []byte("[1,2]\n")); err != nil {
		t.Fatalf("register: %v", err)
	}
	dir := t.TempDir()
	midFile := filepath.Join(dir, "mid.conf")
	if err := os.WriteFile(midFile, []byte("include package(\"test/s3-5-nested-pkg\", \"ref.conf\")\nb = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	if err := os.WriteFile(mainFile, []byte("include \"mid.conf\"\na = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := hocon.ParseFile(mainFile)
	if err == nil {
		t.Fatal("expected array-at-file-root error through the file->package chain, got nil")
	}
	var re *hocon.ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *hocon.ResolveError, got %T: %v", err, err)
	}
	if !strings.Contains(re.FilePath, "test/s3-5-nested-pkg") {
		t.Errorf("FilePath must name the package virtual path, got: %q", re.FilePath)
	}
	if strings.Contains(re.FilePath, "mid.conf") || strings.Contains(re.Message, "mid.conf") {
		t.Errorf("error must not accuse the including file mid.conf, got: %v", re)
	}
}
