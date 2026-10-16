// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Unmarshal into a map whose key type is not plain string. A HOCON key is
// always a string; it converts to a string kind, parses strictly into an
// integer or bool kind, and any other key type is an error — never a panic
// (reflect.Value.SetMapIndex used to panic on the unconverted key).

package hocon_test

import (
	"strings"
	"testing"
)

func TestUnmarshal_MapKey_Int(t *testing.T) {
	cfg := mustParseCfg(t, `a { 1 = x, "-2" = y }`)
	var out struct{ A map[int]string }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(out.A) != 2 || out.A[1] != "x" || out.A[-2] != "y" {
		t.Errorf("A = %v, want map[-2:y 1:x]", out.A)
	}
}

func TestUnmarshal_MapKey_Int64(t *testing.T) {
	cfg := mustParseCfg(t, `a { "9223372036854775807" = 1, "-9223372036854775808" = 2 }`)
	var out struct{ A map[int64]int }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.A[9223372036854775807] != 1 || out.A[-9223372036854775808] != 2 {
		t.Errorf("A = %v", out.A)
	}
}

func TestUnmarshal_MapKey_Uint16(t *testing.T) {
	cfg := mustParseCfg(t, `a { 80 = http, 65535 = top }`)
	var out struct{ A map[uint16]string }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(out.A) != 2 || out.A[80] != "http" || out.A[65535] != "top" {
		t.Errorf("A = %v, want map[80:http 65535:top]", out.A)
	}
}

func TestUnmarshal_MapKey_NamedString(t *testing.T) {
	type Key string
	cfg := mustParseCfg(t, `a { k = 5, "with space" = 6 }`)
	var out struct{ A map[Key]int }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(out.A) != 2 || out.A["k"] != 5 || out.A["with space"] != 6 {
		t.Errorf("A = %v, want map[k:5 with space:6]", out.A)
	}
}

func TestUnmarshal_MapKey_NamedStringAnyValue(t *testing.T) {
	// The interface-element branch takes the converted key too.
	type Key string
	cfg := mustParseCfg(t, `k = 5, n = null`)
	out := map[Key]any{}
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out["k"] != int64(5) {
		t.Errorf("k = %v (%T), want int64 5", out["k"], out["k"])
	}
	if v, ok := out["n"]; !ok || v != nil {
		t.Errorf("n = %v (present %v), want a nil entry", v, ok)
	}
}

func TestUnmarshal_MapKey_Bool(t *testing.T) {
	// A bool key accepts the spellings a bool value does.
	cfg := mustParseCfg(t, `a { true = on-value, no = off-value }`)
	var out struct{ A map[bool]string }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(out.A) != 2 || out.A[true] != "on-value" || out.A[false] != "off-value" {
		t.Errorf("A = %v, want map[false:off-value true:on-value]", out.A)
	}
}

func TestUnmarshal_MapKey_StringAndAnyUnchanged(t *testing.T) {
	// Key types that a string is assignable to keep decoding as before.
	cfg := mustParseCfg(t, `a { 1 = x }`)
	var s struct{ A map[string]string }
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("map[string]: %v", err)
	}
	if s.A["1"] != "x" {
		t.Errorf("map[string]: A = %v", s.A)
	}
	var a struct{ A map[any]string }
	if err := cfg.Unmarshal(&a); err != nil {
		t.Fatalf("map[any]: %v", err)
	}
	if a.A["1"] != "x" {
		t.Errorf("map[any]: A = %v (key is the string \"1\")", a.A)
	}
}

func TestUnmarshal_MapKey_Errors(t *testing.T) {
	type structKey struct{ X int }
	cases := []struct {
		name   string
		src    string
		target any
		want   string
	}{
		{"int not a number", `a { x = 1 }`, &struct{ A map[int]int }{},
			`hocon: field "a": hocon: map key "x": cannot parse as int`},
		{"int not whole", `a { "1.0" = 1 }`, &struct{ A map[int]int }{},
			`hocon: field "a": hocon: map key "1.0": cannot parse as int`},
		{"int8 overflow", `a { 300 = 1 }`, &struct{ A map[int8]int }{},
			`hocon: field "a": hocon: map key "300": cannot parse as int8`},
		{"uint16 negative", `a { "-1" = 1 }`, &struct{ A map[uint16]int }{},
			`hocon: field "a": hocon: map key "-1": cannot parse as uint16`},
		{"uint16 overflow", `a { 65536 = 1 }`, &struct{ A map[uint16]int }{},
			`hocon: field "a": hocon: map key "65536": cannot parse as uint16`},
		{"bool", `a { maybe = 1 }`, &struct{ A map[bool]int }{},
			`hocon: field "a": hocon: map key "maybe": cannot parse as bool`},
		{"struct key", `a { k = 1 }`, &struct{ A map[structKey]int }{},
			`hocon: field "a": hocon: map key "k": unsupported map key type hocon_test.structKey`},
		{"float key", `a { "1.5" = 1 }`, &struct{ A map[float64]int }{},
			`hocon: field "a": hocon: map key "1.5": unsupported map key type float64`},
		{"non-empty interface key", `a { k = 1 }`, &struct{ A map[error]int }{},
			`hocon: field "a": hocon: map key "k": unsupported map key type error`},
		{"nested map path", `a { b { x = 1 } }`, &struct{ A map[string]map[int]int }{},
			`hocon: field "a": hocon: map key "b": hocon: map key "x": cannot parse as int`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mustParseCfg(t, tc.src)
			err := cfg.Unmarshal(tc.target)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q\nwant    %q", err, tc.want)
			}
		})
	}
}

func TestUnmarshal_MapKey_EmptyObjectUnsupportedKey(t *testing.T) {
	// An unsupported key type only fails on a key it has to convert: an empty
	// object still decodes to an empty map, as it did before.
	type structKey struct{ X int }
	cfg := mustParseCfg(t, `a {}`)
	var out struct{ A map[structKey]int }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.A == nil || len(out.A) != 0 {
		t.Errorf("A = %v, want an empty map", out.A)
	}
}

func TestUnmarshalPath_MapKey_Int(t *testing.T) {
	cfg := mustParseCfg(t, `ports { 80 = http, x = bad }`)
	var out map[int]string
	err := cfg.UnmarshalPath("ports", &out)
	if err == nil || !strings.Contains(err.Error(), `hocon: path "ports": hocon: map key "x": cannot parse as int`) {
		t.Fatalf("err = %v, want the path and the key named", err)
	}
}
