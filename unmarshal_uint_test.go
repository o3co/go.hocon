// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Unmarshal into unsigned integer kinds. They read exactly as the signed kinds
// do — a number or a numeric string, a whole-number float/exponent spelling,
// "-0" as 0 — with a negative value or one past the type's range an error
// naming the field.

package hocon_test

import (
	"math"
	"testing"
)

func TestUnmarshal_Uint_Accepted(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want uint16
	}{
		{"number", `a = 5`, 5},
		{"string", `a = "5"`, 5},
		{"whole float string", `a = "5.0"`, 5},
		{"whole float", `a = 5.0`, 5},
		{"exponent", `a = 1e2`, 100},
		{"plus sign", `a = "+5"`, 5},
		{"negative zero", `a = -0`, 0},
		{"negative zero float", `a = "-0.0"`, 0},
		{"max", `a = 65535`, 65535},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mustParseCfg(t, tc.src)
			var out struct{ A uint16 }
			if err := cfg.Unmarshal(&out); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if out.A != tc.want {
				t.Errorf("A = %d, want %d", out.A, tc.want)
			}
		})
	}
}

func TestUnmarshal_Uint_PlusAboveInt64(t *testing.T) {
	// A leading "+" keeps the whole uint64 range.
	cfg := mustParseCfg(t, `a = "+9223372036854775808", b = "+18446744073709551615"`)
	var out struct{ A, B uint64 }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.A != 9223372036854775808 || out.B != 18446744073709551615 {
		t.Errorf("got %d, %d", out.A, out.B)
	}
	var bad struct{ A uint64 }
	if err := mustParseCfg(t, `a = "+-5"`).Unmarshal(&bad); err == nil {
		t.Error(`"+-5" decoded without error`)
	}
}

func TestUnmarshal_Uint_AllKinds(t *testing.T) {
	// The parser refuses an unquoted integer past int64, so uint64's maximum
	// arrives as a string.
	cfg := mustParseCfg(t, `u = 7, u8 = 255, u16 = 65535, u32 = 4294967295, u64 = "18446744073709551615", ptr = 9`)
	var out struct {
		U   uint
		U8  uint8
		U16 uint16
		U32 uint32
		U64 uint64
		Ptr uintptr
	}
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.U != 7 || out.U8 != math.MaxUint8 || out.U16 != math.MaxUint16 ||
		out.U32 != math.MaxUint32 || out.U64 != math.MaxUint64 || out.Ptr != 9 {
		t.Errorf("out = %+v", out)
	}
}

func TestUnmarshal_Uint_EnvVar(t *testing.T) {
	// An environment variable substitutes a string; it coerces like a number.
	t.Setenv("HOCON_TEST_UINT_PORT", "8080")
	cfg := mustParseCfg(t, "port = 80\nport = ${?HOCON_TEST_UINT_PORT}")
	var out struct{ Port uint16 }
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.Port != 8080 {
		t.Errorf("Port = %d, want 8080", out.Port)
	}
}

func TestUnmarshal_Uint_Errors(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		target any
		want   string
	}{
		{"uint8 overflow", `a = 300`, &struct{ A uint8 }{},
			`hocon: field "a": hocon: uint 300 overflows uint8`},
		{"uint8 overflow from float", `a = "3e2"`, &struct{ A uint8 }{},
			`hocon: field "a": hocon: uint 300 overflows uint8`},
		{"negative", `a = -1`, &struct{ A uint16 }{},
			`hocon: field "a": hocon: int -1 overflows uint16`},
		{"negative string", `a = "-5"`, &struct{ A uint }{},
			`hocon: field "a": hocon: int -5 overflows uint`},
		{"negative float", `a = -1.0`, &struct{ A uint64 }{},
			`hocon: field "a": hocon: int -1 overflows uint64`},
		{"not whole", `a = 5.5`, &struct{ A uint16 }{},
			`hocon: field "a": hocon: expected uint, got "5.5"`},
		{"not a number", `a = five`, &struct{ A uint16 }{},
			`hocon: field "a": hocon: expected uint, got "five"`},
		{"past uint64", `a = "18446744073709551616"`, &struct{ A uint64 }{},
			`hocon: field "a": hocon: expected uint, got "18446744073709551616"`},
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

func TestUnmarshal_Uint_Null(t *testing.T) {
	// null leaves the field unchanged, as for every scalar.
	cfg := mustParseCfg(t, `a = null`)
	out := struct{ A uint32 }{A: 42}
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.A != 42 {
		t.Errorf("A = %d, want 42 (unchanged)", out.A)
	}
}

func TestUnmarshal_Uint_Containers(t *testing.T) {
	type Inner struct {
		Port uint16 `hocon:"port"`
	}
	cfg := mustParseCfg(t, `
		ptr = 5
		list = [1, "2", 3.0]
		ports { 80 = 8080, 443 = "8443" }
		inner { port = 9090 }
		inners = [{ port = 1 }]
	`)
	var out struct {
		Ptr    *uint8           `hocon:"ptr"`
		List   []uint32         `hocon:"list"`
		Ports  map[uint16]uint8 `hocon:"ports"`
		Inner  Inner            `hocon:"inner"`
		Inners []Inner          `hocon:"inners"`
	}
	// A uint8 value of 8080 overflows: check the failure first, then decode a
	// map whose values fit.
	err := cfg.Unmarshal(&out)
	want := `hocon: field "ports": hocon: map key "80": hocon: uint 8080 overflows uint8`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v\nwant    %q", err, want)
	}

	var ok struct {
		Ptr    *uint8            `hocon:"ptr"`
		List   []uint32          `hocon:"list"`
		Ports  map[uint16]uint16 `hocon:"ports"`
		Inner  Inner             `hocon:"inner"`
		Inners []Inner           `hocon:"inners"`
	}
	if err := cfg.Unmarshal(&ok); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if ok.Ptr == nil || *ok.Ptr != 5 {
		t.Errorf("Ptr = %v, want 5", ok.Ptr)
	}
	if len(ok.List) != 3 || ok.List[0] != 1 || ok.List[1] != 2 || ok.List[2] != 3 {
		t.Errorf("List = %v, want [1 2 3]", ok.List)
	}
	if len(ok.Ports) != 2 || ok.Ports[80] != 8080 || ok.Ports[443] != 8443 {
		t.Errorf("Ports = %v, want map[80:8080 443:8443]", ok.Ports)
	}
	if ok.Inner.Port != 9090 {
		t.Errorf("Inner.Port = %d, want 9090", ok.Inner.Port)
	}
	if len(ok.Inners) != 1 || ok.Inners[0].Port != 1 {
		t.Errorf("Inners = %v, want [{1}]", ok.Inners)
	}
}

func TestUnmarshal_Uint_SliceElementError(t *testing.T) {
	cfg := mustParseCfg(t, `list = [1, -2]`)
	var out struct {
		List []uint `hocon:"list"`
	}
	err := cfg.Unmarshal(&out)
	want := `hocon: field "list": hocon: slice element 1: hocon: int -2 overflows uint`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v\nwant    %q", err, want)
	}
}

func TestUnmarshalPath_Uint(t *testing.T) {
	cfg := mustParseCfg(t, `a { port = "443" }`)
	var port uint16
	if err := cfg.UnmarshalPath("a.port", &port); err != nil {
		t.Fatalf("UnmarshalPath error: %v", err)
	}
	if port != 443 {
		t.Errorf("port = %d, want 443", port)
	}
}
