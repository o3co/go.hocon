// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"math"
	"testing"
)

// S21.2–S21.4 — unit-table alignment with the Lightbend reference
// (typesafe-config 1.4.6 probes, 2026-08-18), part of the four-impl units
// audit the py.hocon verification wave triggered. The old table stopped at
// TiB, lacked the two-letter binary forms (`Ki`), and was keyed `KB` — the
// reference's kilo-decimal spelling is `kB`, and `KB`/`kb` are errors.

func bytesOf(t *testing.T, lit string) (int64, bool) {
	t.Helper()
	cfg := mustParseCfg(t, `v = "`+lit+`"`)
	return cfg.GetBytesOption("v").Get()
}

func TestSpec_S21_2_DecimalUnitsThroughYB(t *testing.T) {
	cases := map[string]int64{
		"1kB":               1_000,
		"1MB":               1_000_000,
		"1GB":               1_000_000_000,
		"1TB":               1_000_000_000_000,
		"1PB":               1_000_000_000_000_000,
		"1petabyte":         1_000_000_000_000_000,
		"1EB":               1_000_000_000_000_000_000,
		"1exabytes":         1_000_000_000_000_000_000,
		"0.000001ZB":        1_000_000_000_000_000,
		"0.000000001YB":     1_000_000_000_000_000,
		"0.000001zettabyte": 1_000_000_000_000_000,
	}
	for lit, want := range cases {
		got, ok := bytesOf(t, lit)
		if !ok || got != want {
			t.Errorf("%s: got %d ok=%v, want %d", lit, got, ok, want)
		}
	}
	// A ZB/YB magnitude at count ≥1 exceeds int64 — Lightbend range-errors,
	// and so do we.
	if _, ok := bytesOf(t, "1ZB"); ok {
		t.Error("1ZB: expected overflow error, got a value")
	}
}

func TestSpec_S21_3_BinaryUnitsThroughYi(t *testing.T) {
	cases := map[string]int64{
		"1Ki":           1024,
		"1KiB":          1024,
		"1Mi":           1024 * 1024,
		"1Pi":           1 << 50,
		"1PiB":          1 << 50,
		"1pebibytes":    1 << 50,
		"1Ei":           1 << 60,
		"0.000001Zi":    int64(1e-6 * math.Exp2(70)),
		"0.000000001Yi": int64(1e-9 * math.Exp2(80)),
	}
	for lit, want := range cases {
		got, ok := bytesOf(t, lit)
		if !ok || got != want {
			t.Errorf("%s: got %d ok=%v, want %d", lit, got, ok, want)
		}
	}
}

func TestSpec_S21_4_SingleLettersThroughY(t *testing.T) {
	// Z/z/Y/y join the ladder; count 1 overflows, fractional counts pin
	// recognition (matching the Lightbend probe: `1Z` is a range error, not
	// an unknown unit).
	for _, lit := range []string{"1Z", "1z", "1Y", "1y"} {
		if _, ok := bytesOf(t, lit); ok {
			t.Errorf("%s: expected overflow error, got a value", lit)
		}
	}
	want := int64(1e-6 * math.Exp2(70))
	for _, lit := range []string{"0.000001Z", "0.000001z"} {
		got, ok := bytesOf(t, lit)
		if !ok || got != want {
			t.Errorf("%s: got %d ok=%v, want %d", lit, got, ok, want)
		}
	}
}

func TestSpec_S21_LightbendCaseSensitivity(t *testing.T) {
	// Lightbend's unit table is case-sensitive: kB parses, these do not.
	for _, lit := range []string{"1KB", "1kb", "1Kb", "1mB", "1Kilobyte", "1MEGABYTES", "1kiB", "1ki", "1Byte"} {
		if _, ok := bytesOf(t, lit); ok {
			t.Errorf("%s: expected unknown-unit error, got a value", lit)
		}
	}
	// The two-case exceptions: the bare byte unit and the single letters.
	for lit, want := range map[string]int64{"1B": 1, "1b": 1, "1K": 1024, "1k": 1024} {
		got, ok := bytesOf(t, lit)
		if !ok || got != want {
			t.Errorf("%s: got %d ok=%v, want %d", lit, got, ok, want)
		}
	}
}
