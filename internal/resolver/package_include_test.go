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
	"os"
	"path/filepath"
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

// TestPackageLookupErrorIsPreserved verifies that a non-miss error from PackageLookup
// (e.g. I/O error, permission denied) is preserved in the returned error, not replaced
// by a generic "not found" message. (review must-fix: convergent Claude+Codex finding)
func TestPackageLookupErrorIsPreserved(t *testing.T) {
	const sentinelMsg = "permission denied reading package store"
	lookup := func(id, file string) ([]byte, error) {
		return nil, fmt.Errorf("%s", sentinelMsg)
	}
	src := `include package("foo", "bar.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{PackageLookup: lookup})
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
	if !strings.Contains(err.Error(), sentinelMsg) {
		t.Errorf("expected error to contain %q, got: %v", sentinelMsg, err)
	}
}

// TestPackageParseErrorIncludesContext verifies that a parse failure in registered
// package content wraps the error with the package identifier and file, rather than
// leaking a raw parser error. (review must-fix: convergent Claude+Codex finding)
func TestPackageParseErrorIncludesContext(t *testing.T) {
	lookup := func(id, file string) ([]byte, error) {
		return []byte(`{ unclosed`), nil // deliberately malformed HOCON
	}
	src := `include package("github.com/example/lib", "bad.conf")`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{PackageLookup: lookup})
	if err == nil {
		t.Fatal("expected resolve error from malformed package content, got nil")
	}
	// Error must mention the package identifier so the user knows which package failed.
	if !strings.Contains(err.Error(), "github.com/example/lib") {
		t.Errorf("expected error to mention package identifier, got: %v", err)
	}
}

// TestPackageLookupPropagationToChildResolver verifies that PackageLookup is propagated
// to child resolvers created when resolving included files (design Codex must-fix #1).
// The test exercises the full chain: root conf → include "outer.conf" (file include) →
// include package("foo", "inner.conf") (package include within the included file).
// PackageLookup must reach the child resolver that handles the file-included content.
func TestPackageLookupPropagationToChildResolver(t *testing.T) {
	// outer.conf includes a package; it is written to a temp dir so that a real
	// file-include chain is exercised (not just direct package resolution).
	outerContent := `outer_key = "from_outer"
include package("foo", "inner.conf")`
	innerContent := `inner_key = "from_inner"`

	tmpDir := t.TempDir()
	outerFile := tmpDir + "/outer.conf"
	if err := os.WriteFile(outerFile, []byte(outerContent), 0o644); err != nil {
		t.Fatalf("write outer.conf: %v", err)
	}

	pkgs := map[string]string{
		pkgKey("foo", "inner.conf"): innerContent,
	}

	// Root conf performs a file include; the included file performs a package include.
	// Use forward slashes in the include path so HOCON's quoted-string parser doesn't
	// interpret Windows backslashes as invalid escape sequences (e.g. `\U` in `\Users\`).
	src := fmt.Sprintf(`root_key = "root"
include "%s"`, filepath.ToSlash(outerFile))
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{
		BaseDir:       tmpDir,
		PackageLookup: makeLookup(pkgs),
	})
	if err != nil {
		t.Fatalf("resolve (PackageLookup propagation via file include): %v", err)
	}
	if _, ok := res.Root.Get("inner_key"); !ok {
		t.Error("inner_key not found — PackageLookup not propagated through file-include child resolver")
	}
	if _, ok := res.Root.Get("outer_key"); !ok {
		t.Error("outer_key not found — file include itself did not resolve correctly")
	}
}
