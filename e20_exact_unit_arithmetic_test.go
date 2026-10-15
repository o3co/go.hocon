// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// E20 — unit arithmetic is exact decimal (xx.hocon docs/extra-spec-conventions.md).
// The numeric part of a duration / byte size is read from its source text as
// an exact decimal and multiplied by the unit exactly, then truncated toward
// zero (at the nanosecond for durations, at the byte for sizes). The
// units-default fixtures ud09–ud11 / ub07–ub08 pin the headline cases; this
// file covers number scalars (S18.1), negatives, sub-nanosecond truncation,
// overflow, the big Z/Y units, and that Unmarshal reads through the same path.

package hocon_test

import (
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

func TestE20_Duration_Exact(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{`"1.001s"`, 1001 * time.Millisecond},   // float64: 1000999999 ns
		{`"2.3h"`, 8280000 * time.Millisecond},  // float64: 8279999999999 ns
		{`"1.001"`, 1001000 * time.Nanosecond},  // S18.4 no unit = ms
		{`1.001`, 1001000 * time.Nanosecond},    // S18.1 number scalar, literal as written
		{`"-1.001s"`, -1001 * time.Millisecond}, // negative: truncate toward zero
		{`"0.1us"`, 100 * time.Nanosecond},
		{`"1.5ns"`, 1 * time.Nanosecond},                              // below 1 ns → truncate
		{`"-1.5ns"`, -1 * time.Nanosecond},                            // toward zero, not floor
		{`"0.3 m"`, 18 * time.Second},                                 // whitespace before the unit
		{`"106751.99116730064d"`, time.Duration(9223372036854775296)}, // near the int64 ceiling, exact
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			cfg := mustParseCfg(t, "t = "+tc.in)
			got, err := cfg.GetDurationE("t")
			if err != nil {
				t.Fatalf("GetDurationE: %v", err)
			}
			if got != tc.want {
				t.Errorf("GetDurationE = %d ns, want %d ns", got, tc.want)
			}
		})
	}
}

func TestE20_Duration_FractionalOverflowRejected(t *testing.T) {
	// 106752 days exceeds int64 nanoseconds. The float path converted the
	// out-of-range product with implementation-defined results.
	cfg := mustParseCfg(t, `t = "106752.5d"`)
	if d, err := cfg.GetDurationE("t"); err == nil {
		t.Errorf("GetDurationE = %d; want overflow error", d)
	}
	if cfg.GetDurationOption("t").IsSome() {
		t.Error("GetDurationOption = Some; want None")
	}
}

func TestE20_Bytes_Exact(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{`"1.001kB"`, 1001},  // float64: 1000
		{`"1.0009kB"`, 1000}, // truncate, not round
		{`"1.1KiB"`, 1126},   // 1126.4
		{`"2.3MB"`, 2300000},
		{`"0.001z"`, 1180591620717411303},     // 2^70 / 1000 = …303.424; float64: …328
		{`"0.000001ZB"`, 1000000000000000},    // 10^15 exactly
		{`"0.000000001Yi"`, 1208925819614629}, // 2^80 / 10^9 = …629.17
		{`"0.000000000001YB"`, 1000000000000}, // 10^12 exactly
		{`"7.999E"`, 9222219115350168961},     // float64: …168576
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			cfg := mustParseCfg(t, "b = "+tc.in)
			got, err := cfg.GetBytesE("b")
			if err != nil {
				t.Fatalf("GetBytesE: %v", err)
			}
			if got != tc.want {
				t.Errorf("GetBytesE = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestE20_Bytes_BigUnitOverflowRejected(t *testing.T) {
	for _, in := range []string{`"1ZB"`, `"0.5Zi"`, `"1Y"`, `"0.01YB"`} {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "b = "+in)
			if n, err := cfg.GetBytesE("b"); err == nil {
				t.Errorf("GetBytesE = %d; want overflow error", n)
			}
		})
	}
}

// TestE20_Unmarshal_SameReader asserts that Unmarshal reads through the same
// exact path as the getters (one reader per unit family).
func TestE20_Unmarshal_SameReader(t *testing.T) {
	type holder struct {
		T time.Duration  `hocon:"t"`
		B hocon.ByteSize `hocon:"b"`
	}
	cfg := mustParseCfg(t, "t = \"1.001s\"\nb = \"1.001kB\"")
	var h holder
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if h.T != 1001*time.Millisecond {
		t.Errorf("T = %d ns, want %d", h.T, 1001*time.Millisecond)
	}
	if h.B != 1001 {
		t.Errorf("B = %d, want 1001", h.B)
	}
}
