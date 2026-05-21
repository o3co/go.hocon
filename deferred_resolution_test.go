// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package hocon_test exercises E12 deferred substitution resolution.
// All tests in this file are Layer-1 (programmatic per-impl) tests; the
// Layer-2 YAML scenario runner lives in deferred_resolution_fixture_test.go.
package hocon_test

import (
	"errors"
	"testing"

	"github.com/o3co/go.hocon"
)

func TestConfig_IsResolved_FusedParseAndResolveIsResolved(t *testing.T) {
	c, err := hocon.ParseString(`a = 1`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if !c.IsResolved() {
		t.Fatal("fused ParseString must produce a resolved Config")
	}
}

func TestParseStringWithOptions_ResolveSubstitutionsTrue_EquivalentToParseString(t *testing.T) {
	a, err := hocon.ParseString(`a = 1`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	b, err := hocon.ParseStringWithOptions(`a = 1`, hocon.DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	if !a.IsResolved() || !b.IsResolved() {
		t.Fatal("both must be resolved")
	}
	if a.GetInt("a") != 1 || b.GetInt("a") != 1 {
		t.Fatalf("a=%d b=%d, want 1", a.GetInt("a"), b.GetInt("a"))
	}
}

func TestParseStringWithOptions_ResolveSubstitutionsFalse_IsUnresolvedWhenSubstPresent(t *testing.T) {
	c, err := hocon.ParseStringWithOptions(
		`a = ${b}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	if c.IsResolved() {
		t.Fatal("expected unresolved Config (a = ${b} unresolved)")
	}
}

func TestParseStringWithOptions_ResolveSubstitutionsFalse_NoSubstReturnsResolved(t *testing.T) {
	c, err := hocon.ParseStringWithOptions(
		`a = 1`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	if !c.IsResolved() {
		t.Fatal("expected resolved (no substitutions present)")
	}
}

func TestGetter_OnUnresolvedSubstitutionPanicsErrNotResolved(t *testing.T) {
	c, err := hocon.ParseStringWithOptions(
		`a = ${b}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on getter against unresolved path")
		}
		ce, ok := r.(*hocon.ConfigError)
		if !ok {
			t.Fatalf("expected *ConfigError, got %T (%v)", r, r)
		}
		if !errors.Is(ce, hocon.ErrNotResolved) {
			t.Fatalf("expected ConfigError to wrap ErrNotResolved, got %v", ce)
		}
		if ce.Path != "a" {
			t.Fatalf("expected path 'a', got %q", ce.Path)
		}
	}()

	_ = c.GetString("a") // must panic
}

func TestGetter_OnResolvedLiteralWithinUnresolvedConfigSucceeds(t *testing.T) {
	// dr15 boundary: literal value accessible before resolve; substitution panics.
	c, err := hocon.ParseStringWithOptions(
		`lit = "value"
		 sub = ${KEY}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := c.GetString("lit"); got != "value" {
		t.Fatalf("lit=%q want 'value'", got)
	}
}

func TestGetterOption_OnUnresolvedReturnsNone(t *testing.T) {
	// Option-flavoured getters do not panic; an unresolved placeholder is
	// "not a scalar" and therefore not present from the Option's perspective.
	c, err := hocon.ParseStringWithOptions(
		`a = ${b}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !c.GetStringOption("a").IsNone() {
		t.Fatal("expected None for unresolved path via GetStringOption")
	}
}

func TestWithFallback_BothResolved_PreservesExistingSemantics(t *testing.T) {
	a, _ := hocon.ParseString(`a = 1
b = 2`)
	b, _ := hocon.ParseString(`b = 99
c = 3`)
	m := a.WithFallback(b)
	if !m.IsResolved() {
		t.Fatal("both inputs resolved → result resolved")
	}
	if m.GetInt("a") != 1 || m.GetInt("b") != 2 || m.GetInt("c") != 3 {
		t.Fatalf("a=%d b=%d c=%d, want 1 2 3", m.GetInt("a"), m.GetInt("b"), m.GetInt("c"))
	}
}

func TestWithFallback_UnresolvedAndResolved_ResultIsUnresolved(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${b}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	f, _ := hocon.ParseString(`b = 7`)
	m := r.WithFallback(f)
	if m.IsResolved() {
		t.Fatal("receiver unresolved → merged result unresolved (until Resolve())")
	}
}

func TestWithFallback_NilFallback_ReturnsReceiver(t *testing.T) {
	c, _ := hocon.ParseString(`a = 1`)
	if c.WithFallback(nil) != c {
		t.Fatal("nil fallback must return receiver verbatim")
	}
}

func TestWithFallback_ObjectMergeRecursive(t *testing.T) {
	a, _ := hocon.ParseStringWithOptions(
		`a { x = 1 }`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	b, _ := hocon.ParseStringWithOptions(
		`a { y = 2 }`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	m := a.WithFallback(b)
	if !m.IsResolved() {
		t.Fatal("both inputs are resolvable (no subs) → result must be resolved")
	}
	if m.GetInt("a.x") != 1 || m.GetInt("a.y") != 2 {
		t.Fatalf("a.x=%d a.y=%d, want 1 2", m.GetInt("a.x"), m.GetInt("a.y"))
	}
}

func TestResolve_OnAlreadyResolvedIsIdempotent(t *testing.T) {
	c, _ := hocon.ParseString(`a = 1`)
	r1, err := c.Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !r1.IsResolved() {
		t.Fatal("Resolve(resolved) must remain resolved")
	}
	// Double-resolve.
	r2, err := r1.Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("double Resolve: %v", err)
	}
	if r2.GetInt("a") != 1 {
		t.Fatalf("double resolve drift: a=%d", r2.GetInt("a"))
	}
}

func TestResolve_DeferredPathSucceeds(t *testing.T) {
	c, _ := hocon.ParseStringWithOptions(
		`a = ${b}
b = 1`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	r, err := c.Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !r.IsResolved() {
		t.Fatal("Resolve must produce resolved Config")
	}
	if r.GetInt("a") != 1 {
		t.Fatalf("a=%d, want 1", r.GetInt("a"))
	}
}

func TestResolve_FallbackThenResolve_IssueNinetyNineExample(t *testing.T) {
	// dr01 minimal: receiver references CI_RUN_NUMBER and shortversion;
	// runtime FromMap is not yet available (T11), so use ParseString fallback.
	r, _ := hocon.ParseStringWithOptions(
		`version = ${shortversion}-${CI_RUN_NUMBER}
variables { shortversion = "1.2.3" }`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	rt, _ := hocon.ParseStringWithOptions(
		`CI_RUN_NUMBER = "42"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	vars := r.GetConfig("variables")
	merged := r.WithFallback(rt).WithFallback(vars)
	resolved, err := merged.Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("version"); got != "1.2.3-42" {
		t.Fatalf("version=%q, want 1.2.3-42", got)
	}
}

func TestResolve_NoSystemEnvironment(t *testing.T) {
	t.Setenv("SHOULD_NOT_BE_READ", "from-env")
	c, _ := hocon.ParseStringWithOptions(
		`a = ${SHOULD_NOT_BE_READ}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	_, err := c.Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err == nil {
		t.Fatal("expected ResolveError when env disabled and var absent from config")
	}
	var re *hocon.ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *ResolveError, got %T (%v)", err, err)
	}
}

func TestResolveWith_SourceKeysAbsentFromResult(t *testing.T) {
	// dr11a equivalent.
	r, _ := hocon.ParseStringWithOptions(
		`r = ${value}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	src, _ := hocon.ParseString(`value = "found"`)
	out, err := r.ResolveWith(src, hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("ResolveWith: %v", err)
	}
	if out.GetString("r") != "found" {
		t.Fatalf("r=%q, want 'found'", out.GetString("r"))
	}
	// source's `value` key must NOT appear in the result.
	if out.Has("value") {
		t.Fatal("source's key 'value' must not appear in ResolveWith result")
	}
}

func TestResolveWith_UnresolvedSource_RaisesNotResolved(t *testing.T) {
	// dr11b: source is unresolved → ResolveWith MUST raise ErrNotResolved
	// BEFORE attempting to resolve the receiver (decision 10).
	r, _ := hocon.ParseStringWithOptions(
		`r = ${value}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	src, _ := hocon.ParseStringWithOptions(
		`value = ${still_unresolved}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	_, err := r.ResolveWith(src, hocon.DefaultResolveOptions())
	if err == nil {
		t.Fatal("expected ErrNotResolved when source is unresolved")
	}
	if !errors.Is(err, hocon.ErrNotResolved) {
		t.Fatalf("expected errors.Is(err, ErrNotResolved); got %v", err)
	}
}

func TestResolveWith_OnResolvedReceiver_IsNoOp(t *testing.T) {
	r, _ := hocon.ParseString(`r = 5`)
	src, _ := hocon.ParseString(`unused = 99`)
	out, err := r.ResolveWith(src, hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("ResolveWith: %v", err)
	}
	if out.GetInt("r") != 5 || out.Has("unused") {
		t.Fatalf("ResolveWith on resolved receiver must be a no-op (r=%d has-unused=%v)",
			out.GetInt("r"), out.Has("unused"))
	}
}

func TestResolve_AllowUnresolved_DoesNotError(t *testing.T) {
	c, _ := hocon.ParseStringWithOptions(
		`a = ${avail}
b = ${unavail}
avail = "hello"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	r, err := c.Resolve(hocon.DefaultResolveOptions().
		WithAllowUnresolved(true).
		WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve(allowUnresolved): %v", err)
	}
	if r.IsResolved() {
		t.Fatal("expected unresolved (b still has placeholder)")
	}
	if got := r.GetString("a"); got != "hello" {
		t.Fatalf("a=%q, want hello", got)
	}
	// Getter on b must panic NotResolved.
	defer func() {
		rec := recover()
		ce, ok := rec.(*hocon.ConfigError)
		if !ok || !errors.Is(ce, hocon.ErrNotResolved) {
			t.Fatalf("expected NotResolved panic on b, got %T %v", rec, rec)
		}
	}()
	_ = r.GetString("b")
}

func TestFromMap_ScalarTypes(t *testing.T) {
	c, err := hocon.FromMap(map[string]any{
		"flag":   true,
		"count":  42,
		"ratio":  3.14,
		"label":  "hello",
		"items":  []any{int64(1), int64(2), int64(3)},
		"nested": map[string]any{"inner": "deep"},
		"nothing": nil,
	}, "")
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	if !c.IsResolved() {
		t.Fatal("FromMap must produce resolved Config")
	}
	if c.GetBool("flag") != true {
		t.Fatalf("flag=%v", c.GetBool("flag"))
	}
	if c.GetInt("count") != 42 {
		t.Fatalf("count=%d", c.GetInt("count"))
	}
	if c.GetFloat64("ratio") != 3.14 {
		t.Fatalf("ratio=%v", c.GetFloat64("ratio"))
	}
	if c.GetString("label") != "hello" {
		t.Fatalf("label=%q", c.GetString("label"))
	}
	if c.GetString("nested.inner") != "deep" {
		t.Fatalf("nested.inner=%q", c.GetString("nested.inner"))
	}
	if got := c.GetIntSlice("items"); len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("items=%v", got)
	}
	// `nothing` is null: GetStringOption returns None.
	if !c.GetStringOption("nothing").IsNone() {
		t.Fatal("nothing must be None (null)")
	}
}

func TestFromMap_NilMap_ReturnsEmpty(t *testing.T) {
	c, err := hocon.FromMap(nil, "")
	if err != nil {
		t.Fatalf("FromMap(nil): %v", err)
	}
	if !c.IsResolved() {
		t.Fatal("must be resolved (no substitutions)")
	}
	if len(c.Keys()) != 0 {
		t.Fatalf("expected empty, got %v", c.Keys())
	}
}

func TestFromMap_Uint64Overflow_Errors(t *testing.T) {
	_, err := hocon.FromMap(map[string]any{
		"big": uint64(1<<63 + 1), // exceeds int64 range
	}, "")
	if err == nil {
		t.Fatal("expected overflow error for uint64 > int64.Max")
	}
}

func TestFromMap_UnsupportedType_Errors(t *testing.T) {
	_, err := hocon.FromMap(map[string]any{
		"oops": make(chan int),
	}, "")
	if err == nil {
		t.Fatal("expected error for unsupported type chan int")
	}
}

func TestEmpty_HasNoKeys(t *testing.T) {
	c := hocon.Empty("")
	if !c.IsResolved() {
		t.Fatal("Empty must be resolved")
	}
	if len(c.Keys()) != 0 {
		t.Fatalf("expected empty, got %v", c.Keys())
	}
}

func TestEmpty_AsFallbackIsNoOp(t *testing.T) {
	c, _ := hocon.ParseString(`a = 1`)
	m := c.WithFallback(hocon.Empty(""))
	if m.GetInt("a") != 1 {
		t.Fatalf("Empty() fallback must be no-op; a=%d", m.GetInt("a"))
	}
}

func TestEmpty_AsReceiverWithFallback(t *testing.T) {
	c, _ := hocon.ParseString(`a = 1
b = 2`)
	m := hocon.Empty("").WithFallback(c)
	if m.GetInt("a") != 1 || m.GetInt("b") != 2 {
		t.Fatalf("Empty().WithFallback(c) must expose c's keys; a=%d b=%d", m.GetInt("a"), m.GetInt("b"))
	}
}

func TestEmpty_Resolve_IsNoOp(t *testing.T) {
	r, err := hocon.Empty("").Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("Resolve(Empty): %v", err)
	}
	if !r.IsResolved() || len(r.Keys()) != 0 {
		t.Fatalf("Empty().Resolve() must be {}; keys=%v", r.Keys())
	}
}

// ── E12 Layer-1 programmatic conformance tests (T12) ──────────────────────────
// Covers spec edges not directly expressible in YAML scenarios.

func TestS13a_OptionalSelfRefAcrossFallback_Dr04(t *testing.T) {
	// receiver: a = ${?a} extra
	// fallback: a = base
	// result:   a = "base extra"
	r, _ := hocon.ParseStringWithOptions(
		`a = ${?a} extra`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	f, _ := hocon.ParseStringWithOptions(
		`a = base`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	resolved, err := r.WithFallback(f).Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != "base extra" {
		t.Fatalf("a=%q, want 'base extra'", got)
	}
}

func TestS13a_RequiredSelfRefWithFallbackPrior_Dr05(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${a} extra`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	f, _ := hocon.ParseStringWithOptions(
		`a = base`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	resolved, err := r.WithFallback(f).Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != "base extra" {
		t.Fatalf("a=%q, want 'base extra'", got)
	}
}

func TestS13a_RequiredSelfRefNoFallback_Dr06(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${a} extra`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	_, err := r.Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err == nil {
		t.Fatal("required self-ref with no prior must error")
	}
}

func TestTransitive_CrossLayer_Dr21(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${b}`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	f1, _ := hocon.ParseStringWithOptions(
		`b = ${c}`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	f2, _ := hocon.ParseStringWithOptions(
		`c = 1`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	resolved, err := r.WithFallback(f1).WithFallback(f2).Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetInt("a"); got != 1 {
		t.Fatalf("a=%d, want 1", got)
	}
}

func TestHidden_AcrossLayers_Dr23(t *testing.T) {
	// Receiver foo = 42, fallback foo = ${nonexist} → {foo:42} no error.
	r, _ := hocon.ParseStringWithOptions(
		`foo = 42`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	f, _ := hocon.ParseStringWithOptions(
		`foo = ${nonexist}`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	resolved, err := r.WithFallback(f).Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("hidden substitution must not error; got %v", err)
	}
	if got := resolved.GetInt("foo"); got != 42 {
		t.Fatalf("foo=%d, want 42", got)
	}
}

func TestCrossLayerCycle_Dr18(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${b}`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	f, _ := hocon.ParseStringWithOptions(
		`b = ${a}`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	_, err := r.WithFallback(f).Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err == nil {
		t.Fatal("cross-layer cycle must raise ResolveError")
	}
}

func TestOptionalUndefMaterialisation_Standalone(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${?x}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	resolved, err := r.Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Has("a") {
		t.Fatal("standalone optional undefined → field must be omitted")
	}
}

func TestOptionalUndefMaterialisation_StringConcat(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a = ${?x} "tail"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	resolved, err := r.Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != " tail" {
		t.Fatalf("a=%q, want ' tail' (leading space preserved)", got)
	}
}

func TestDr10_CompositionBarrier(t *testing.T) {
	r, _ := hocon.ParseStringWithOptions(
		`a { x = 1 }`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	fb1, _ := hocon.ParseStringWithOptions(
		`a = "scalar"`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	fb2, _ := hocon.ParseStringWithOptions(
		`a { y = 2 }`, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	m := r.WithFallback(fb1).WithFallback(fb2)
	resolved, err := m.Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.GetInt("a.x") != 1 {
		t.Fatalf("a.x=%d, want 1", resolved.GetInt("a.x"))
	}
	if resolved.GetConfig("a").Has("y") {
		t.Fatal("composition barrier: fb2's y must not contribute")
	}
}

func TestRenderJSON_BasicScalarsAndObjects(t *testing.T) {
	c, _ := hocon.ParseString(`
		a = 1
		b = "hello"
		c { x = true
		    y = null }
		d = [1, 2, 3]
	`)
	got, err := hocon.RenderJSON_ForTest(c) // export-for-test shim
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	expected := `{"a":1,"b":"hello","c":{"x":true,"y":null},"d":[1,2,3]}`
	if got != expected {
		t.Fatalf("renderJSON mismatch:\n got  %s\n want %s", got, expected)
	}
}

func TestDr17_E11PackageIncludeDeferred(t *testing.T) {
	// Register a package that itself contains a substitution.  After parse-
	// only, the include is expanded — the substitution placeholder remains.
	// After Resolve with a FromMap providing EXTERNAL, value resolves.
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	pkgContent := []byte(`value = ${EXTERNAL}`)
	if err := hocon.RegisterPackage("dr17-test-pkg", "ref.conf", pkgContent); err != nil {
		t.Fatalf("RegisterPackage: %v", err)
	}
	defer hocon.UnregisterPackage("dr17-test-pkg", "ref.conf")

	r, err := hocon.ParseStringWithOptions(
		`include package("dr17-test-pkg", "ref.conf")
outer = "top"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	if r.IsResolved() {
		t.Fatal("expected unresolved (EXTERNAL placeholder remains)")
	}
	fb, _ := hocon.FromMap(map[string]any{"EXTERNAL": "injected"}, "")
	resolved, err := r.WithFallback(fb).Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.GetString("value") != "injected" {
		t.Fatalf("value=%q, want 'injected'", resolved.GetString("value"))
	}
	if resolved.GetString("outer") != "top" {
		t.Fatalf("outer=%q, want 'top'", resolved.GetString("outer"))
	}
}
