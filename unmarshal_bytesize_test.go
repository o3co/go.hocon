// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Unmarshal into hocon.ByteSize (xx.hocon#102): a byte-size value read through
// Unmarshal follows exactly the rules of GetBytes — same accepted forms, bare
// number = bytes, non-negative, fractional truncated, case-sensitive units —
// and a failure names the key path like Unmarshal's other decode errors.

package hocon_test

import (
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// TestUnmarshal_ByteSize_ParityWithGetBytesE asserts that Unmarshal into a
// ByteSize field and GetBytesE agree on every input: both succeed with the same
// count, or both fail. null is the one deliberate exception (Unmarshal's
// scalar rule: field left unchanged) — see TestUnmarshal_ByteSize_Null.
func TestUnmarshal_ByteSize_ParityWithGetBytesE(t *testing.T) {
	inputs := []string{
		`1024`,                  // bare number scalar → bytes
		`"1024"`,                // bare number string → bytes
		`" 1024 "`,              // HOCON whitespace around the number
		`64KiB`,                 // binary unit
		`"512 MiB"`,             // whitespace between number and unit
		`10kB`,                  // decimal kilo
		`64k`,                   // single-letter -Xmx form → power of two
		`"1.5K"`,                // fractional → truncated after multiply
		`"1024.9"`,              // fractional bare number → truncated
		`0`,                     // zero
		`-1`,                    // negative → rejected
		`"-5K"`,                 // negative with unit → rejected
		`10KB`,                  // wrong case (Lightbend spells it kB) → rejected
		`10kb`,                  // wrong case → rejected
		`"10KX"`,                // unknown unit → rejected
		`"not-a-size"`,          // no number → rejected
		`""`,                    // empty → rejected
		`true`,                  // boolean → rejected
		`"8E"`,                  // overflows int64 → rejected
		`"9223372036854775807"`, // int64 max
	}
	type holder struct {
		B hocon.ByteSize `hocon:"b"`
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "b = "+in)
			want, wantErr := cfg.GetBytesE("b")

			var h holder
			gotErr := cfg.Unmarshal(&h)

			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("GetBytesE err = %v, Unmarshal err = %v; want both nil or both non-nil", wantErr, gotErr)
			}
			if wantErr != nil {
				return
			}
			if int64(h.B) != want {
				t.Errorf("Unmarshal ByteSize = %d, GetBytesE = %d", int64(h.B), want)
			}
		})
	}
}

func TestUnmarshal_ByteSize_Values(t *testing.T) {
	type limits struct {
		MaxBody hocon.ByteSize `hocon:"max-body"`
		Chunk   hocon.ByteSize `hocon:"chunk"`
		Plain   hocon.ByteSize `hocon:"plain"`
	}
	cfg := mustParseCfg(t, "max-body = 64KiB\nchunk = \"1.5K\"\nplain = 512")
	var l limits
	if err := cfg.Unmarshal(&l); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if l.MaxBody != 64*1024 {
		t.Errorf("MaxBody = %d, want %d", l.MaxBody, 64*1024)
	}
	if l.Chunk != 1536 {
		t.Errorf("Chunk = %d, want 1536", l.Chunk)
	}
	if l.Plain != 512 {
		t.Errorf("Plain = %d, want 512", l.Plain)
	}
}

func TestUnmarshal_ByteSize_Containers(t *testing.T) {
	type holder struct {
		List []hocon.ByteSize          `hocon:"list"`
		Map  map[string]hocon.ByteSize `hocon:"map"`
		Ptr  *hocon.ByteSize           `hocon:"ptr"`
		Nil  *hocon.ByteSize           `hocon:"nil"`
	}
	cfg := mustParseCfg(t, "list = [1k, 2k]\nmap { a = 1MiB }\nptr = 4k\nnil = null")
	var h holder
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(h.List) != 2 || h.List[0] != 1024 || h.List[1] != 2048 {
		t.Errorf("List = %v, want [1024 2048]", h.List)
	}
	if h.Map["a"] != 1<<20 {
		t.Errorf("Map[a] = %d, want %d", h.Map["a"], 1<<20)
	}
	if h.Ptr == nil || *h.Ptr != 4096 {
		t.Errorf("Ptr = %v, want 4096", h.Ptr)
	}
	if h.Nil != nil {
		t.Errorf("Nil = %v, want nil", *h.Nil)
	}
}

func TestUnmarshalPath_ByteSize_Scalar(t *testing.T) {
	cfg := mustParseCfg(t, "server { max-body = 2MiB }")
	var b hocon.ByteSize
	if err := cfg.UnmarshalPath("server.max-body", &b); err != nil {
		t.Fatalf("UnmarshalPath: %v", err)
	}
	if b != 2<<20 {
		t.Errorf("b = %d, want %d", b, 2<<20)
	}
}

// TestUnmarshal_ByteSize_ErrorCarriesPath asserts that a byte-size failure
// names every key on the way down, and the getter's reason.
func TestUnmarshal_ByteSize_ErrorCarriesPath(t *testing.T) {
	type limits struct {
		MaxBody hocon.ByteSize `hocon:"max-body"`
	}
	type root struct {
		Limits limits `hocon:"limits"`
	}
	cases := []struct {
		name, conf, reason string
	}{
		{"negative", `limits { max-body = -1 }`, "byte size must not be negative"},
		{"invalid", `limits { max-body = 10KB }`, "invalid byte size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mustParseCfg(t, tc.conf)
			var r root
			err := cfg.Unmarshal(&r)
			if err == nil {
				t.Fatal("Unmarshal: expected error, got nil")
			}
			msg := err.Error()
			for _, want := range []string{`"limits"`, `"max-body"`, tc.reason} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
		})
	}
}

// TestUnmarshal_ByteSize_Null pins the one deliberate difference from GetBytesE:
// null leaves a ByteSize field unchanged, as for every scalar in Unmarshal,
// while the getter returns an error.
func TestUnmarshal_ByteSize_Null(t *testing.T) {
	type holder struct {
		B hocon.ByteSize `hocon:"b"`
	}
	cfg := mustParseCfg(t, "b = null")
	if _, err := cfg.GetBytesE("b"); err == nil {
		t.Error("GetBytesE on null: want error")
	}
	h := holder{B: 7}
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if h.B != 7 {
		t.Errorf("B = %d, want 7 (unchanged)", h.B)
	}
}
