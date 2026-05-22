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
	"strconv"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/resolver"
)

// Regression suite for issue #118: chained self-referential append (N>=3) crashes
// the resolver with infinite recursion. Lightbend produces well-defined chain
// semantics where each ${key} resolves to the value the key had immediately
// before its current assignment. go.hocon must match.
//
// Each test runs in isolation via `go test -run <name>` because a stack
// overflow in any one scenario terminates the entire test binary, masking
// later tests.

// TestIssue118_TwoStepChainStillWorks — chain length 2 (the pre-#118 working
// case) must not regress under the extended save-trigger + fold-at-save logic.
// Mirrors TestResolver_SelfReference but lives in the #118 suite so the
// regression-prevention narrative is self-contained.
func TestIssue118_TwoStepChainStillWorks(t *testing.T) {
	res := resolve(t, `branches = ["main"]
branches = ${branches} ["dev"]`)
	got := getStringSlice(t, res, "branches")
	want := []string{"main", "dev"}
	if !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestIssue118_FlatArrayChain — direct 3-step array self-ref. No includes.
func TestIssue118_FlatArrayChain(t *testing.T) {
	res := resolve(t, `branches = ["main"]
branches = ${branches} ["dev"]
branches = ${branches} ["release"]`)
	got := getStringSlice(t, res, "branches")
	want := []string{"main", "dev", "release"}
	if !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestIssue118_FourStepArrayChain — induction check: chain length 4 still resolves.
func TestIssue118_FourStepArrayChain(t *testing.T) {
	res := resolve(t, `a = ["a"]
a = ${a} ["b"]
a = ${a} ["c"]
a = ${a} ["d"]`)
	got := getStringSlice(t, res, "a")
	want := []string{"a", "b", "c", "d"}
	if !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestIssue118_ChainedInclude — 3-file scenario from the original issue report.
// parent.conf: include common; include child; branches = ${branches} ["release"]
func TestIssue118_ChainedInclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "common.conf"), `branches = ["main"]`)
	writeFile(t, filepath.Join(dir, "child.conf"), `branches = ${branches} ["dev"]`)
	res := resolveWithDir(t, `include "common.conf"
include "child.conf"
branches = ${branches} ["release"]`, dir)
	got := getStringSlice(t, res, "branches")
	want := []string{"main", "dev", "release"}
	if !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestIssue118_ChainedIncludeAllChain — every include itself appends self-ref.
// parent: branches = ["root"]; include a; include b
// a: branches = ${branches} ["x"]
// b: branches = ${branches} ["y"]
func TestIssue118_ChainedIncludeAllChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.conf"), `branches = ${branches} ["x"]`)
	writeFile(t, filepath.Join(dir, "b.conf"), `branches = ${branches} ["y"]`)
	res := resolveWithDir(t, `branches = ["root"]
include "a.conf"
include "b.conf"`, dir)
	got := getStringSlice(t, res, "branches")
	want := []string{"root", "x", "y"}
	if !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestIssue118_ObjectChain — 3-step object self-ref (HOCON spec: object-object
// concat means deep merge). The previous prior-save logic skipped the existing-Object
// case when val is non-Object (a concat), losing the step-2 prior; this regression
// test asserts the chain resolves correctly to {a:1, b:2, c:3}.
func TestIssue118_ObjectChain(t *testing.T) {
	res := resolve(t, `obj = { a = 1 }
obj = ${obj} { b = 2 }
obj = ${obj} { c = 3 }`)
	v, ok := res.Root.Get("obj")
	if !ok {
		t.Fatal("obj not found")
	}
	ov, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("expected ObjectVal, got %T", v)
	}
	for _, k := range []string{"a", "b", "c"} {
		if _, ok := ov.Get(k); !ok {
			t.Errorf("expected key %q in result, got keys=%v", k, ov.Keys())
		}
	}
	expectInt(t, ov, "a", 1)
	expectInt(t, ov, "b", 2)
	expectInt(t, ov, "c", 3)
}

// TestIssue118_MultiSegmentChain — chain on a nested path (`r.x = ${r.x} [...]` × 3).
// Before #118 this produced a "circular reference detected" error rather than the
// stack overflow seen on top-level chains, but the bug class is the same: the
// per-object priorValues got overwritten with a self-referential concat. Fixed
// by extending the same fold-at-save logic to setPath.
func TestIssue118_MultiSegmentChain(t *testing.T) {
	res := resolve(t, `r.x = ["a"]
r.x = ${r.x} ["b"]
r.x = ${r.x} ["c"]`)
	v, ok := res.Root.Get("r")
	if !ok {
		t.Fatal("r not found")
	}
	rObj, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("r: expected ObjectVal, got %T", v)
	}
	xv, ok := rObj.Get("x")
	if !ok {
		t.Fatal("r.x not found")
	}
	arr, ok := xv.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("r.x: expected ArrayVal, got %T", xv)
	}
	want := []string{"a", "b", "c"}
	if len(arr.Elements) != len(want) {
		t.Fatalf("r.x: expected %d elements, got %d", len(want), len(arr.Elements))
	}
	for i, w := range want {
		sv, ok := arr.Elements[i].(*resolver.ScalarVal)
		if !ok || sv.Raw != w {
			t.Errorf("r.x[%d]: expected %q, got %v", i, w, arr.Elements[i])
		}
	}
}

// TestIssue118_NestedObjectScopedChain — nested-object form where the inner
// field uses `${parent.leaf}` from inside a nested object block:
//
//	r {
//	  x = ["a"]
//	  x = ${r.x} ["b"]
//	  x = ${r.x} ["c"]
//	}
//
// The inner resolveObject runs with pathPrefix=[r]; each inner field has
// Key=[x] (single segment). The substPlaceholder for `${r.x}` carries
// segments=[r, x] — the fully-qualified path. The original PR-121 fix
// computed fullKey from the bare leaf key only, missing the self-ref and
// silently falling through to a "circular reference detected" error
// instead of resolving the chain. Surfaced by Copilot review on PR #121.
//
// Lightbend resolves this to ["a", "b", "c"].
func TestIssue118_NestedObjectScopedChain(t *testing.T) {
	res := resolve(t, `r {
  x = ["a"]
  x = ${r.x} ["b"]
  x = ${r.x} ["c"]
}`)
	v, ok := res.Root.Get("r")
	if !ok {
		t.Fatal("r not found")
	}
	rObj, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("r: expected ObjectVal, got %T", v)
	}
	xv, ok := rObj.Get("x")
	if !ok {
		t.Fatal("r.x not found")
	}
	arr, ok := xv.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("r.x: expected ArrayVal, got %T", xv)
	}
	want := []string{"a", "b", "c"}
	if len(arr.Elements) != len(want) {
		t.Fatalf("r.x: expected %d elements, got %d", len(want), len(arr.Elements))
	}
	for i, w := range want {
		sv, ok := arr.Elements[i].(*resolver.ScalarVal)
		if !ok || sv.Raw != w {
			t.Errorf("r.x[%d]: expected %q, got %v", i, w, arr.Elements[i])
		}
	}
}

// TestIssue118_SingleSelfRefWithoutPriorErrors — edge case: lone `a = ${a} ["x"]`
// (no earlier prior assignment) must still produce a clean "unresolved
// self-referential substitution" error, NOT a crash. Verifies the
// skip-save-when-no-old-prior behavior preserves the existing error path.
func TestIssue118_SingleSelfRefWithoutPriorErrors(t *testing.T) {
	// Direct call to resolver (not resolve helper) because resolve helper
	// t.Fatal's on error; we want to verify error is returned.
	got := tryResolve(t, `a = ${a} ["x"]`)
	if got.err == nil {
		t.Fatal("expected resolve error for self-ref with no prior, got nil")
	}
}

// ---- helpers ----

func getStringSlice(t *testing.T, res *resolver.Result, key string) []string {
	t.Helper()
	v, ok := res.Root.Get(key)
	if !ok {
		t.Fatalf("%s not found in result", key)
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("%s: expected ArrayVal, got %T", key, v)
	}
	out := make([]string, len(arr.Elements))
	for i, e := range arr.Elements {
		sv, ok := e.(*resolver.ScalarVal)
		if !ok {
			t.Fatalf("%s[%d]: expected ScalarVal, got %T", key, i, e)
		}
		out[i] = sv.Raw
	}
	return out
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func expectInt(t *testing.T, ov *resolver.ObjectVal, key string, want int) {
	t.Helper()
	v, ok := ov.Get(key)
	if !ok {
		t.Errorf("key %q not found in object", key)
		return
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		t.Errorf("%s: expected ScalarVal, got %T", key, v)
		return
	}
	// Numeric scalars are stored as their raw textual form.
	if sv.Raw != strconv.Itoa(want) {
		t.Errorf("%s: expected %d, got %s", key, want, sv.Raw)
	}
}

type tryResolveResult struct {
	res *resolver.Result
	err error
}

func tryResolve(t *testing.T, src string) tryResolveResult {
	t.Helper()
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{})
	return tryResolveResult{res: res, err: err}
}
