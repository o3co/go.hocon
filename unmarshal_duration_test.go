// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Unmarshal into time.Duration (xx.hocon#102 contract 2): the getters and
// Unmarshal read durations through one reader, so they accept the same forms
// and fail for the same reason.

package hocon_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

// TestUnmarshal_Duration_ParityWithGetDurationE asserts that Unmarshal into a
// time.Duration field and GetDurationE agree on every input: both succeed with
// the same value, or both fail with the same reason. null is the one
// deliberate exception (Unmarshal's scalar rule: field left unchanged).
func TestUnmarshal_Duration_ParityWithGetDurationE(t *testing.T) {
	inputs := []string{
		`500`,                     // bare number scalar → ms (S18.1)
		`"500"`,                   // bare number string → ms (S18.4)
		`" 500 "`,                 // HOCON whitespace
		`30s`,                     // unit
		`"2 h"`,                   // whitespace before the unit
		`"1.001s"`,                // fractional, exact (E20)
		`"-1.5ns"`,                // truncate toward zero
		`"9223372036854775807ns"`, // int64 ceiling
		`"200000d"`,               // past int64 ns (E21)
		`"106752.0d"`,             // past int64 ns, fractional form (E21)
		`"9223372036854775807"`,   // ms, past int64 ns (E21)
		`30S`,                     // case-sensitive unit
		`"1w"`,                    // weeks are a period unit, not a duration unit
		`"5 lightyears"`,          // unknown unit
		`""`,                      // empty
		`"s"`,                     // unit only
		`true`,                    // boolean
	}
	type holder struct {
		T time.Duration `hocon:"t"`
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "t = "+in)
			want, wantErr := cfg.GetDurationE("t")

			var h holder
			gotErr := cfg.Unmarshal(&h)

			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("GetDurationE err = %v, Unmarshal err = %v; want both nil or both non-nil", wantErr, gotErr)
			}
			if wantErr == nil {
				if h.T != want {
					t.Errorf("Unmarshal = %d ns, GetDurationE = %d ns", h.T, want)
				}
				if got := cfg.GetDuration("t"); got != want {
					t.Errorf("GetDuration = %d ns, GetDurationE = %d ns", got, want)
				}
				if got, ok := cfg.GetDurationOption("t").Get(); !ok || got != want {
					t.Errorf("GetDurationOption = %d ns, %v; GetDurationE = %d ns", got, ok, want)
				}
				return
			}
			// Same reason: the Unmarshal error ends with the getter's message.
			var ce *hocon.ConfigError
			if !errors.As(wantErr, &ce) {
				t.Fatalf("GetDurationE err = %T, want *ConfigError", wantErr)
			}
			if !strings.HasSuffix(gotErr.Error(), ce.Message) {
				t.Errorf("Unmarshal err %q does not end with the getter's reason %q", gotErr, ce.Message)
			}
			// The panicking and Option getters read through the same reader.
			if cfg.GetDurationOption("t").IsSome() {
				t.Error("GetDurationOption = Some; want None")
			}
			func() {
				defer func() {
					pe, ok := recover().(*hocon.ConfigError)
					if !ok || pe.Message != ce.Message {
						t.Errorf("GetDuration panic = %v; want *ConfigError with message %q", pe, ce.Message)
					}
				}()
				_ = cfg.GetDuration("t")
			}()
		})
	}
}

func TestDuration_ErrorReasons(t *testing.T) {
	cases := []struct {
		in, reason string
	}{
		{`"5 lightyears"`, `invalid duration: 5 lightyears (unknown unit "lightyears")`},
		{`"30S"`, `invalid duration: 30S (unknown unit "S")`},
		{`"1x2"`, "invalid duration: 1x2 (unexpected characters)"},
		{`"s"`, "invalid duration: s (no number)"},
		{`"200000d"`, "duration overflows int64 nanoseconds: 200000d"},
	}
	type holder struct {
		T time.Duration `hocon:"timeout"`
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			cfg := mustParseCfg(t, "timeout = "+tc.in)
			if _, err := cfg.GetDurationE("timeout"); err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Errorf("GetDurationE err = %v; want reason %q", err, tc.reason)
			}
			var h holder
			err := cfg.Unmarshal(&h)
			if err == nil || !strings.Contains(err.Error(), tc.reason) || !strings.Contains(err.Error(), `"timeout"`) {
				t.Errorf("Unmarshal err = %v; want key \"timeout\" and reason %q", err, tc.reason)
			}
		})
	}
}
