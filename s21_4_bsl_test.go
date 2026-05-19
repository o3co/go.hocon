// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S21.4 — single-letter byte abbreviations (K/k/M/m/G/g/T/t/P/p/E/e) must map
// to powers of two, following the java -Xmx convention (HOCON.md L1385).
// Fixtures: testdata/hocon/byte-single-letter/bsl01-bsl09 (from xx.hocon, SHA 5beedfa).
// Expected: testdata/expected/byte-single-letter/bsl0N-expected.json (raw string,
// e.g. {"b": "1K"}). Per-fixture GetBytes() assertions use spec'd byte counts.
//
// Note: expected JSON sidecars contain the raw string (b = "1K"), not the
// integer. GetBytes() is the impl-side assertion for the byte count.

package hocon_test

import (
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// bslFixture describes a byte-single-letter test case.
type bslFixture struct {
	name      string
	wantBytes int64
}

// bslFixtures lists bsl01-bsl09 with expected GetBytes("b") values per HOCON.md L1385.
var bslFixtures = []bslFixture{
	{"bsl01-1K", 1024},
	{"bsl02-1k", 1024},
	{"bsl03-1M", 1 << 20},
	{"bsl04-1G", 1 << 30},
	{"bsl05-1T", 1 << 40},
	{"bsl06-1P", 1 << 50},
	{"bsl07-1E", 1 << 60},
	{"bsl08-1024K", 1024 * 1024},
	{"bsl09-05K", 512},
}

// TestS21_4_BSL_GetBytes asserts that each byte-single-letter fixture parses and
// GetBytes("b") returns the expected binary byte count.
func TestS21_4_BSL_GetBytes(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "hocon", "byte-single-letter")
	if _, err := os.Stat(fixtureDir); err != nil {
		t.Skipf("byte-single-letter fixtures missing at %s; run `make testdata`", fixtureDir)
	}

	for _, fx := range bslFixtures {
		fx := fx
		t.Run(fx.name, func(t *testing.T) {
			confPath := filepath.Join(fixtureDir, fx.name+".conf")
			cfg, err := hocon.ParseFile(confPath)
			if err != nil {
				t.Fatalf("ParseFile(%s): %v", confPath, err)
			}
			got := cfg.GetBytes("b")
			if got != fx.wantBytes {
				t.Errorf("GetBytes(\"b\") = %d, want %d", got, fx.wantBytes)
			}
		})
	}
}

// TestS21_4_Overflow asserts that values exceeding int64 max produce an error
// via GetBytesOption (returns None) rather than a corrupted positive value.
//
// Boundary cases for the float64 precision fix (go-I1 / go-T1 convergent):
//   - "8E"  = 8×2^60 = 2^63 exactly  → overflow (boundary, must error)
//   - "8.0E"= 8.0×2^60 = 2^63 exactly → overflow (float path, must error)
//   - "8.001E"                         → overflow (above boundary)
//   - "10.0E"                          → overflow
//   - "9E"  = 9×2^60 ≈ 1.04e19        → overflow
//
// Non-overflow cases for the same boundary:
//   - "7E"  = 7×2^60 = 8070450532247928832 → success
//   - "7.999E"                              → success (just below 2^63)
func TestS21_4_Overflow(t *testing.T) {
	t.Run("overflow_cases", func(t *testing.T) {
		overflowCases := []string{
			`v: "8E"`,
			`v: "8.0E"`,
			`v: "8.001E"`,
			`v: "10.0E"`,
			`v: "9E"`,
		}
		for _, src := range overflowCases {
			src := src
			t.Run(src, func(t *testing.T) {
				cfg, err := hocon.ParseString(src)
				if err != nil {
					// parse error is acceptable
					return
				}
				opt := cfg.GetBytesOption("v")
				if opt.IsSome() {
					got, _ := opt.Get()
					// Strictly: any integer value here is a bug — overflow must not
					// produce a positive or wrapped result.
					t.Errorf("GetBytesOption with overflow input %q: expected None, got %d", src, got)
				}
			})
		}
	})

	t.Run("non_overflow_cases", func(t *testing.T) {
		type tc struct {
			src  string
			want int64
		}
		// 7E = 7 * 2^60
		const exp2_60 = int64(1) << 60
		cases := []tc{
			{`v: "7E"`, 7 * exp2_60},                              // = 8070450532247928832
			{`v: "7.999E"`, int64(7.999 * float64(int64(1)<<60))}, // below 2^63
		}
		for _, c := range cases {
			c := c
			t.Run(c.src, func(t *testing.T) {
				cfg, err := hocon.ParseString(c.src)
				if err != nil {
					t.Fatalf("ParseString(%q): unexpected error %v", c.src, err)
				}
				opt := cfg.GetBytesOption("v")
				if !opt.IsSome() {
					t.Errorf("GetBytesOption(%q): expected Some, got None", c.src)
					return
				}
				got, _ := opt.Get()
				if got != c.want {
					t.Errorf("GetBytesOption(%q) = %d, want %d", c.src, got, c.want)
				}
			})
		}
	})
}
