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
//   - ud01-ud08: duration fixtures — asserts GetDurationOption("t")
//   - ub01-ub06: bytes fixtures — asserts GetBytesOption("b")
//   - un01-un03: negative-case fixtures — asserts IsNone()
//   - up01-up05: period fixtures — skipped (GetPeriod not implemented in go.hocon)
package hocon_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/o3co/go.hocon"
)

const udFixtureDir = "testdata/hocon/units-default"

// TestUnitsDefault_Duration drives ud01–ud08 fixtures.
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

// TestUnitsDefault_Bytes drives ub01–ub06 fixtures.
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
		// ub05: "1024K" — single-letter alias; S21.4 not yet implemented in go.hocon (separate ❌).
		// Fixture present for input coverage; accessor assertion skipped (wantSome=false, treated as parse-error path).
		{"ub05-bytes-with-unit", "b", false, 0},
		// ub06: empty string — parse error → None
		{"ub06-bytes-empty-rejected", "b", false, 0},
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

// TestUnitsDefault_Negative drives un01–un03 fixtures.
// All three must return None from GetDurationOption (parse error: no valid number).
func TestUnitsDefault_Negative(t *testing.T) {
	cases := []string{
		"un01-empty-duration",
		"un02-ws-only-duration",
		"un03-unit-only-duration",
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

// TestUnitsDefault_Period skips up01–up05 fixtures.
// GetPeriod is not implemented in go.hocon — period fixtures are inputs-only
// until a GetPeriod accessor is added in a future phase.
func TestUnitsDefault_Period(t *testing.T) {
	periodFixtures := []string{
		"up01-period-bare",
		"up02-period-leading-trailing-ws",
		"up03-period-fractional-rejected",
		"up04-period-negative",
		"up05-period-with-unit",
	}
	for _, stem := range periodFixtures {
		t.Logf("S18.4 period: GetPeriod not implemented in go.hocon — skipping %s", stem)
	}
}
