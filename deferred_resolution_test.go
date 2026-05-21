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
