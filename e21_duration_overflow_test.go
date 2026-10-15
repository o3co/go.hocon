// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// E21 — a duration the implementation cannot represent is an error
// (xx.hocon docs/extra-spec-conventions.md). For go that is anything past
// ±2^63 ns (time.Duration). It is never wrapped and never saturated
// (Lightbend saturates — a deliberate divergence). The integer and
// fractional forms, quoted and unquoted, give the same answer. The shared
// fixtures ud12 / ud13 pass in go either way; these tests pin go's own ❌,
// the integer-path wrap.

package hocon_test

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestE21_Duration_OverflowRejected(t *testing.T) {
	inputs := []string{
		`"200000d"`,                // ~547 years; wrapped to a negative duration
		`200000d`,                  // unquoted, same value
		`"106752d"`,                // just past 2^63 ns; wrapped
		`"106752.0d"`,              // fractional form of the same value (E20 path)
		`"9223372036854775807"`,    // ms (S18.4); wrapped to -1ms
		`9223372036854775807`,      // number scalar (S18.1), same value
		`"9223372036854775808ns"`,  // 2^63 ns, one past the ceiling
		`"-9223372036854775809ns"`, // one past the floor
		`"-200000d"`,               // negative overflow
		`"9223372036854776us"`,     // one past MaxInt64/1000 µs
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "t = "+in)
			if d, err := cfg.GetDurationE("t"); err == nil {
				t.Errorf("GetDurationE = %v (%d ns); want overflow error", d, int64(d))
			}
			if cfg.GetDurationOption("t").IsSome() {
				t.Error("GetDurationOption = Some; want None")
			}
		})
	}
}

func TestE21_Duration_BoundaryAccepted(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{`"9223372036854775807ns"`, time.Duration(math.MaxInt64)},
		{`"-9223372036854775808ns"`, time.Duration(math.MinInt64)},
		{`"9223372036854775us"`, 9223372036854775 * time.Microsecond},
		{`"106751d"`, 106751 * 24 * time.Hour},
		{`"-106751d"`, -106751 * 24 * time.Hour},
		{`"9223372036854ms"`, 9223372036854 * time.Millisecond},
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

// TestE21_Duration_IntegerAndFractionalAgree: adding ".0" must not change the
// answer (the inconsistency that motivated E21).
func TestE21_Duration_IntegerAndFractionalAgree(t *testing.T) {
	for _, n := range []string{"106751", "106752", "200000", "-106752"} {
		t.Run(n, func(t *testing.T) {
			cfg := mustParseCfg(t, "i = \""+n+"d\"\nf = \""+n+".0d\"")
			di, ei := cfg.GetDurationE("i")
			df, ef := cfg.GetDurationE("f")
			if (ei == nil) != (ef == nil) || di != df {
				t.Errorf("%sd = %d, %v; %s.0d = %d, %v; want the same", n, di, ei, n, df, ef)
			}
		})
	}
}

// TestE21_Unmarshal_SameReader: Unmarshal into time.Duration reads through the
// same reader, so it rejects the same overflow and names the key.
func TestE21_Unmarshal_SameReader(t *testing.T) {
	type holder struct {
		T time.Duration `hocon:"timeout"`
	}
	cfg := mustParseCfg(t, `timeout = "200000d"`)
	var h holder
	err := cfg.Unmarshal(&h)
	if err == nil {
		t.Fatalf("Unmarshal = %v; want overflow error", h.T)
	}
	if want := `"timeout"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %s", err, want)
	}
}
