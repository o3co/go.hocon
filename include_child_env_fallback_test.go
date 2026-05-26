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
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// Bug regression suite: env-var fallback (${?ENV_NAME}) inside content reached
// through an `include` (file or package) was being processed by a child
// resolver whose Options dropped UseSystemEnvironment, so the substitution
// could not see the process env. Combined with the lenient-pass placeholder
// preservation (#45 fix), env-unset turned into "drop the field entirely",
// erasing the prior assignment that the env override was meant to layer on.
//
// The classic operator pattern is "default in source + optional env override":
//
//	registry {
//	  instance-id = "localhost"
//	  instance-id = ${?REGISTRY_INSTANCE_ID}
//	}
//
// With the bug, env set → ignored (default returned); env unset → field
// vanishes (silent regression vs. the documented Lightbend semantics where
// the prior assignment is retained). Both broke real operator-facing
// reference.conf files shipped via include package(...) (E11).

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
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	src := fmt.Sprintf(`include "%s"`+"\n", slashChild)
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
	os.Unsetenv("ICEF_TEST_VAR_UNSET")

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
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	src := fmt.Sprintf(`include "%s"`+"\n", slashChild)
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
	os.Unsetenv("ICEF_PKG_VAR_UNSET")

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
	os.Unsetenv("ICEF_PKG_VAR_DEFERRED")

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
