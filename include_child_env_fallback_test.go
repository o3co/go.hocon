// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/o3co/go.hocon"
)

// Bug regression suite for #128: the include-child path of resolveSubstitutions
// constructed a fresh result object and copied obj.values[k] forward but
// dropped obj.priorValues. For the canonical "default + optional env override"
// pattern
//
//	registry {
//	  instance-id = "localhost"
//	  instance-id = ${?REGISTRY_INSTANCE_ID}
//	}
//
// reached through an include (file or package), this stripped the prior chain
// at the include boundary. When the parent's strict resolve pass then evaluated
// the still-placeholder ${?…} and the env var was unset, the optional return
// looked up obj.priorValues[k] in an empty map and dropped the field entirely
// — silently regressing the documented Lightbend semantics where the prior
// duplicate-key assignment must be retained.
//
// Originally fixed in PR #129 by carrying obj.priorValues across the include
// child's own resolve pass. Since #135 the include child no longer resolves at
// all — it returns an unresolved tree, the parent merges it (priors and all),
// and ResolveTree's single top-level pass falls back to the merged prior when
// the optional ${?…} is unsatisfied — so the original-source assignment is
// retained without any child-scoped carry.
//
// These regression tests pin both halves of the bug (env set → applied; env
// unset → prior retained) on both the file-include and include-package
// (E11) paths, plus the deferred parse/resolve lifecycle.

// unsetEnvForTest unsets `key` for the duration of the test and restores any
// prior value via t.Cleanup. Mirrors t.Setenv's auto-restore semantics for the
// unset case (which testing does not provide a direct helper for).
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	if prev, ok := os.LookupEnv(key); ok {
		t.Cleanup(func() { _ = os.Setenv(key, prev) })
	}
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

// TestIncludeFile_OptionalEnvFallback_AppliesSystemEnv pins: env set →
// child-included content resolves ${?VAR} against the process environment.
func TestIncludeFile_OptionalEnvFallback_AppliesSystemEnv(t *testing.T) {
	t.Setenv("ICEF_TEST_VAR_SET", "from-env")

	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte(`result = ${?ICEF_TEST_VAR_SET}`), 0o644); err != nil {
		t.Fatal(err)
	}
	parentFile := filepath.Join(dir, "parent.conf")
	src := fmt.Sprintf(`include "%s"`+"\n", filepath.ToSlash(childFile))
	if err := os.WriteFile(parentFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(parentFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := cfg.GetStringOption("result").Get()
	if !ok {
		t.Fatalf("result missing — env-var fallback must apply through include")
	}
	if got != "from-env" {
		t.Errorf("result=%q, want %q", got, "from-env")
	}
}

// TestIncludeFile_OptionalEnvFallback_PreservesPriorDefaultWhenEnvUnset pins
// the failing-half of the bug: env unset → the prior in-source default
// assignment must remain, not be erased.
func TestIncludeFile_OptionalEnvFallback_PreservesPriorDefaultWhenEnvUnset(t *testing.T) {
	unsetEnvForTest(t, "ICEF_TEST_VAR_UNSET")

	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	childContent := `
registry {
  instance-id = "localhost"
  instance-id = ${?ICEF_TEST_VAR_UNSET}
}
`
	if err := os.WriteFile(childFile, []byte(childContent), 0o644); err != nil {
		t.Fatal(err)
	}
	parentFile := filepath.Join(dir, "parent.conf")
	src := fmt.Sprintf(`include "%s"`+"\n", filepath.ToSlash(childFile))
	if err := os.WriteFile(parentFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(parentFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := cfg.GetStringOption("registry.instance-id").Get()
	if !ok {
		t.Fatalf("registry.instance-id missing — prior default must remain when ${?ENV} is unset")
	}
	if got != "localhost" {
		t.Errorf("registry.instance-id=%q, want %q", got, "localhost")
	}
}

// TestIncludePackage_OptionalEnvFallback_AppliesSystemEnv pins: env set →
// content reached via include package(...) (E11) resolves ${?VAR} against
// the process environment.
func TestIncludePackage_OptionalEnvFallback_AppliesSystemEnv(t *testing.T) {
	t.Setenv("ICEF_PKG_VAR_SET", "from-pkg-env")
	t.Cleanup(hocon.ResetPackageRegistry)

	const refContent = `result = ${?ICEF_PKG_VAR_SET}`
	const pkgID = "github.com/o3co/go.hocon/test/include-env-fallback-set"
	if err := hocon.RegisterPackage(pkgID, "reference.conf", []byte(refContent)); err != nil {
		t.Fatalf("RegisterPackage: %v", err)
	}

	cfg, err := hocon.ParseString(fmt.Sprintf(`include package(%q, "reference.conf")`+"\n", pkgID))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := cfg.GetStringOption("result").Get()
	if !ok {
		t.Fatalf("result missing — env-var fallback must apply through include package")
	}
	if got != "from-pkg-env" {
		t.Errorf("result=%q, want %q", got, "from-pkg-env")
	}
}

// TestIncludePackage_OptionalEnvFallback_PreservesPriorDefaultWhenEnvUnset
// pins the failing-half of the bug for the E11 path: env unset → the prior
// in-source default assignment must remain.
func TestIncludePackage_OptionalEnvFallback_PreservesPriorDefaultWhenEnvUnset(t *testing.T) {
	unsetEnvForTest(t, "ICEF_PKG_VAR_UNSET")
	t.Cleanup(hocon.ResetPackageRegistry)

	const refContent = `
registry {
  instance-id = "localhost"
  instance-id = ${?ICEF_PKG_VAR_UNSET}
}
`
	const pkgID = "github.com/o3co/go.hocon/test/include-env-fallback-unset"
	if err := hocon.RegisterPackage(pkgID, "reference.conf", []byte(refContent)); err != nil {
		t.Fatalf("RegisterPackage: %v", err)
	}

	cfg, err := hocon.ParseString(fmt.Sprintf(`include package(%q, "reference.conf")`+"\n", pkgID))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := cfg.GetStringOption("registry.instance-id").Get()
	if !ok {
		t.Fatalf("registry.instance-id missing — prior default must remain when ${?ENV} is unset")
	}
	if got != "localhost" {
		t.Errorf("registry.instance-id=%q, want %q", got, "localhost")
	}
}

// TestIncludePackage_OptionalEnvFallback_DeferredPath_PreservesPriorDefault
// pins the deferred-resolution path (parse with ResolveSubstitutions=false,
// then explicit Resolve): the include-child prior chain must still surface
// when the optional env-var fallback evaluates to nothing.
func TestIncludePackage_OptionalEnvFallback_DeferredPath_PreservesPriorDefault(t *testing.T) {
	unsetEnvForTest(t, "ICEF_PKG_VAR_DEFERRED")
	t.Cleanup(hocon.ResetPackageRegistry)

	const refContent = `
registry {
  instance-id = "localhost"
  instance-id = ${?ICEF_PKG_VAR_DEFERRED}
}
`
	const pkgID = "github.com/o3co/go.hocon/test/include-env-fallback-deferred"
	if err := hocon.RegisterPackage(pkgID, "reference.conf", []byte(refContent)); err != nil {
		t.Fatalf("RegisterPackage: %v", err)
	}

	cfg, err := hocon.ParseStringWithOptions(
		fmt.Sprintf(`include package(%q, "reference.conf")`+"\n", pkgID),
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, ok := resolved.GetStringOption("registry.instance-id").Get()
	if !ok {
		t.Fatalf("registry.instance-id missing — prior default must remain when ${?ENV} is unset")
	}
	if got != "localhost" {
		t.Errorf("registry.instance-id=%q, want %q", got, "localhost")
	}
}
