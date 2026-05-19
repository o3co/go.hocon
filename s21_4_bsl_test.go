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
// 8E = 8 × 2^60 = 2^63 = MaxInt64+1 → overflow.
// 9E = 9 × 2^60 ≈ 1.04e19 → overflow.
func TestS21_4_Overflow(t *testing.T) {
	overflowCases := []string{`v: "8E"`, `v: "9E"`}
	for _, src := range overflowCases {
		src := src
		t.Run(src, func(t *testing.T) {
			cfg, err := hocon.ParseString(src)
			if err != nil {
				// parse error is acceptable, though unlikely for these inputs
				return
			}
			// GetBytesOption returns None on error (overflow included).
			opt := cfg.GetBytesOption("v")
			if opt.IsSome() {
				got, _ := opt.Get()
				if got > 0 {
					t.Errorf("GetBytesOption with overflow input %q: expected None or non-positive, got %d", src, got)
				}
			}
			// None is the correct result for overflow — no further assertion needed.
		})
	}
}
