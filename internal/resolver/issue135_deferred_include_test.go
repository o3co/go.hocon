// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver_test

import (
	"path/filepath"
	"testing"

	"github.com/o3co/go.hocon/internal/resolver"
)

// go.hocon#135: substitutions from an included file must be deferred until the
// included content has merged with the including document, matching Lightbend.
// `computed = ${base}` in common.conf must resolve against the FINAL `base`
// (17, from override.conf), not the include-local 11.
func TestIssue135_DeferredSubstitutionAcrossIncludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "common.conf"), "base = 11\ncomputed = ${base}")
	writeFile(t, filepath.Join(dir, "override.conf"), `base = 17`)
	res := resolveWithDir(t, `include "common.conf"
include "override.conf"
value = ${computed}`, dir)

	for _, tc := range []struct{ key, want string }{
		{"base", "17"},
		{"computed", "17"},
		{"value", "17"},
	} {
		v, ok := res.Root.Get(tc.key)
		if !ok {
			t.Fatalf("%s not found", tc.key)
		}
		sv, ok := v.(*resolver.ScalarVal)
		if !ok {
			t.Fatalf("%s: expected ScalarVal, got %T", tc.key, v)
		}
		if sv.Raw != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, sv.Raw, tc.want)
		}
	}
}

// A later include overriding a key that an earlier include's substitution
// depends on, mounted under a nested prefix — the deferral must relativize and
// still pick up the override.
func TestIssue135_DeferredSubstitutionNestedMount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "common.conf"), "base = 11\ncomputed = ${base}")
	writeFile(t, filepath.Join(dir, "override.conf"), `base = 17`)
	res := resolveWithDir(t, `app {
  include "common.conf"
  include "override.conf"
}`, dir)
	app, ok := res.Root.Get("app")
	if !ok {
		t.Fatal("app not found")
	}
	obj, ok := app.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("app: expected ObjectVal, got %T", app)
	}
	v, ok := obj.Get("computed")
	if !ok {
		t.Fatal("app.computed not found")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		t.Fatalf("app.computed: expected ScalarVal, got %T", v)
	}
	if sv.Raw != "17" {
		t.Errorf("app.computed = %q, want 17", sv.Raw)
	}
}
