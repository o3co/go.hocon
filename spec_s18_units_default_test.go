// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package hocon_test — S18.4 units-default conformance tests (Phase 6 #3d).
// Drives xx.hocon fixtures from testdata/hocon/units-default/ against
// GetDurationOption / GetBytesOption and asserts accessor-output values.
//
// No expected sidecars are used — per-impl tests carry assertion burden
// (xx.hocon accessor-time output capture deferred to S13c generator extension).
//
// Fixture families:
//   - ud01-ud11: duration fixtures — asserts GetDurationOption("t")
//   - ub01-ub08: bytes fixtures — asserts GetBytesOption("b")
//   - ud09-ud11, ub07-ub08: E20 exact-decimal unit arithmetic
//   - un01-un03: negative-case fixtures — asserts IsNone()
//   - ud12-ud13: E21 duration overflow — asserts IsNone()
//   - up01-up05: period fixtures — asserts GetPeriodE("p")
package hocon_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/o3co/go.hocon"
)

const udFixtureDir = "testdata/hocon/units-default"

// TestUnitsDefault_Duration drives ud01–ud11 fixtures.
func TestUnitsDefault_Duration(t *testing.T) {
	type dcase struct {
		stem string
		key  string
		want time.Duration
	}
	cases := []dcase{
		// ud01: bare number string — default ms
		{"ud01-duration-bare", "t", 500 * time.Millisecond},
		// ud02: leading whitespace
		{"ud02-duration-leading-ws", "t", 500 * time.Millisecond},
		// ud03: trailing whitespace
		{"ud03-duration-trailing-ws", "t", 500 * time.Millisecond},
		// ud04: leading + trailing whitespace
		{"ud04-duration-both-ws", "t", 500 * time.Millisecond},
		// ud05: fractional — Lightbend double*nanos, 500.5ms = 500_500_000 ns
		{"ud05-duration-fractional", "t", time.Duration(500_500_000)},
		// ud06: negative bare number — -500ms
		{"ud06-duration-negative", "t", -500 * time.Millisecond},
		// ud07: explicit "ms" unit (regression guard — S18.2 still works)
		{"ud07-duration-with-unit", "t", 500 * time.Millisecond},
		// ud08: whitespace between number and unit (regression guard — S18.2)
		{"ud08-duration-ws-between", "t", 500 * time.Millisecond},
		// ud09–ud11 (E20): exact decimal × unit, then truncate at the ns.
		// Lightbend's double path gives 1000999999 ns / 8279999999999 ns /
		// 1000999 ns — a deliberate divergence.
		{"ud09-duration-fractional-exact", "t", 1001 * time.Millisecond},
		{"ud10-duration-fractional-exact-hours", "t", 8280000 * time.Millisecond},
		{"ud11-duration-fractional-exact-no-unit", "t", 1001000 * time.Nanosecond},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.stem, func(t *testing.T) {
			confPath := filepath.Join(udFixtureDir, tc.stem+".conf")
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("fixture not found — run `make testdata` first: %s", confPath)
			}
			src, err := os.ReadFile(confPath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			cfg, err := hocon.ParseString(string(src))
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			got, ok := cfg.GetDurationOption(tc.key).Get()
			if !ok || got != tc.want {
				t.Errorf("%s: GetDurationOption(%q) = ok=%v val=%v; want %v",
					tc.stem, tc.key, ok, got, tc.want)
			}
		})
	}
}

// TestUnitsDefault_Bytes drives ub01–ub08 fixtures.
func TestUnitsDefault_Bytes(t *testing.T) {
	type bcase struct {
		stem      string
		key       string
		wantSome  bool
		wantValue int64
	}
	cases := []bcase{
		// ub01: bare number string — default bytes
		{"ub01-bytes-bare", "b", true, 1024},
		// ub02: leading + trailing whitespace
		{"ub02-bytes-leading-trailing-ws", "b", true, 1024},
		// ub03: fractional — Lightbend truncate toward zero
		{"ub03-bytes-fractional-truncated", "b", true, 1024},
		// ub04: negative — accessor rejects (returns None)
		{"ub04-bytes-negative-accessor-rejects", "b", false, 0},
		// ub05: "1024K" — single-letter alias. S21.4 fixed in cluster 3h.
		// 1024K = 1024 × 2^10 = 2^20 = 1048576 (Lightbend-verified, HOCON.md L1385).
		{"ub05-bytes-with-unit", "b", true, 1024 * 1024},
		// ub06: empty string — parse error → None
		{"ub06-bytes-empty-rejected", "b", false, 0},
		// ub07 (E20): 1.0009kB = 1000.9 bytes → truncate (not round) → 1000
		{"ub07-bytes-fractional-unit-truncated", "b", true, 1000},
		// ub08 (E20): 1.001kB exact = 1001 (float64 gives 1000.9999999999999 → 1000)
		{"ub08-bytes-fractional-unit-exact", "b", true, 1001},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.stem, func(t *testing.T) {
			confPath := filepath.Join(udFixtureDir, tc.stem+".conf")
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("fixture not found — run `make testdata` first: %s", confPath)
			}
			src, err := os.ReadFile(confPath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			cfg, err := hocon.ParseString(string(src))
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			opt := cfg.GetBytesOption(tc.key)
			if tc.wantSome {
				got, ok := opt.Get()
				if !ok || got != tc.wantValue {
					t.Errorf("%s: GetBytesOption(%q) = ok=%v val=%d; want %d",
						tc.stem, tc.key, ok, got, tc.wantValue)
				}
			} else {
				if opt.IsSome() {
					got, _ := opt.Get()
					t.Errorf("%s: GetBytesOption(%q) = Some(%d); want None",
						tc.stem, tc.key, got)
				}
			}
		})
	}
}

// TestUnitsDefault_Negative drives un01–un03 and the E21 ud12–ud13 fixtures.
// All three must return None from GetDurationOption (parse error: no valid number).
func TestUnitsDefault_Negative(t *testing.T) {
	cases := []string{
		"un01-empty-duration",
		"un02-ws-only-duration",
		"un03-unit-only-duration",
		// E21: a duration past int64 ns is an error — integer form (ud12,
		// Lightbend parity) and fractional form (ud13, Lightbend saturates).
		"ud12-duration-overflow-rejected",
		"ud13-duration-overflow-fractional-rejected",
	}
	for _, stem := range cases {
		stem := stem
		t.Run(stem, func(t *testing.T) {
			confPath := filepath.Join(udFixtureDir, stem+".conf")
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("fixture not found — run `make testdata` first: %s", confPath)
			}
			src, err := os.ReadFile(confPath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			cfg, err := hocon.ParseString(string(src))
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			if cfg.GetDurationOption("t").IsSome() {
				t.Errorf("%s: GetDurationOption(\"t\") = Some; want None (parse error)", stem)
			}
		})
	}
}

// TestUnitsDefault_Period drives up01–up05 fixtures. Period is integer-only,
// so the fractional up03 is an error (unlike duration/bytes), and weeks fold
// into days (up05: 7w = 49 days).
func TestUnitsDefault_Period(t *testing.T) {
	cases := []struct {
		stem    string
		want    hocon.Period
		wantErr bool
	}{
		{"up01-period-bare", hocon.Period{Days: 7}, false},
		{"up02-period-leading-trailing-ws", hocon.Period{Days: 7}, false},
		{"up03-period-fractional-rejected", hocon.Period{}, true},
		{"up04-period-negative", hocon.Period{Days: -7}, false},
		{"up05-period-with-unit", hocon.Period{Days: 49}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.stem, func(t *testing.T) {
			confPath := filepath.Join(udFixtureDir, tc.stem+".conf")
			if _, err := os.Stat(confPath); os.IsNotExist(err) {
				t.Skipf("fixture not found — run `make testdata` first: %s", confPath)
			}
			src, err := os.ReadFile(confPath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			cfg, err := hocon.ParseString(string(src))
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			got, err := cfg.GetPeriodE("p")
			if tc.wantErr {
				if err == nil {
					t.Errorf("%s: GetPeriodE(\"p\") = %+v; want error", tc.stem, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("%s: GetPeriodE(\"p\") = %+v, %v; want %+v", tc.stem, got, err, tc.want)
			}
		})
	}
}
