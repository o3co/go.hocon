// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// The requireunit struct tag option (ts.hocon#233 parity): a time.Duration,
// hocon.ByteSize or hocon.Period field that carries it refuses a value that
// reads but does not name its unit, so a key migrated from `ttl-seconds = 30`
// to `ttl = 30` cannot silently become 30 ms.

package hocon_test

import (
	"strings"
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

type requireUnitDuration struct {
	TTL time.Duration `hocon:"ttl,requireunit"`
}

type requireUnitBytes struct {
	TTL hocon.ByteSize `hocon:"ttl,requireunit"`
}

type requireUnitPeriod struct {
	TTL hocon.Period `hocon:"ttl,requireunit"`
}

// decodeRequireUnit unmarshals src into a fresh value of each holder type and
// returns the field value and error, keyed by the holder's noun.
func decodeRequireUnit(t *testing.T, kind, src string) (any, error) {
	t.Helper()
	cfg := mustParseCfg(t, src)
	switch kind {
	case "duration":
		var h requireUnitDuration
		err := cfg.Unmarshal(&h)
		return h.TTL, err
	case "byte size":
		var h requireUnitBytes
		err := cfg.Unmarshal(&h)
		return h.TTL, err
	case "period":
		var h requireUnitPeriod
		err := cfg.Unmarshal(&h)
		return h.TTL, err
	}
	t.Fatalf("unknown kind %q", kind)
	return nil, nil
}

func TestUnmarshal_RequireUnit_RefusesBareNumber(t *testing.T) {
	for _, kind := range []string{"duration", "byte size", "period"} {
		for _, in := range []string{`30`, `"30"`, `" 30 "`, `"+30"`} {
			t.Run(kind+"/"+in, func(t *testing.T) {
				_, err := decodeRequireUnit(t, kind, "ttl = "+in)
				want := `hocon: field "ttl": ` + kind + ` must name its unit`
				if err == nil || err.Error() != want {
					t.Fatalf("err = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestUnmarshal_RequireUnit_RefusesEnvVar(t *testing.T) {
	t.Setenv("HOCON_REQUIRE_UNIT_TTL", "30")
	for _, kind := range []string{"duration", "byte size", "period"} {
		t.Run(kind, func(t *testing.T) {
			_, err := decodeRequireUnit(t, kind, "ttl = 1\nttl = ${?HOCON_REQUIRE_UNIT_TTL}")
			want := `hocon: field "ttl": ` + kind + ` must name its unit`
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

// "1e3" is a number without a unit to the duration and byte-size readers, and
// not a period at all (periods are integers), so the period keeps the
// reader's own error.
func TestUnmarshal_RequireUnit_RefusesExponent(t *testing.T) {
	cases := []struct{ kind, want string }{
		{"duration", `hocon: field "ttl": duration must name its unit`},
		{"byte size", `hocon: field "ttl": byte size must name its unit`},
		{"period", `hocon: field "ttl": hocon: invalid period: 1e3`},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			_, err := decodeRequireUnit(t, c.kind, `ttl = "1e3"`)
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
	// The number scalar 1e3 too.
	if _, err := decodeRequireUnit(t, "duration", `ttl = 1e3`); err == nil ||
		err.Error() != `hocon: field "ttl": duration must name its unit` {
		t.Fatalf("number 1e3: err = %v", err)
	}
}

func TestUnmarshal_RequireUnit_ReadsValuesWithUnit(t *testing.T) {
	cases := []struct {
		kind, in string
		want     any
	}{
		{"duration", `30s`, 30 * time.Second},
		{"duration", `"30 s"`, 30 * time.Second},
		{"duration", `"1e3ms"`, time.Second},
		{"duration", `30 s`, 30 * time.Second},
		{"byte size", `64KiB`, hocon.ByteSize(64 << 10)},
		{"byte size", `"30 B"`, hocon.ByteSize(30)},
		{"byte size", `"1e3kB"`, hocon.ByteSize(1_000_000)},
		{"period", `2w`, hocon.Period{Days: 14}},
		{"period", `"30 d"`, hocon.Period{Days: 30}},
		{"period", `6mo`, hocon.Period{Months: 6}},
	}
	for _, c := range cases {
		t.Run(c.kind+"/"+c.in, func(t *testing.T) {
			got, err := decodeRequireUnit(t, c.kind, "ttl = "+c.in)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestUnmarshal_RequireUnit_ReadsEnvVarWithUnit(t *testing.T) {
	t.Setenv("HOCON_REQUIRE_UNIT_TTL", "30s")
	got, err := decodeRequireUnit(t, "duration", "ttl = 1s\nttl = ${?HOCON_REQUIRE_UNIT_TTL}")
	if err != nil || got != 30*time.Second {
		t.Fatalf("got %v, %v; want 30s", got, err)
	}
}

// A value that does not read keeps the reader's error: the reader runs first,
// then the unit check.
func TestUnmarshal_RequireUnit_ReaderErrorsFirst(t *testing.T) {
	cases := []struct{ kind, in, want string }{
		{"duration", `"abc"`, `hocon: field "ttl": hocon: invalid duration: abc (no number)`},
		{"duration", `"9223372036854775807"`, `hocon: field "ttl": hocon: duration overflows int64 nanoseconds: 9223372036854775807`},
		{"duration", `"200000d"`, `hocon: field "ttl": hocon: duration overflows int64 nanoseconds: 200000d`},
		{"byte size", `-5`, `hocon: field "ttl": hocon: byte size must not be negative`},
		{"byte size", `"99999999999999999999"`, `hocon: field "ttl": hocon: invalid byte size: 99999999999999999999`},
		{"byte size", `"5 lightyears"`, `hocon: field "ttl": hocon: invalid byte size: 5 lightyears`},
		{"period", `"abc"`, `hocon: field "ttl": hocon: invalid period: abc`},
		{"period", `"9999999999"`, `hocon: field "ttl": hocon: invalid period: 9999999999`},
		{"period", `"7.5"`, `hocon: field "ttl": hocon: invalid period: 7.5`},
	}
	for _, c := range cases {
		t.Run(c.kind+"/"+c.in, func(t *testing.T) {
			_, err := decodeRequireUnit(t, c.kind, "ttl = "+c.in)
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

// The message names the problem, not the value: a value can be a secret set
// under the wrong key, and the error ends up in a boot log.
func TestUnmarshal_RequireUnit_ErrorOmitsValue(t *testing.T) {
	t.Setenv("HOCON_REQUIRE_UNIT_SECRET", "8675309")
	for _, kind := range []string{"duration", "byte size", "period"} {
		t.Run(kind, func(t *testing.T) {
			_, err := decodeRequireUnit(t, kind, "ttl = ${HOCON_REQUIRE_UNIT_SECRET}")
			if err == nil {
				t.Fatal("want an error")
			}
			if strings.Contains(err.Error(), "8675309") {
				t.Errorf("error %q contains the value", err)
			}
		})
	}
}

func TestUnmarshal_RequireUnit_WithOmitempty(t *testing.T) {
	type either struct {
		A time.Duration `hocon:"a,omitempty,requireunit"`
		B time.Duration `hocon:"b,requireunit,omitempty"`
	}
	t.Run("absent keeps the pre-populated value", func(t *testing.T) {
		h := either{A: time.Minute, B: time.Hour}
		if err := mustParseCfg(t, `x = 1`).Unmarshal(&h); err != nil {
			t.Fatal(err)
		}
		if h.A != time.Minute || h.B != time.Hour {
			t.Errorf("got %+v", h)
		}
	})
	t.Run("null keeps the pre-populated value", func(t *testing.T) {
		h := either{A: time.Minute, B: time.Hour}
		if err := mustParseCfg(t, "a = null\nb = null").Unmarshal(&h); err != nil {
			t.Fatal(err)
		}
		if h.A != time.Minute || h.B != time.Hour {
			t.Errorf("got %+v", h)
		}
	})
	t.Run("a unit reads", func(t *testing.T) {
		var h either
		if err := mustParseCfg(t, "a = 2s\nb = 3s").Unmarshal(&h); err != nil {
			t.Fatal(err)
		}
		if h.A != 2*time.Second || h.B != 3*time.Second {
			t.Errorf("got %+v", h)
		}
	})
	for _, key := range []string{"a", "b"} {
		t.Run("bare number refused at "+key, func(t *testing.T) {
			var h either
			err := mustParseCfg(t, key+" = 30").Unmarshal(&h)
			want := `hocon: field "` + key + `": duration must name its unit`
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestUnmarshal_RequireUnit_PointerField(t *testing.T) {
	type holder struct {
		D *time.Duration  `hocon:"d,requireunit"`
		B *hocon.ByteSize `hocon:"b,requireunit"`
		P *hocon.Period   `hocon:"p,requireunit"`
	}
	t.Run("units read", func(t *testing.T) {
		var h holder
		if err := mustParseCfg(t, "d = 30s\nb = 64KiB\np = 2w").Unmarshal(&h); err != nil {
			t.Fatal(err)
		}
		if h.D == nil || *h.D != 30*time.Second {
			t.Errorf("D = %v", h.D)
		}
		if h.B == nil || *h.B != 64<<10 {
			t.Errorf("B = %v", h.B)
		}
		if h.P == nil || *h.P != (hocon.Period{Days: 14}) {
			t.Errorf("P = %v", h.P)
		}
	})
	t.Run("null leaves nil", func(t *testing.T) {
		var h holder
		if err := mustParseCfg(t, "d = null\nb = null\np = null").Unmarshal(&h); err != nil {
			t.Fatal(err)
		}
		if h.D != nil || h.B != nil || h.P != nil {
			t.Errorf("got %+v", h)
		}
	})
	cases := []struct{ src, want string }{
		{"d = 30\nb = 1KiB\np = 1d", `hocon: field "d": duration must name its unit`},
		{"d = 1s\nb = \"30\"\np = 1d", `hocon: field "b": byte size must name its unit`},
		{"d = 1s\nb = 1KiB\np = 30", `hocon: field "p": period must name its unit`},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			var h holder
			err := mustParseCfg(t, c.src).Unmarshal(&h)
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

// requireunit on a field that is not a unit value is a decode error, reported
// even when the key is absent, never silently ignored.
func TestUnmarshal_RequireUnit_OnOtherTypeIsError(t *testing.T) {
	type onInt struct {
		X int `hocon:"x,requireunit"`
	}
	type onString struct {
		X string `hocon:"x,omitempty,requireunit"`
	}
	type onSlice struct {
		X []time.Duration `hocon:"x,requireunit"`
	}
	const want = `hocon: field "x": requireunit applies to a duration, byte size or period field`
	for _, src := range []string{`x = 30`, `y = 1`} {
		for name, target := range map[string]any{"int": &onInt{}, "string": &onString{}, "slice": &onSlice{}} {
			t.Run(name+"/"+src, func(t *testing.T) {
				err := mustParseCfg(t, src).Unmarshal(target)
				if err == nil || err.Error() != want {
					t.Fatalf("err = %v, want %q", err, want)
				}
			})
		}
	}
}

// A recursive pointer type is "another type": the misuse error comes back
// instead of the type walk looping forever (the key is absent, so only the
// type check runs).
type recursivePtr *recursivePtr

func TestUnmarshal_RequireUnit_RecursivePointerTypeIsError(t *testing.T) {
	type onLoop struct {
		X recursivePtr `hocon:"x,omitempty,requireunit"`
	}
	const want = `hocon: field "x": requireunit applies to a duration, byte size or period field`
	cfg := mustParseCfg(t, `y = 1`)
	done := make(chan error, 1)
	go func() { done <- cfg.Unmarshal(&onLoop{}) }()
	select {
	case err := <-done:
		if err == nil || err.Error() != want {
			t.Fatalf("err = %v, want %q", err, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Unmarshal did not return: the type walk loops on a recursive pointer type")
	}
}

// Without the option a bare number reads in the default unit, through
// Unmarshal and the getters alike.
func TestUnmarshal_RequireUnit_AbsentReadsDefaultUnit(t *testing.T) {
	type holder struct {
		D time.Duration  `hocon:"d"`
		B hocon.ByteSize `hocon:"b"`
		P hocon.Period   `hocon:"p"`
	}
	cfg := mustParseCfg(t, "d = 30\nb = \"30\"\np = 30")
	var h holder
	if err := cfg.Unmarshal(&h); err != nil {
		t.Fatal(err)
	}
	if h.D != 30*time.Millisecond || h.B != 30 || h.P != (hocon.Period{Days: 30}) {
		t.Errorf("got %+v", h)
	}
	if got := cfg.GetDuration("d"); got != 30*time.Millisecond {
		t.Errorf("GetDuration = %v", got)
	}
	if got := cfg.GetBytes("b"); got != 30 {
		t.Errorf("GetBytes = %v", got)
	}
	if got := cfg.GetPeriod("p"); got != (hocon.Period{Days: 30}) {
		t.Errorf("GetPeriod = %v", got)
	}
}

// Nested: the outer field names its path segment, as for every Unmarshal error.
func TestUnmarshal_RequireUnit_Nested(t *testing.T) {
	type outer struct {
		Cache requireUnitDuration `hocon:"cache"`
	}
	var o outer
	err := mustParseCfg(t, "cache { ttl = 30 }").Unmarshal(&o)
	const want = `hocon: field "cache": hocon: field "ttl": duration must name its unit`
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}
