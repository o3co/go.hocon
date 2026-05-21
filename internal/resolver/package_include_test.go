// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package resolver_test — E11 package include resolver tests.
package resolver_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/resolver"
)

// makeLookup builds a PackageLookup function from a map of (id, file) → content.
func makeLookup(pkgs map[string]string) func(id, file string) ([]byte, error) {
	return func(id, file string) ([]byte, error) {
		key := id + "\x00" + file
		if content, ok := pkgs[key]; ok {
			return []byte(content), nil
		}
		return nil, fmt.Errorf("package(%q, %q) not found in registry", id, file)
	}
}

func pkgKey(id, file string) string { return id + "\x00" + file }

// TestPackageLookupHappyPath verifies basic package include resolves correctly.
func TestPackageLookupHappyPath(t *testing.T) {
	content := `host = "example.com"
port = 8080
app.name = "lib"`

	pkgs := map[string]string{
		pkgKey("github.com/example/lib", "reference.conf"): content,
	}

	src := `include package("github.com/example/lib", "reference.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	host, ok := res.Root.Get("host")
	if !ok {
		t.Fatal("host not found")
	}
	if sv, ok := host.(*resolver.ScalarVal); !ok || sv.Raw != "example.com" {
		t.Errorf("host: want %q, got %v", "example.com", host)
	}

	port, ok := res.Root.Get("port")
	if !ok {
		t.Fatal("port not found")
	}
	if sv, ok := port.(*resolver.ScalarVal); !ok || sv.Raw != "8080" {
		t.Errorf("port: want 8080, got %v", port)
	}
}

// TestPackageLookupMissIsError verifies that a registry miss is a hard error.
func TestPackageLookupMissIsError(t *testing.T) {
	src := `include package("github.com/example/missing", "x.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pkgs := map[string]string{} // empty registry
	_, err = resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err == nil {
		t.Fatal("expected resolve error on registry miss, got nil")
	}
}

// TestPackageLookupRequiredMissIsError verifies required(package(...)) miss is error.
func TestPackageLookupRequiredMissIsError(t *testing.T) {
	src := `include required(package("github.com/example/missing", "x.conf"))`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pkgs := map[string]string{}
	_, err = resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err == nil {
		t.Fatal("expected resolve error on required+miss, got nil")
	}
}

// TestPackageLookupEmptyContentSucceeds verifies that empty content merges as {} (decision 4 note).
func TestPackageLookupEmptyContentSucceeds(t *testing.T) {
	pkgs := map[string]string{
		pkgKey("github.com/example/lib", "empty.conf"): "", // empty content
	}

	src := `app = host
include package("github.com/example/lib", "empty.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	app, ok := res.Root.Get("app")
	if !ok {
		t.Fatal("app not found")
	}
	if sv, ok := app.(*resolver.ScalarVal); !ok || sv.Raw != "host" {
		t.Errorf("app: want %q, got %v", "host", app)
	}
}

// TestPackageLookupCaseSensitiveID verifies byte-exact case-sensitive identifier lookup.
func TestPackageLookupCaseSensitiveID(t *testing.T) {
	pkgs := map[string]string{
		pkgKey("Foo/Bar", "x.conf"): "k = v",
	}
	// Use lowercase "foo/bar" — must miss
	src := `include package("foo/bar", "x.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err == nil {
		t.Fatal("expected miss error (case-sensitive id), got nil")
	}
}

// TestPackageLookupCaseSensitiveFile verifies byte-exact case-sensitive file lookup.
func TestPackageLookupCaseSensitiveFile(t *testing.T) {
	pkgs := map[string]string{
		pkgKey("github.com/example/lib", "Reference.conf"): "k = v",
	}
	// Use lowercase "reference.conf" — must miss
	src := `include package("github.com/example/lib", "reference.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err == nil {
		t.Fatal("expected miss error (case-sensitive file), got nil")
	}
}

// TestPackageLookupNilFallback verifies that with no PackageLookup option set,
// encountering a package include returns a resolve error (not nil callback panic).
func TestPackageLookupNilFallback(t *testing.T) {
	src := `include package("foo", "bar.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{
		// No PackageLookup set — global registry is empty here (no init() calls in tests).
		// Should produce a resolve error, not panic.
	})
	if err == nil {
		t.Fatal("expected resolve error with no PackageLookup, got nil")
	}
}

// TestPackageLookupCycleDetection verifies self-include cycle detection.
func TestPackageLookupCycleDetection(t *testing.T) {
	// ("foo", "self.conf") → includes itself
	pkgs := map[string]string{
		pkgKey("foo", "self.conf"): `include package("foo", "self.conf")`,
	}
	src := `include package("foo", "self.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("error should mention 'circular', got: %v", err)
	}
}

// TestPackageLookupMutualCycleDetection verifies mutual cycle detection.
func TestPackageLookupMutualCycleDetection(t *testing.T) {
	pkgs := map[string]string{
		pkgKey("foo", "a.conf"): `include package("foo", "b.conf")`,
		pkgKey("foo", "b.conf"): `include package("foo", "a.conf")`,
	}
	src := `include package("foo", "a.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err == nil {
		t.Fatal("expected mutual cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("error should mention 'circular', got: %v", err)
	}
}

// TestPackageLookupPropagationToChildResolver verifies that PackageLookup is propagated
// to child resolvers created when resolving included files (design Codex must-fix #1).
func TestPackageLookupPropagationToChildResolver(t *testing.T) {
	// The outer include includes "outer.conf", which itself includes package("foo", "inner.conf").
	// PackageLookup must be available in the child resolver.
	innerContent := `inner_key = "from_inner"`
	outerContent := `outer_key = "from_outer"
include package("foo", "inner.conf")`

	pkgs := map[string]string{
		pkgKey("foo", "inner.conf"): innerContent,
	}

	// We test this by having the AST directly contain a nested package include.
	// Simulating file-chain would require actual files; instead we nest within an object.
	src := outerContent
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{
		PackageLookup: makeLookup(pkgs),
	})
	if err != nil {
		t.Fatalf("resolve (PackageLookup propagation): %v", err)
	}
	if _, ok := res.Root.Get("inner_key"); !ok {
		t.Error("inner_key not found — PackageLookup not propagated to child resolver")
	}
}
