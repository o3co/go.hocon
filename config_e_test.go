// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

// go.hocon#142 — error-returning (T, error) accessor family.
//
// Coverage: for each GetXxxE accessor, assert (a) the success path returns the
// expected value with nil error, (b) each error class produces a *ConfigError
// with the right Path + Message shape. Unresolved-placeholder errors must wrap
// ErrNotResolved for errors.Is matching.

const issue142FixtureConf = `
s = "hello"
i = 42
f = 3.14
b = true
d = 5 seconds
bytes = "10K"
n = null
obj { x = 1 }
arr = [1, 2, 3]
strs = ["a", "b"]
mixed = [1, "two"]
cfgs = [{ k = 1 }, { k = 2 }]
neg_bytes = "-1B"
`

func mustParse142(t *testing.T) *hocon.Config {
	t.Helper()
	cfg, err := hocon.ParseString(issue142FixtureConf)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return cfg
}

// asConfigError unwraps to *ConfigError; fails the test if err is nil or not a
// *ConfigError. Returns the unwrapped value for further assertions.
func asConfigError(t *testing.T, err error) *hocon.ConfigError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *hocon.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *hocon.ConfigError, got %T (%v)", err, err)
	}
	return ce
}

// ── lookupE shared error classes (verified via GetStringE as proxy) ─────

func TestIssue142_GetStringE_EmptyPath(t *testing.T) {
	cfg := mustParse142(t)
	_, err := cfg.GetStringE("")
	ce := asConfigError(t, err)
	if ce.Message != "empty path" {
		t.Errorf("Message = %q, want %q", ce.Message, "empty path")
	}
}

func TestIssue142_GetStringE_MissingKey(t *testing.T) {
	cfg := mustParse142(t)
	_, err := cfg.GetStringE("nope")
	ce := asConfigError(t, err)
	if ce.Path != "nope" || ce.Message != "key not found" {
		t.Errorf("got Path=%q Message=%q", ce.Path, ce.Message)
	}
}

// GetConfigE on an unresolved OBJECT path must also wrap ErrNotResolved (the
// lookupE branch walks ObjectVal recursively via isUnresolvedPlaceholder, so
// any transitive unresolved placeholder under the path surfaces it). This pins
// that the wrapping is not scalar-only.
func TestIssue142_GetConfigE_UnresolvedWrapsErrNotResolved(t *testing.T) {
	cfg, err := hocon.ParseStringWithOptions(`obj { x = ${obj.x} "suffix" }`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(hocon.DefaultResolveOptions().WithAllowUnresolved(true))
	if err != nil {
		t.Fatalf("Resolve(AllowUnresolved=true): %v", err)
	}
	if resolved.IsResolved() {
		t.Fatal("expected unresolved Config")
	}
	_, gerr := resolved.GetConfigE("obj")
	if !errors.Is(gerr, hocon.ErrNotResolved) {
		t.Errorf("expected errors.Is(err, ErrNotResolved); got %v", gerr)
	}
}

func TestIssue142_UnresolvedWrapsErrNotResolved(t *testing.T) {
	// An unresolved placeholder must surface as a *ConfigError whose Unwrap
	// chain returns ErrNotResolved — so callers can use errors.Is. Build an
	// unresolved Config the same way issue106_test.go does: parse without
	// substitution resolution, then Resolve(AllowUnresolved=true).
	cfg, err := hocon.ParseStringWithOptions(`x = ${x} "suffix"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(hocon.DefaultResolveOptions().WithAllowUnresolved(true))
	if err != nil {
		t.Fatalf("Resolve(AllowUnresolved=true): %v", err)
	}
	if resolved.IsResolved() {
		t.Fatal("expected unresolved Config (self-ref with no prior)")
	}
	_, gerr := resolved.GetStringE("x")
	if !errors.Is(gerr, hocon.ErrNotResolved) {
		t.Errorf("expected errors.Is(err, ErrNotResolved); got %v", gerr)
	}
	// Also verify the wrapping type is *ConfigError so callers can inspect Path.
	ce := asConfigError(t, gerr)
	if ce.Path != "x" {
		t.Errorf("Path = %q, want %q", ce.Path, "x")
	}
}

// ── per-accessor success + scalar-class errors ───────────────────────

func TestIssue142_GetStringE(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetStringE("s"); err != nil || got != "hello" {
		t.Errorf("success: got (%q, %v), want (\"hello\", nil)", got, err)
	}
	// non-scalar
	_, err := cfg.GetStringE("obj")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "expected scalar") {
		t.Errorf("non-scalar Message = %q", ce.Message)
	}
	// null
	_, err = cfg.GetStringE("n")
	ce = asConfigError(t, err)
	if ce.Message != "value is null" {
		t.Errorf("null Message = %q, want \"value is null\"", ce.Message)
	}
}

func TestIssue142_GetInt64E(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetInt64E("i"); err != nil || got != 42 {
		t.Errorf("success: got (%d, %v)", got, err)
	}
	// type-conversion failure (string-shaped value)
	_, err := cfg.GetInt64E("s")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "expected int64") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetIntE(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetIntE("i"); err != nil || got != 42 {
		t.Errorf("success: got (%d, %v)", got, err)
	}
	// Underlying int64 failure surfaces through.
	_, err := cfg.GetIntE("s")
	_ = asConfigError(t, err)
}

func TestIssue142_GetFloat64E(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetFloat64E("f"); err != nil || got != 3.14 {
		t.Errorf("success: got (%v, %v)", got, err)
	}
	_, err := cfg.GetFloat64E("s")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "expected float64") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetFloat32E(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetFloat32E("f"); err != nil || float64(got) < 3.13 || float64(got) > 3.15 {
		t.Errorf("success: got (%v, %v)", got, err)
	}
	_, err := cfg.GetFloat32E("s")
	_ = asConfigError(t, err)
}

func TestIssue142_GetBoolE(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetBoolE("b"); err != nil || got != true {
		t.Errorf("success: got (%v, %v)", got, err)
	}
	_, err := cfg.GetBoolE("s")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "expected bool") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetDurationE(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetDurationE("d"); err != nil || got != 5*time.Second {
		t.Errorf("success: got (%v, %v)", got, err)
	}
	// Invalid duration text (just a plain string).
	_, err := cfg.GetDurationE("s")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "invalid duration") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetBytesE(t *testing.T) {
	cfg := mustParse142(t)
	if got, err := cfg.GetBytesE("bytes"); err != nil || got != 10*1024 {
		t.Errorf("success: got (%v, %v)", got, err)
	}
	// Negative byte size — S18.4 accessor invariant rejects.
	_, err := cfg.GetBytesE("neg_bytes")
	ce := asConfigError(t, err)
	if ce.Message != "byte size must not be negative" {
		t.Errorf("Message = %q", ce.Message)
	}
	// Invalid byte size text.
	_, err = cfg.GetBytesE("s")
	ce = asConfigError(t, err)
	if !contains(ce.Message, "invalid byte size") {
		t.Errorf("Message = %q", ce.Message)
	}
}

// ── slice accessors ──────────────────────────────────────────────────

func TestIssue142_GetStringSliceE(t *testing.T) {
	cfg := mustParse142(t)
	got, err := cfg.GetStringSliceE("strs")
	if err != nil {
		t.Fatalf("success: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v", got)
	}
	// non-array
	_, err = cfg.GetStringSliceE("s")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "expected array") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetInt64SliceE(t *testing.T) {
	cfg := mustParse142(t)
	got, err := cfg.GetInt64SliceE("arr")
	if err != nil {
		t.Fatalf("success: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("got %v", got)
	}
	// element-level conversion failure (mixed array has a string)
	_, err = cfg.GetInt64SliceE("mixed")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "element 1") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetIntSliceE(t *testing.T) {
	cfg := mustParse142(t)
	got, err := cfg.GetIntSliceE("arr")
	if err != nil {
		t.Fatalf("success: %v", err)
	}
	if len(got) != 3 || got[0] != 1 {
		t.Errorf("got %v", got)
	}
	// Underlying int64-slice failure surfaces.
	_, err = cfg.GetIntSliceE("mixed")
	_ = asConfigError(t, err)
}

func TestIssue142_GetStringSliceE_NullElement(t *testing.T) {
	cfg, err := hocon.ParseString(`xs = ["a", null]`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	_, gerr := cfg.GetStringSliceE("xs")
	ce := asConfigError(t, gerr)
	if !contains(ce.Message, "element 1 is not a non-null scalar") {
		t.Errorf("Message = %q", ce.Message)
	}
}

func TestIssue142_GetInt64SliceE_NullElement(t *testing.T) {
	cfg, err := hocon.ParseString(`xs = [1, null]`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	_, gerr := cfg.GetInt64SliceE("xs")
	ce := asConfigError(t, gerr)
	if !contains(ce.Message, "element 1 is not an int") {
		t.Errorf("Message = %q", ce.Message)
	}
}

// Empty array → empty slice, no error. Pins the boundary that an empty array
// is a successful lookup (matching the panic-getter contract, which would
// return a zero-length slice rather than panicking on len==0).
func TestIssue142_EmptyArray(t *testing.T) {
	cfg, err := hocon.ParseString(`xs = []`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	for _, c := range []struct {
		name string
		call func() (int, error)
	}{
		{"GetStringSliceE", func() (int, error) { s, e := cfg.GetStringSliceE("xs"); return len(s), e }},
		{"GetInt64SliceE", func() (int, error) { s, e := cfg.GetInt64SliceE("xs"); return len(s), e }},
		{"GetIntSliceE", func() (int, error) { s, e := cfg.GetIntSliceE("xs"); return len(s), e }},
		{"GetConfigSliceE", func() (int, error) { s, e := cfg.GetConfigSliceE("xs"); return len(s), e }},
	} {
		t.Run(c.name, func(t *testing.T) {
			n, err := c.call()
			if err != nil {
				t.Fatalf("empty array: %v", err)
			}
			if n != 0 {
				t.Errorf("empty array: got len=%d, want 0", n)
			}
		})
	}
}

// Nested-array element on a scalar-slice accessor must surface as a non-scalar
// element error (a `[[1]]` element is an ArrayVal, not a ScalarVal).
func TestIssue142_GetStringSliceE_NestedArrayElement(t *testing.T) {
	cfg, err := hocon.ParseString(`xs = [[1]]`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	_, gerr := cfg.GetStringSliceE("xs")
	ce := asConfigError(t, gerr)
	if !contains(ce.Message, "element 0 is not a non-null scalar") {
		t.Errorf("Message = %q", ce.Message)
	}
}

// ── object accessors ─────────────────────────────────────────────────

func TestIssue142_GetConfigE(t *testing.T) {
	cfg := mustParse142(t)
	sub, err := cfg.GetConfigE("obj")
	if err != nil {
		t.Fatalf("success: %v", err)
	}
	if got := sub.GetInt64("x"); got != 1 {
		t.Errorf("nested x = %d, want 1", got)
	}
	// non-object (scalar)
	_, err = cfg.GetConfigE("s")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "expected object") {
		t.Errorf("Message = %q", ce.Message)
	}
	// null — falls under "non-object" (parity with the GetConfig panic-getter).
	// GetConfigOption distinguishes null with a None return; GetConfigE folds
	// null into the same expected-object error to keep the panic-getter parity.
	_, err = cfg.GetConfigE("n")
	ce = asConfigError(t, err)
	if !contains(ce.Message, "expected object") {
		t.Errorf("null: Message = %q", ce.Message)
	}
}

func TestIssue142_GetConfigSliceE(t *testing.T) {
	cfg := mustParse142(t)
	cs, err := cfg.GetConfigSliceE("cfgs")
	if err != nil {
		t.Fatalf("success: %v", err)
	}
	if len(cs) != 2 || cs[0].GetInt64("k") != 1 || cs[1].GetInt64("k") != 2 {
		t.Errorf("got %v", cs)
	}
	// non-array
	_, err = cfg.GetConfigSliceE("s")
	_ = asConfigError(t, err)
	// non-object element
	_, err = cfg.GetConfigSliceE("arr")
	ce := asConfigError(t, err)
	if !contains(ce.Message, "element 0 is not an object") {
		t.Errorf("Message = %q", ce.Message)
	}
}

// ── S15 numeric-object → array conversion still applies on the E path ──

func TestIssue142_GetStringSliceE_NumericObjectConversion(t *testing.T) {
	// S15: a numerically-indexed object should convert to an array on the
	// E-accessor path too, matching the existing Get*Slice / Get*SliceOption
	// contract (lookupArray applies the same conversion).
	cfg, err := hocon.ParseString("xs { \"0\" = \"a\"\n\"1\" = \"b\" }")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, gerr := cfg.GetStringSliceE("xs")
	if gerr != nil {
		t.Fatalf("expected numeric-object conversion, got error: %v", gerr)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v", got)
	}
}

// Cover the err-passthrough branch in every E accessor: each must surface a
// missing-key error from lookupE rather than swallowing it. One parametric
// test for all 13 accessors keeps the coverage gate green without bloat.
func TestIssue142_AllAccessors_MissingKeyPassthrough(t *testing.T) {
	cfg := mustParse142(t)
	checks := []struct {
		name string
		call func() error
	}{
		{"GetStringE", func() error { _, e := cfg.GetStringE("nope"); return e }},
		{"GetInt64E", func() error { _, e := cfg.GetInt64E("nope"); return e }},
		{"GetIntE", func() error { _, e := cfg.GetIntE("nope"); return e }},
		{"GetFloat64E", func() error { _, e := cfg.GetFloat64E("nope"); return e }},
		{"GetFloat32E", func() error { _, e := cfg.GetFloat32E("nope"); return e }},
		{"GetBoolE", func() error { _, e := cfg.GetBoolE("nope"); return e }},
		{"GetDurationE", func() error { _, e := cfg.GetDurationE("nope"); return e }},
		{"GetBytesE", func() error { _, e := cfg.GetBytesE("nope"); return e }},
		{"GetStringSliceE", func() error { _, e := cfg.GetStringSliceE("nope"); return e }},
		{"GetInt64SliceE", func() error { _, e := cfg.GetInt64SliceE("nope"); return e }},
		{"GetIntSliceE", func() error { _, e := cfg.GetIntSliceE("nope"); return e }},
		{"GetConfigE", func() error { _, e := cfg.GetConfigE("nope"); return e }},
		{"GetConfigSliceE", func() error { _, e := cfg.GetConfigSliceE("nope"); return e }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			ce := asConfigError(t, c.call())
			if ce.Path != "nope" || ce.Message != "key not found" {
				t.Errorf("got Path=%q Message=%q", ce.Path, ce.Message)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
