// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// E20 exponent forms (xx.hocon#111): the number part of a duration or byte
// size is a full number, exponent included — Java's double grammar
// [+-]?(digits(.digits*)?|.digits)([eE][+-]?digits)? — and the unit is the
// trailing run of letters (Lightbend getUnits). The value stays exact (E20);
// a huge exponent never builds 10^N: it is an overflow error (E21 / S21.4)
// or 0 straight away. Values measured on Lightbend 1.4.6.

package hocon_test

import (
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

func TestE20Exp_Duration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{`"1e+5ms"`, 100000 * time.Millisecond}, // ud14
		{`"1.5e2s"`, 150000 * time.Millisecond}, // ud15
		{`"1E3"`, 1000 * time.Millisecond},      // ud16: no unit = ms
		{`1e3`, 1000 * time.Millisecond},        // ud17: NUMBER (S18.1)
		{`"1e-3s"`, time.Millisecond},           // ud18
		{`"-1e3ms"`, -1000 * time.Millisecond},
		{`"1e3 ms"`, 1000 * time.Millisecond},
		{`"1.e3ms"`, 1000 * time.Millisecond},
		{`".5s"`, 500 * time.Millisecond},
		{`"-.5s"`, -500 * time.Millisecond},
		{`"1e-400s"`, 0},       // below 1 ns → truncates to 0
		{`"1e-999999999s"`, 0}, // fast: no 10^N built
		{`"0e999999999s"`, 0},  // a zero mantissa is 0 for any exponent
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			cfg := mustParseCfg(t, "t = "+tc.in)
			got, err := cfg.GetDurationE("t")
			if err != nil || got != tc.want {
				t.Errorf("GetDurationE = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestE20Exp_DurationRejects(t *testing.T) {
	for _, in := range []string{
		`"1e400s"`,       // ud19: past int64 ns (E21; Lightbend saturates)
		`"1e999999999s"`, // fast overflow, no 10^N built
		`"-1e400s"`,
		`"1e3.5ms"`, // ud20: not a number
		`"1e3e"`,    // unit "e" is not a duration unit
		`"1e"`,      // unit "e"
		`"1e+ms"`,   // exponent without digits
		`"e3ms"`,    // "e3" is not a number (unit "ms")
	} {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "t = "+in)
			if d, err := cfg.GetDurationE("t"); err == nil {
				t.Errorf("GetDurationE = %d; want an error", d)
			}
		})
	}
}

func TestE20Exp_Bytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{`"1e3kB"`, 1000000},    // ub09
		{`"1e3"`, 1000},         // ub10: no unit = bytes
		{`"2E2KiB"`, 204800},    // ub11
		{`"1e-3kB"`, 1},         // ub12
		{`"1.0005e3B"`, 1000},   // ub12: 1000.5 truncates
		{`"1E"`, 1 << 60},       // E alone is the exa unit, not an exponent
		{`"1e-999999999kB"`, 0}, // fast: no 10^N built
		{`"1e-30Yi"`, 0},        // below 1 byte even with the largest unit
		{`"1e-6ZB"`, 1000000000000000},
		{`"1e-3e"`, 1152921504606846}, // unit e = 2^60 (Lightbend getUnits); 1e-3 × 2^60 truncated
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			cfg := mustParseCfg(t, "b = "+tc.in)
			got, err := cfg.GetBytesE("b")
			if err != nil || got != tc.want {
				t.Errorf("GetBytesE = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestE20Exp_BytesRejects(t *testing.T) {
	for _, in := range []string{
		`"1e400B"`,        // ub13: out of range (S21.4)
		`"1e999999999kB"`, // fast overflow
		`"1e3E"`,          // 1000 exabytes overflows
		`"1e3.5kB"`,       // not a number
	} {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "b = "+in)
			if n, err := cfg.GetBytesE("b"); err == nil {
				t.Errorf("GetBytesE = %d; want an error", n)
			}
		})
	}
}

// TestE20Exp_UnmarshalSameReader: Unmarshal reads exponent forms through the
// same readers as the getters.
func TestE20Exp_UnmarshalSameReader(t *testing.T) {
	type holder struct {
		T time.Duration  `hocon:"t"`
		B hocon.ByteSize `hocon:"b"`
	}
	cfg := mustParseCfg(t, "t = \"1e+5ms\"\nb = \"1e3kB\"")
	var h holder
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if h.T != 100*time.Second || h.B != 1000000 {
		t.Errorf("Unmarshal = %v, %d; want 1m40s, 1000000", h.T, h.B)
	}
}

// TestE20Exp_PeriodUnchanged: a period is integer-only; an exponent is an
// error, as in Lightbend.
func TestE20Exp_PeriodUnchanged(t *testing.T) {
	cfg := mustParseCfg(t, `p = "1e2d"`)
	if p, err := cfg.GetPeriodE("p"); err == nil {
		t.Errorf("GetPeriodE = %+v; want an error", p)
	}
}
