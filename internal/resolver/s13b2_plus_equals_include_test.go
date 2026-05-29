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

// `+=` array-append accumulation across include boundaries (go.hocon#134,
// S13b.2). `a += b` ≡ `a = ${?a} [b]` (HOCON.md L732), and includes are treated
// as textual inlining, so repeated `+=` across included files must accumulate
// in document order. Pre-fix `+=` did an eager lookup-and-append that
// snapshotted the existing array in each included file's isolated scope; the
// cross-include merge then overwrote it, dropping earlier includes' elements.
// The fix desugars `+=` to the `${?key} [value]` self-ref concat so it flows
// through the chained-self-ref machinery that already accumulates across
// includes. Reset semantics (an explicit `k = [...]` before a `k +=`) fall out
// for free: the desugared form is an ordinary self-ref chain, and a
// non-self-ref `=` breaks it exactly as the existing duplicate-key machinery
// dictates.

// nestedStringSlice extracts res.outer.inner as a []string of scalar Raws.
func nestedStringSlice(t *testing.T, res *resolver.Result, outer, inner string) []string {
	t.Helper()
	ov, ok := res.Root.Get(outer)
	if !ok {
		t.Fatalf("%s not found", outer)
	}
	obj, ok := ov.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("%s: expected ObjectVal, got %T", outer, ov)
	}
	iv, ok := obj.Get(inner)
	if !ok {
		t.Fatalf("%s.%s not found", outer, inner)
	}
	arr, ok := iv.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("%s.%s: expected ArrayVal, got %T", outer, inner, iv)
	}
	out := make([]string, len(arr.Elements))
	for i, e := range arr.Elements {
		sv, ok := e.(*resolver.ScalarVal)
		if !ok {
			t.Fatalf("%s.%s[%d]: expected ScalarVal, got %T", outer, inner, i, e)
		}
		out[i] = sv.Raw
	}
	return out
}

func TestS13b2_PlusEqualsAccumulatesAcrossIncludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.conf"), `items += "first"`)
	writeFile(t, filepath.Join(dir, "second.conf"), `items += "second"`)
	res := resolveWithDir(t, `include "first.conf"
include "second.conf"
items += "main"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"first", "second", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_ExplicitResetInIncludeBreaksChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.conf"), `items += "first"`)
	writeFile(t, filepath.Join(dir, "second.conf"), "items = []\nitems += \"second\"")
	res := resolveWithDir(t, `include "first.conf"
include "second.conf"
items += "main"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"second", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_WithinFileChainUnchanged(t *testing.T) {
	res := resolve(t, "items += \"a\"\nitems += \"b\"\nitems += \"c\"")
	got := getStringSlice(t, res, "items")
	if want := []string{"a", "b", "c"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_PlusEqualsWithPriorArraySeed(t *testing.T) {
	res := resolve(t, "items = [\"seed\"]\nitems += \"a\"\nitems += \"b\"")
	got := getStringSlice(t, res, "items")
	if want := []string{"seed", "a", "b"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// Separate object blocks each contributing a `+=` to a nested key (no includes)
// must accumulate in document order. This routes through deepMerge's object
// collision + the setPath prior fold; the #135 self-ref-prior stitch
// (selfRefFullKey + setPath object-scoped prior fallback) fixed it incidentally
// — it produced ["c"] then ["b","c"] before. Pinned so it cannot regress.
func TestS13b2_NestedSeparateBlocksPlusEqualsAccumulate(t *testing.T) {
	res := resolve(t, "srv { items = [\"a\"] }\nsrv { items += \"b\" }\nsrv { items += \"c\" }")
	got := nestedStringSlice(t, res, "srv", "items")
	if want := []string{"a", "b", "c"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_PlusEqualsOnNonArrayPriorErrors(t *testing.T) {
	if _, err := resolveErr("a = 42\na += 1"); err == nil {
		t.Fatal("expected error for += on non-array prior")
	}
}

func TestS13b2_NestedKeyPlusEqualsAccumulatesAcrossIncludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.conf"), `srv { items += "a" }`)
	writeFile(t, filepath.Join(dir, "b.conf"), `srv { items += "b" }`)
	res := resolveWithDir(t, `include "a.conf"
include "b.conf"
srv.items += "main"`, dir)
	got := nestedStringSlice(t, res, "srv", "items")
	if want := []string{"a", "b", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// #135 regression: a within-file `+=` chain in a NESTED include (mounted under
// an object key) must keep its chain bottom across the object merge. The nested
// merge routes through deepMerge, which must stitch the earlier include's full
// within-file prior (not just its last assignment) — the single-write nested
// test above does not exercise this.
func TestS13b2_NestedMultiWriteEarlierIncludeKeepsChainBottom(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.conf"), `srv { items += "a1"`+"\n"+`items += "a2" }`)
	writeFile(t, filepath.Join(dir, "b.conf"), `srv { items += "b1" }`)
	res := resolveWithDir(t, `include "a.conf"
include "b.conf"`, dir)
	got := nestedStringSlice(t, res, "srv", "items")
	if want := []string{"a1", "a2", "b1"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_TwoNestedMultiWriteIncludesAccumulate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.conf"), `srv { items += "a1"`+"\n"+`items += "a2" }`)
	writeFile(t, filepath.Join(dir, "b.conf"), `srv { items += "b1"`+"\n"+`items += "b2" }`)
	res := resolveWithDir(t, `include "a.conf"
include "b.conf"
srv.items += "main"`, dir)
	got := nestedStringSlice(t, res, "srv", "items")
	if want := []string{"a1", "a2", "b1", "b2", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_PrefixMountedIncludeRelativizesSelfRef(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "inner.conf"), "items += \"i1\"\nitems += \"i2\"")
	res := resolveWithDir(t, `mount { include "inner.conf" }
mount.items += "outer"`, dir)
	got := nestedStringSlice(t, res, "mount", "items")
	if want := []string{"i1", "i2", "outer"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_ParentResetAfterIncludeBreaksChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "c.conf"), `items += "c"`)
	res := resolveWithDir(t, `include "c.conf"
items = ["reset"]
items += "after"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"reset", "after"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// The cases below exercise the path the rejected reset discriminator got wrong:
// a within-file `+=` chain inside a LATER include merged onto a non-empty
// destination. The desugar reduces these to ordinary self-ref chains.

func TestS13b2_WithinFileChainInLaterIncludeAccumulates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.conf"), `items += "first"`)
	writeFile(t, filepath.Join(dir, "second.conf"), "items += \"s1\"\nitems += \"s2\"")
	res := resolveWithDir(t, `include "first.conf"
include "second.conf"
items += "main"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"first", "s1", "s2", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_TwoMultiWriteIncludesAccumulate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.conf"), "items += \"a1\"\nitems += \"a2\"")
	writeFile(t, filepath.Join(dir, "second.conf"), "items += \"b1\"\nitems += \"b2\"")
	res := resolveWithDir(t, `include "first.conf"
include "second.conf"
items += "main"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"a1", "a2", "b1", "b2", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_ResetInMultiWriteLaterIncludeBreaksChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.conf"), `items += "first"`)
	writeFile(t, filepath.Join(dir, "second.conf"), "items = [\"r1\"]\nitems += \"r2\"")
	res := resolveWithDir(t, `include "first.conf"
include "second.conf"
items += "main"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"r1", "r2", "main"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestS13b2_ThreeLevelWithinFileChainInInclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "first.conf"), `items += "first"`)
	writeFile(t, filepath.Join(dir, "second.conf"), "items += \"s1\"\nitems += \"s2\"\nitems += \"s3\"")
	res := resolveWithDir(t, `include "first.conf"
include "second.conf"`, dir)
	got := getStringSlice(t, res, "items")
	if want := []string{"first", "s1", "s2", "s3"}; !equalStringSlice(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
