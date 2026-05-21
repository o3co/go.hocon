// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package parser_test — E11 include package(...) parser-level tests (Phase 6 #E11).
// Tests the parser's ability to recognize and validate the package(...) qualifier.
package parser_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
)

// TestParseIncludePackageBasic verifies the parser correctly builds an IncludeNode
// with IsPackage=true and the correct PkgID/PkgFile when given a well-formed
// include package("id", "file") directive.
func TestParseIncludePackageBasic(t *testing.T) {
	src := `include package("github.com/example/lib", "reference.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(ast.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(ast.Fields))
	}
	inc, ok := ast.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", ast.Fields[0].Value)
	}
	if !inc.IsPackage {
		t.Error("IsPackage should be true")
	}
	if inc.PkgID != "github.com/example/lib" {
		t.Errorf("PkgID: want %q, got %q", "github.com/example/lib", inc.PkgID)
	}
	if inc.PkgFile != "reference.conf" {
		t.Errorf("PkgFile: want %q, got %q", "reference.conf", inc.PkgFile)
	}
	if inc.Required {
		t.Error("Required should be false for bare include")
	}
}

// TestParseIncludePackageRequired verifies include required(package(...)) sets Required=true.
func TestParseIncludePackageRequired(t *testing.T) {
	src := `include required(package("github.com/example/lib", "reference.conf"))`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	inc, ok := ast.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", ast.Fields[0].Value)
	}
	if !inc.IsPackage {
		t.Error("IsPackage should be true")
	}
	if !inc.Required {
		t.Error("Required should be true for required(package(...))")
	}
	if inc.PkgID != "github.com/example/lib" {
		t.Errorf("PkgID: want %q, got %q", "github.com/example/lib", inc.PkgID)
	}
}

// TestParseIncludePackageOneArgRejected verifies that one-arg form is rejected (E11 decision 2).
func TestParseIncludePackageOneArgRejected(t *testing.T) {
	src := `include package("github.com/example/lib/reference.conf")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for one-arg package(...), got nil")
	}
}

// TestParseIncludePackageFileEmpty verifies that empty file arg is rejected (E11 decision 6).
func TestParseIncludePackageFileEmpty(t *testing.T) {
	src := `include package("foo", "")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for empty file arg, got nil")
	}
}

// TestParseIncludePackageFileAbsolute verifies that absolute path file arg is rejected (E11 decision 6).
func TestParseIncludePackageFileAbsolute(t *testing.T) {
	src := `include package("foo", "/etc/passwd")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for absolute file arg, got nil")
	}
}

// TestParseIncludePackageFileTraversal verifies that .. segments are rejected (E11 decision 6).
func TestParseIncludePackageFileTraversal(t *testing.T) {
	src := `include package("foo", "../escape.conf")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for .. traversal, got nil")
	}
}

// TestParseIncludePackageFileDotSegment verifies that . segments are rejected (E11 decision 6).
func TestParseIncludePackageFileDotSegment(t *testing.T) {
	src := `include package("foo", "./x.conf")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for . segment, got nil")
	}
}

// TestParseIncludePackageFileBackslash verifies that backslash is rejected after unescaping (E11 decision 6).
// ipk12: "x\\y.conf" after HOCON unescape → "x\y.conf" (one backslash) → rejected.
func TestParseIncludePackageFileBackslash(t *testing.T) {
	src := `include package("foo", "x\\y.conf")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for backslash in file arg, got nil")
	}
}

// TestParseIncludePackageFileConsecutiveSlashes verifies that // is rejected (E11 decision 6).
func TestParseIncludePackageFileConsecutiveSlashes(t *testing.T) {
	src := `include package("foo", "a//b.conf")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for consecutive slashes, got nil")
	}
}

// TestParseIncludePackageFileSubdir verifies that a valid subdir path is accepted.
func TestParseIncludePackageFileSubdir(t *testing.T) {
	src := `include package("foo", "conf/reference.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	inc, ok := ast.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", ast.Fields[0].Value)
	}
	if inc.PkgFile != "conf/reference.conf" {
		t.Errorf("PkgFile: want %q, got %q", "conf/reference.conf", inc.PkgFile)
	}
}

// TestValidatePackageFile exercises the exported validation function directly.
func TestValidatePackageFile(t *testing.T) {
	cases := []struct {
		file    string
		wantErr bool
	}{
		{"reference.conf", false},
		{"conf/reference.conf", false},
		{"subdir/nested/x.conf", false},
		{"", true},
		{"/abs/path.conf", true},
		{"../escape.conf", true},
		{"./x.conf", true},
		{"conf\\x.conf", true},
		{"conf//x.conf", true},
		{"conf/./x.conf", true},
	}
	for _, tc := range cases {
		err := parser.ValidatePackageFile(tc.file)
		if tc.wantErr && err == nil {
			t.Errorf("ValidatePackageFile(%q): expected error, got nil", tc.file)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidatePackageFile(%q): unexpected error: %v", tc.file, err)
		}
	}
}

// TestParseIncludePackageIdentifierEmpty verifies that empty identifier produces parse error.
func TestParseIncludePackageIdentifierEmpty(t *testing.T) {
	src := `include package("", "foo.conf")`
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatal("expected parse error for empty identifier, got nil")
	}
}

// TestParseIncludePackageNestedInObject verifies package include inside an object.
func TestParseIncludePackageNestedInObject(t *testing.T) {
	src := `{ include package("foo", "bar.conf") }`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(ast.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(ast.Fields))
	}
	inc, ok := ast.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", ast.Fields[0].Value)
	}
	if !inc.IsPackage {
		t.Error("IsPackage should be true")
	}
	_ = strings.Contains // suppress unused import warning
}
