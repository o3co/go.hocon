// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S20 Period format (xx.hocon#102): GetPeriod / GetPeriodE / GetPeriodOption
// and Unmarshal into hocon.Period, at parity with rs.hocon's get_period and
// py.hocon's get_period — integer-only, units d / w / m / mo / y (+ long
// forms), a bare number is days, unit names case-sensitive, negatives allowed,
// values bounded to int32.

package hocon_test

import (
	"errors"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

func TestGetPeriodE_Forms(t *testing.T) {
	cases := []struct {
		in   string
		want hocon.Period
	}{
		// S20.1 days (also the no-unit default)
		{`7`, hocon.Period{Days: 7}},
		{`"7"`, hocon.Period{Days: 7}},
		{`7d`, hocon.Period{Days: 7}},
		{`"7 day"`, hocon.Period{Days: 7}},
		{`7days`, hocon.Period{Days: 7}},
		// S20.2 weeks fold into days
		{`2w`, hocon.Period{Days: 14}},
		{`"2 week"`, hocon.Period{Days: 14}},
		{`2weeks`, hocon.Period{Days: 14}},
		// S20.3 months
		{`3m`, hocon.Period{Months: 3}},
		{`3mo`, hocon.Period{Months: 3}},
		{`3month`, hocon.Period{Months: 3}},
		{`3months`, hocon.Period{Months: 3}},
		// S20.4 years
		{`1y`, hocon.Period{Years: 1}},
		{`1year`, hocon.Period{Years: 1}},
		{`1years`, hocon.Period{Years: 1}},
		// sign, whitespace, int32 bounds
		{`"+7d"`, hocon.Period{Days: 7}},
		{`"-3mo"`, hocon.Period{Months: -3}},
		{`" 7 d "`, hocon.Period{Days: 7}},
		{`"2147483647"`, hocon.Period{Days: 2147483647}},
		{`"-2147483648"`, hocon.Period{Days: -2147483648}},
		{`"306783378w"`, hocon.Period{Days: 306783378 * 7}},
		{`"-306783378w"`, hocon.Period{Days: -306783378 * 7}},
		{"\"7\uFEFFd\"", hocon.Period{Days: 7}}, // U+FEFF is HOCON whitespace
		{"\"7\u00A0d\"", hocon.Period{Days: 7}}, // U+00A0 is HOCON whitespace
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			cfg := mustParseCfg(t, "p = "+tc.in)
			got, err := cfg.GetPeriodE("p")
			if err != nil {
				t.Fatalf("GetPeriodE: %v", err)
			}
			if got != tc.want {
				t.Errorf("GetPeriodE = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGetPeriodE_Rejects(t *testing.T) {
	inputs := []string{
		`"7.5"`,         // fractional (period is integer-only)
		`"7.5d"`,        // fractional with unit
		`7D`,            // case-sensitive: uppercase rejected
		`7M`,            // case-sensitive
		`7Y`,            // case-sensitive
		`"7 ms"`,        // duration unit, not a period unit
		`"7x"`,          // unknown unit
		`"d"`,           // unit with no number
		`""`,            // empty
		`"   "`,         // whitespace only
		`"+-7"`,         // malformed sign
		`"2147483648"`,  // past int32
		`"-2147483649"`, // past int32 (negative)
		"\"7\u0085d\"",  // U+0085 (NEL) is not HOCON whitespace
		"\"7\u200Bd\"",  // U+200B is not HOCON whitespace
		"\"\uFF17\"",    // fullwidth digit
		`"7e1"`,         // exponent
		`"306783379w"`,  // weeks × 7 past int32
		`true`,          // boolean
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "p = "+in)
			got, err := cfg.GetPeriodE("p")
			if err == nil {
				t.Fatalf("GetPeriodE = %+v; want error", got)
			}
			var ce *hocon.ConfigError
			if !errors.As(err, &ce) || ce.Path != "p" || !strings.Contains(ce.Message, "invalid period") {
				t.Errorf("err = %#v; want *ConfigError{Path: \"p\", Message: \"invalid period: ...\"}", err)
			}
			if cfg.GetPeriodOption("p").IsSome() {
				t.Error("GetPeriodOption = Some; want None")
			}
		})
	}
}

func TestGetPeriod_Variants(t *testing.T) {
	cfg := mustParseCfg(t, "ok = 2w\nbad = \"7.5\"\nnil = null\nobj { a = 1 }")

	if got := cfg.GetPeriod("ok"); got != (hocon.Period{Days: 14}) {
		t.Errorf("GetPeriod(ok) = %+v", got)
	}
	if got, ok := cfg.GetPeriodOption("ok").Get(); !ok || got != (hocon.Period{Days: 14}) {
		t.Errorf("GetPeriodOption(ok) = %+v, %v", got, ok)
	}
	for _, path := range []string{"bad", "nil", "obj", "missing"} {
		if _, err := cfg.GetPeriodE(path); err == nil {
			t.Errorf("GetPeriodE(%s): want error", path)
		}
		if cfg.GetPeriodOption(path).IsSome() {
			t.Errorf("GetPeriodOption(%s) = Some; want None", path)
		}
		func() {
			defer func() {
				if _, ok := recover().(*hocon.ConfigError); !ok {
					t.Errorf("GetPeriod(%s): want *ConfigError panic", path)
				}
			}()
			_ = cfg.GetPeriod(path)
		}()
	}
}

// TestUnmarshal_Period_ParityWithGetPeriodE asserts that Unmarshal into a
// Period field and GetPeriodE agree on every input: both succeed with the same
// Period, or both fail. null is the one deliberate exception (Unmarshal's
// scalar rule: field left unchanged) — see TestUnmarshal_Period_Null.
func TestUnmarshal_Period_ParityWithGetPeriodE(t *testing.T) {
	inputs := []string{
		`7`, `"7"`, `" 7 "`, `7d`, `2w`, `3mo`, `3m`, `1y`, `"-7"`, `"+7d"`,
		`"7.5"`, `7D`, `"7x"`, `""`, `"2147483648"`, `"306783379w"`, `true`,
	}
	type holder struct {
		P hocon.Period `hocon:"p"`
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			cfg := mustParseCfg(t, "p = "+in)
			want, wantErr := cfg.GetPeriodE("p")

			var h holder
			gotErr := cfg.Unmarshal(&h)

			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("GetPeriodE err = %v, Unmarshal err = %v; want both nil or both non-nil", wantErr, gotErr)
			}
			if wantErr == nil && h.P != want {
				t.Errorf("Unmarshal Period = %+v, GetPeriodE = %+v", h.P, want)
			}
		})
	}
}

func TestUnmarshal_Period_Containers(t *testing.T) {
	type holder struct {
		Retention hocon.Period            `hocon:"retention"`
		List      []hocon.Period          `hocon:"list"`
		Map       map[string]hocon.Period `hocon:"map"`
		Ptr       *hocon.Period           `hocon:"ptr"`
		Nil       *hocon.Period           `hocon:"nil"`
	}
	cfg := mustParseCfg(t, "retention = 30d\nlist = [1y, 6mo]\nmap { a = 2w }\nptr = 1y\nnil = null")
	var h holder
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if h.Retention != (hocon.Period{Days: 30}) {
		t.Errorf("Retention = %+v", h.Retention)
	}
	if len(h.List) != 2 || h.List[0] != (hocon.Period{Years: 1}) || h.List[1] != (hocon.Period{Months: 6}) {
		t.Errorf("List = %+v", h.List)
	}
	if h.Map["a"] != (hocon.Period{Days: 14}) {
		t.Errorf("Map[a] = %+v", h.Map["a"])
	}
	if h.Ptr == nil || *h.Ptr != (hocon.Period{Years: 1}) {
		t.Errorf("Ptr = %+v", h.Ptr)
	}
	if h.Nil != nil {
		t.Errorf("Nil = %+v, want nil", *h.Nil)
	}
}

func TestUnmarshalPath_Period_Scalar(t *testing.T) {
	cfg := mustParseCfg(t, "logs { retention = 90d }")
	var p hocon.Period
	if err := cfg.UnmarshalPath("logs.retention", &p); err != nil {
		t.Fatalf("UnmarshalPath: %v", err)
	}
	if p != (hocon.Period{Days: 90}) {
		t.Errorf("p = %+v", p)
	}
}

// TestUnmarshal_Period_ErrorCarriesPath asserts that a period failure names
// every key on the way down, and the getter's reason. An object value is
// rejected rather than decoded field-by-field: a Period reads only from a
// scalar, exactly like GetPeriod.
func TestUnmarshal_Period_ErrorCarriesPath(t *testing.T) {
	type logs struct {
		Retention hocon.Period `hocon:"retention"`
	}
	type root struct {
		Logs logs `hocon:"logs"`
	}
	cases := []struct {
		name, conf, reason string
	}{
		{"fractional", `logs { retention = "7.5" }`, "invalid period"},
		{"object", `logs { retention { days = 7 } }`, "expected scalar"},
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
			for _, want := range []string{`"logs"`, `"retention"`, tc.reason} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
		})
	}
}

// TestUnmarshal_Period_Null pins the one deliberate difference from GetPeriodE:
// null leaves a Period field unchanged, as for every scalar in Unmarshal, while
// the getter returns an error.
func TestUnmarshal_Period_Null(t *testing.T) {
	type holder struct {
		P hocon.Period `hocon:"p"`
	}
	cfg := mustParseCfg(t, "p = null")
	if _, err := cfg.GetPeriodE("p"); err == nil {
		t.Error("GetPeriodE on null: want error")
	}
	h := holder{P: hocon.Period{Days: 5}}
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if h.P != (hocon.Period{Days: 5}) {
		t.Errorf("P = %+v, want {Days:5} (unchanged)", h.P)
	}
}
