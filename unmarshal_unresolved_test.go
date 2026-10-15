// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Unmarshal on a config that still holds unresolved substitutions (a deferred
// parse, or Resolve with AllowUnresolved). Reading an unresolved value is an
// error wrapping ErrNotResolved and naming the key — never a silent nil, and
// never an internal type name — as UnmarshalPath and the getters already do.
// Resolved parts of the same config still decode.

package hocon_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

func partiallyResolved(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := hocon.ParseStringWithOptions(src, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	r, err := cfg.Resolve(hocon.DefaultResolveOptions().WithAllowUnresolved(true).WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return r
}

func assertNotResolved(t *testing.T, err error, key string) {
	t.Helper()
	if err == nil {
		t.Fatal("Unmarshal: want an error for an unresolved value, got nil")
	}
	if !errors.Is(err, hocon.ErrNotResolved) {
		t.Errorf("err = %v; want errors.Is(err, hocon.ErrNotResolved)", err)
	}
	if !strings.Contains(err.Error(), key) {
		t.Errorf("err = %v; want it to name %s", err, key)
	}
	if strings.Contains(err.Error(), "Placeholder") {
		t.Errorf("err = %v; must not expose the internal placeholder type", err)
	}
}

func TestUnmarshal_Unresolved_IntoAny(t *testing.T) {
	cases := []struct{ src, key string }{
		{"a = [1, ${y}]", `"a"`},       // inside an array
		{"b = ${z}", `"b"`},            // top-level scalar
		{"c { d = [${y}, 2] }", `"d"`}, // array inside an object
		{"e = [[${y}]]", `"e"`},        // nested arrays
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			cfg := partiallyResolved(t, tc.src)
			var m map[string]any
			assertNotResolved(t, cfg.Unmarshal(&m), tc.key)
			var v any
			assertNotResolved(t, cfg.Unmarshal(&v), tc.key)
		})
	}
}

func TestUnmarshal_Unresolved_IntoTypedFields(t *testing.T) {
	cfg := partiallyResolved(t, "a = [1, ${y}]\nb = ${z}\nc = 3\no = ${w}")
	var s1 struct {
		A []int `hocon:"a"`
	}
	assertNotResolved(t, cfg.Unmarshal(&s1), `"a"`)
	var s2 struct {
		B int `hocon:"b"`
	}
	assertNotResolved(t, cfg.Unmarshal(&s2), `"b"`)
	var s3 struct {
		O struct {
			X int `hocon:"x"`
		} `hocon:"o"`
	}
	assertNotResolved(t, cfg.Unmarshal(&s3), `"o"`)
	var s4 struct {
		M map[string]int `hocon:"o"`
	}
	assertNotResolved(t, cfg.Unmarshal(&s4), `"o"`)
	var s5 struct {
		L []int `hocon:"o"`
	}
	assertNotResolved(t, cfg.Unmarshal(&s5), `"o"`)
}

// TestUnmarshal_Unresolved_ResolvedPartsStillDecode: a struct that reads only
// resolved keys is unaffected by unresolved keys it does not read.
func TestUnmarshal_Unresolved_ResolvedPartsStillDecode(t *testing.T) {
	cfg := partiallyResolved(t, "a { x = ${v} }\nv = 1\nb = ${missing}")
	var s struct {
		A struct {
			X int `hocon:"x"`
		} `hocon:"a"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s.A.X != 1 {
		t.Errorf("A.X = %d, want 1", s.A.X)
	}
}

// TestUnmarshal_Unresolved_Shapes runs every target shape against both kinds
// of unresolved config: a deferred parse never resolved, and one resolved
// with AllowUnresolved.
func TestUnmarshal_Unresolved_Shapes(t *testing.T) {
	deferred := func(t *testing.T, src string) *hocon.Config {
		t.Helper()
		cfg, err := hocon.ParseStringWithOptions(src, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
		if err != nil {
			t.Fatalf("ParseStringWithOptions: %v", err)
		}
		return cfg
	}
	kinds := map[string]func(*testing.T, string) *hocon.Config{
		"deferred":         deferred,
		"allow-unresolved": partiallyResolved,
	}
	type tcase struct {
		name, src, key string
		target         func() any
	}
	cases := []tcase{
		{"pointer", "p = ${y}", `"p"`, func() any { return &struct{ P *int }{} }},
		{"duration", "d = ${y}", `"d"`, func() any {
			return &struct {
				D time.Duration `hocon:"d"`
			}{}
		}},
		{"bytesize", "b = ${y}", `"b"`, func() any {
			return &struct {
				B hocon.ByteSize `hocon:"b"`
			}{}
		}},
		{"period", "p = ${y}", `"p"`, func() any {
			return &struct {
				P hocon.Period `hocon:"p"`
			}{}
		}},
		{"concat string", "s = ${y} foo", `"s"`, func() any {
			return &struct {
				S string `hocon:"s"`
			}{}
		}},
		{"map element", "m { a = 1, b = ${y} }", `"b"`, func() any {
			return &struct {
				M map[string]int `hocon:"m"`
			}{}
		}},
		{"numeric object to slice", "l { 0 = 1, 1 = ${y} }", "slice element 1", func() any {
			return &struct {
				L []int `hocon:"l"`
			}{}
		}},
		{"any in struct", "o { a = [ { b = ${y} } ] }", `"b"`, func() any {
			return &struct {
				O any `hocon:"o"`
			}{}
		}},
	}
	for kname, load := range kinds {
		for _, tc := range cases {
			t.Run(kname+"/"+tc.name, func(t *testing.T) {
				assertNotResolved(t, load(t, tc.src).Unmarshal(tc.target()), tc.key)
			})
		}
	}
}

// TestUnmarshal_Unresolved_TypeMismatchStaysTypeMismatch: an object holding a
// placeholder, read into an int, is a type error — resolving would not help.
func TestUnmarshal_Unresolved_TypeMismatchStaysTypeMismatch(t *testing.T) {
	cfg := partiallyResolved(t, "s { a = ${y} }")
	var v struct {
		S int `hocon:"s"`
	}
	err := cfg.Unmarshal(&v)
	if err == nil {
		t.Fatal("Unmarshal: want a type-mismatch error")
	}
	if errors.Is(err, hocon.ErrNotResolved) {
		t.Errorf("err = %v; a type mismatch must not be reported as not resolved", err)
	}
}
