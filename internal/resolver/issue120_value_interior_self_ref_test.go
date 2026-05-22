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

// Regression suite for issue #120: self-references embedded inside ArrayVal
// elements or ObjectVal field values. Distinct from #118 (which covered
// concat/subst placeholder chains directly). Each test runs in isolation
// via `go test -run <name>` because stack overflow terminates the test
// binary.

// TestIssue120_ArrayElementChain3 — `a = [${a}, "x"]` × 3.
// Lightbend resolves to deeply-nested array: each ${a} expands to the value
// `a` had immediately before this assignment.
func TestIssue120_ArrayElementChain3(t *testing.T) {
	res := resolve(t, `a = ["init"]
a = [${a}, "x"]
a = [${a}, "y"]`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	// Expected nesting:
	//   step 1: a = ["init"]
	//   step 2: a = [["init"], "x"]    (${a} → ["init"])
	//   step 3: a = [[["init"], "x"], "y"]  (${a} → step 2's value)
	if len(arr.Elements) != 2 {
		t.Fatalf("expected top-level length 2, got %d", len(arr.Elements))
	}
	// elem[1] should be "y"
	if sv, ok := arr.Elements[1].(*resolver.ScalarVal); !ok || sv.Raw != "y" {
		t.Errorf("a[1]: expected \"y\", got %v", arr.Elements[1])
	}
	// elem[0] should be [["init"], "x"]
	inner, ok := arr.Elements[0].(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("a[0]: expected ArrayVal, got %T", arr.Elements[0])
	}
	if len(inner.Elements) != 2 {
		t.Fatalf("a[0]: expected length 2, got %d", len(inner.Elements))
	}
	if sv, ok := inner.Elements[1].(*resolver.ScalarVal); !ok || sv.Raw != "x" {
		t.Errorf("a[0][1]: expected \"x\", got %v", inner.Elements[1])
	}
	innerInner, ok := inner.Elements[0].(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("a[0][0]: expected ArrayVal, got %T", inner.Elements[0])
	}
	if len(innerInner.Elements) != 1 || innerInner.Elements[0].(*resolver.ScalarVal).Raw != "init" {
		t.Errorf("a[0][0]: expected [\"init\"], got %v", innerInner.Elements)
	}
}

// TestIssue120_ArrayHeadPositionChain — `a = [${a}]` × 3.
// Each step wraps the previous value in a single-element array.
// Expected: a = [[[["init"]]]] (step1) → [[["init"]]] (step2) → ... actually:
//
//	step 1: a = ["init"]
//	step 2: a = [${a}] → [["init"]]
//	step 3: a = [${a}] → [[["init"]]]
func TestIssue120_ArrayHeadPositionChain(t *testing.T) {
	res := resolve(t, `a = ["init"]
a = [${a}]
a = [${a}]`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	if len(arr.Elements) != 1 {
		t.Fatalf("top-level length: expected 1, got %d", len(arr.Elements))
	}
	a1, ok := arr.Elements[0].(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("a[0]: expected ArrayVal, got %T", arr.Elements[0])
	}
	if len(a1.Elements) != 1 {
		t.Fatalf("a[0]: expected length 1, got %d", len(a1.Elements))
	}
	a2, ok := a1.Elements[0].(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("a[0][0]: expected ArrayVal, got %T", a1.Elements[0])
	}
	if len(a2.Elements) != 1 || a2.Elements[0].(*resolver.ScalarVal).Raw != "init" {
		t.Errorf("a[0][0]: expected [\"init\"], got %v", a2.Elements)
	}
}

// TestIssue120_ObjectFieldChain2 — `o = { history = ${o}, v = 2 }` over `o = { v = 1 }`.
// Currently errors with "circular reference detected" because object-deep-merge
// path consumes prior info without saving. Chain length 2 is sufficient to
// demonstrate the bug (no chain needed).
func TestIssue120_ObjectFieldChain2(t *testing.T) {
	res := resolve(t, `o = { v = 1 }
o = { history = ${o}, v = 2 }`)
	v, ok := res.Root.Get("o")
	if !ok {
		t.Fatal("o not found")
	}
	ov, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o: expected ObjectVal, got %T", v)
	}
	// Expected: { history: {v:1}, v: 2 }
	vField, ok := ov.Get("v")
	if !ok {
		t.Fatal("o.v not found")
	}
	if sv := vField.(*resolver.ScalarVal); sv.Raw != "2" {
		t.Errorf("o.v: expected 2, got %s", sv.Raw)
	}
	hField, ok := ov.Get("history")
	if !ok {
		t.Fatal("o.history not found")
	}
	hObj, ok := hField.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o.history: expected ObjectVal, got %T", hField)
	}
	hv, ok := hObj.Get("v")
	if !ok {
		t.Fatal("o.history.v not found")
	}
	if sv := hv.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("o.history.v: expected 1, got %s", sv.Raw)
	}
}

// TestIssue120_ObjectFieldChain3 — same shape, chain length 3.
// Each step's history captures the immediately-prior o value.
func TestIssue120_ObjectFieldChain3(t *testing.T) {
	res := resolve(t, `o = { v = 1 }
o = { history = ${o}, v = 2 }
o = { history = ${o}, v = 3 }`)
	v, ok := res.Root.Get("o")
	if !ok {
		t.Fatal("o not found")
	}
	ov, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o: expected ObjectVal, got %T", v)
	}
	vField, ok := ov.Get("v")
	if !ok {
		t.Fatal("o.v not found")
	}
	if sv := vField.(*resolver.ScalarVal); sv.Raw != "3" {
		t.Errorf("o.v: expected 3, got %s", sv.Raw)
	}
	// o.history should be {history: {v:1}, v: 2} (step 2 snapshot)
	hField, ok := ov.Get("history")
	if !ok {
		t.Fatal("o.history not found")
	}
	hObj, ok := hField.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o.history: expected ObjectVal, got %T", hField)
	}
	hv, ok := hObj.Get("v")
	if !ok {
		t.Fatal("o.history.v not found")
	}
	if sv := hv.(*resolver.ScalarVal); sv.Raw != "2" {
		t.Errorf("o.history.v: expected 2, got %s", sv.Raw)
	}
}

// TestIssue120_ObjectFieldChain2RetainedKey — Codex review surfaced gap: the
// minimal-repro test only asserted the *changed* field (`v`). HOCON's
// duplicate-object merge semantics keep BOTH sides' keys (latter wins on
// conflict). This test pins the retained-key path so a future refactor that
// breaks merge semantics (and accidentally replaces existing instead of
// merging) is caught.
func TestIssue120_ObjectFieldChain2RetainedKey(t *testing.T) {
	res := resolve(t, `o = { a = 1, v = 1 }
o = { history = ${o}, v = 2 }`)
	v, ok := res.Root.Get("o")
	if !ok {
		t.Fatal("o not found")
	}
	ov, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o: expected ObjectVal, got %T", v)
	}
	// o.a should still be 1 (retained from step 1 via merge)
	aField, ok := ov.Get("a")
	if !ok {
		t.Fatal("o.a not found")
	}
	if sv := aField.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("o.a: expected 1 (retained from step 1), got %s", sv.Raw)
	}
	// o.v overridden to 2
	vField, _ := ov.Get("v")
	if sv := vField.(*resolver.ScalarVal); sv.Raw != "2" {
		t.Errorf("o.v: expected 2, got %s", sv.Raw)
	}
	// o.history should be step-1 snapshot = {a:1, v:1}
	hField, _ := ov.Get("history")
	hObj := hField.(*resolver.ObjectVal)
	ha, _ := hObj.Get("a")
	if sv := ha.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("o.history.a: expected 1, got %s", sv.Raw)
	}
	hv, _ := hObj.Get("v")
	if sv := hv.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("o.history.v: expected 1 (step-1 snapshot), got %s", sv.Raw)
	}
}

// TestIssue120_MixedConcatArrayChain — interaction between #118 concat-path
// and #120 array-element-path within the same key's chain. step 2 uses
// concat-substitution; step 3 uses array-element-substitution.
func TestIssue120_MixedConcatArrayChain(t *testing.T) {
	res := resolve(t, `a = ["init"]
a = ${a} ["x"]
a = [${a}, "y"]`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	// step 2 concat: ["init"] ++ ["x"] = ["init", "x"]
	// step 3 array literal: [step2_value, "y"] = [["init", "x"], "y"]
	if len(arr.Elements) != 2 {
		t.Fatalf("top-level length: expected 2, got %d", len(arr.Elements))
	}
	if sv, ok := arr.Elements[1].(*resolver.ScalarVal); !ok || sv.Raw != "y" {
		t.Errorf("a[1]: expected \"y\", got %v", arr.Elements[1])
	}
	inner, ok := arr.Elements[0].(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("a[0]: expected ArrayVal, got %T", arr.Elements[0])
	}
	if len(inner.Elements) != 2 {
		t.Fatalf("a[0]: expected length 2, got %d", len(inner.Elements))
	}
	if sv := inner.Elements[0].(*resolver.ScalarVal); sv.Raw != "init" {
		t.Errorf("a[0][0]: expected \"init\", got %s", sv.Raw)
	}
	if sv := inner.Elements[1].(*resolver.ScalarVal); sv.Raw != "x" {
		t.Errorf("a[0][1]: expected \"x\", got %s", sv.Raw)
	}
}

// TestIssue120_NestedPathObjectMerge — Critical-1 from PR #123 multi-agent-review.
// Multi-segment object-merge form (`r.s = {v=1}; r.s = {history=${r.s}, v=2}`)
// goes through setPath, whose save was also gated on !merged before #120's
// follow-up fix.
func TestIssue120_NestedPathObjectMerge(t *testing.T) {
	res := resolve(t, `r.s = { v = 1 }
r.s = { history = ${r.s}, v = 2 }`)
	v, ok := res.Root.Get("r")
	if !ok {
		t.Fatal("r not found")
	}
	rObj, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("r: expected ObjectVal, got %T", v)
	}
	s, ok := rObj.Get("s")
	if !ok {
		t.Fatal("r.s not found")
	}
	sObj, ok := s.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("r.s: expected ObjectVal, got %T", s)
	}
	vField, ok := sObj.Get("v")
	if !ok {
		t.Fatal("r.s.v not found")
	}
	if sv := vField.(*resolver.ScalarVal); sv.Raw != "2" {
		t.Errorf("r.s.v: expected 2, got %s", sv.Raw)
	}
	hField, ok := sObj.Get("history")
	if !ok {
		t.Fatal("r.s.history not found")
	}
	hObj, ok := hField.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("r.s.history: expected ObjectVal, got %T", hField)
	}
	hv, ok := hObj.Get("v")
	if !ok {
		t.Fatal("r.s.history.v not found")
	}
	if sv := hv.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("r.s.history.v: expected 1, got %s", sv.Raw)
	}
}

// TestIssue120_IncludeMergeObjectForm — Critical-2 from PR #123 multi-agent-review.
// Parent has `o = {v=1}`; included file has `o = {history=${o}, v=2}`. The
// include-merge object+object branch deepMerged but didn't save prior; the
// merged val retains `${o}` and resolveSubst hit "unresolved self-ref" error.
func TestIssue120_IncludeMergeObjectForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "inc.conf"), `o = { history = ${o}, v = 2 }`)
	res := resolveWithDir(t, `o = { v = 1 }
include "inc.conf"`, dir)
	v, ok := res.Root.Get("o")
	if !ok {
		t.Fatal("o not found")
	}
	ov, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o: expected ObjectVal, got %T", v)
	}
	vField, ok := ov.Get("v")
	if !ok {
		t.Fatal("o.v not found")
	}
	if sv := vField.(*resolver.ScalarVal); sv.Raw != "2" {
		t.Errorf("o.v: expected 2, got %s", sv.Raw)
	}
	hField, ok := ov.Get("history")
	if !ok {
		t.Fatal("o.history not found")
	}
	hObj, ok := hField.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("o.history: expected ObjectVal, got %T", hField)
	}
	hv, ok := hObj.Get("v")
	if !ok {
		t.Fatal("o.history.v not found")
	}
	if sv := hv.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("o.history.v: expected 1, got %s", sv.Raw)
	}
}
