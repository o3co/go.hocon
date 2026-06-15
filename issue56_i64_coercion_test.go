package hocon_test

import (
	"math"
	"testing"
)

// xx.hocon#56: integer coercion of a float-like scalar must derive wholeness
// from the raw decimal text, not from an intermediate float64. Above 2^52 a
// float64 cannot represent fractional parts, so the previous
// `f != math.Trunc(f)` check both (a) false-accepted non-whole values and
// (b) off-by-one'd would-be-whole values. The only coercion surface is
// Unmarshal/UnmarshalPath (typed getters use strconv.ParseInt with no float
// fallback).

func TestUnmarshalIntCoercion_NonWholeHighMagnitudeRejected(t *testing.T) {
	for _, raw := range []string{"9007199254740992.5", "4503599627370496.5"} {
		cfg := mustParseCfg(t, "n = "+raw)
		var s struct {
			N int64 `hocon:"n"`
		}
		if err := cfg.Unmarshal(&s); err == nil {
			t.Errorf("%s -> int64 must error (non-whole), got %d", raw, s.N)
		}
	}
}

func TestUnmarshalIntCoercion_ExactWholeAbove2Pow53(t *testing.T) {
	// 2^53 + 1 as a whole float: via float64 this off-by-ones to 2^53.
	cfg := mustParseCfg(t, "n = 9007199254740993.0")
	var s struct {
		N int64 `hocon:"n"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.N != 9007199254740993 {
		t.Errorf("got %d, want 9007199254740993", s.N)
	}
}

func TestUnmarshalIntCoercion_LargeWholeExponentNoRegression(t *testing.T) {
	cfg := mustParseCfg(t, "n = 1e16")
	var s struct {
		N int64 `hocon:"n"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.N != 10000000000000000 {
		t.Errorf("got %d", s.N)
	}
}

func TestUnmarshalIntCoercion_NegativeExponentTextWholeness(t *testing.T) {
	cases := []struct {
		raw  string
		ok   bool
		want int64
	}{
		{"1e-3", false, 0},
		{"1000e-3", true, 1},
		{"1500e-3", false, 0},
		{"1.5e1", true, 15},
		{"1.234e2", false, 0},
	}
	for _, tc := range cases {
		cfg := mustParseCfg(t, "n = "+tc.raw)
		var s struct {
			N int64 `hocon:"n"`
		}
		err := cfg.Unmarshal(&s)
		if tc.ok {
			if err != nil || s.N != tc.want {
				t.Errorf("%s: got (%v, %d), want %d", tc.raw, err, s.N, tc.want)
			}
		} else if err == nil {
			t.Errorf("%s: want error, got %d", tc.raw, s.N)
		}
	}
}

func TestUnmarshalIntCoercion_Int64Boundaries(t *testing.T) {
	cfg := mustParseCfg(t, "min = -9223372036854775808.0\nmax = 9223372036854775807.0")
	var s struct {
		Min int64 `hocon:"min"`
		Max int64 `hocon:"max"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.Min != math.MinInt64 {
		t.Errorf("min got %d, want %d", s.Min, int64(math.MinInt64))
	}
	if s.Max != math.MaxInt64 {
		t.Errorf("max got %d, want %d", s.Max, int64(math.MaxInt64))
	}
	// one past i64::MAX as a whole float must reject, not wrap
	over := mustParseCfg(t, "n = 9223372036854775808.0")
	var o struct {
		N int64 `hocon:"n"`
	}
	if err := over.Unmarshal(&o); err == nil {
		t.Errorf("9223372036854775808.0 must error, got %d", o.N)
	}
}

func TestUnmarshalIntCoercion_HugeExponentNoHugeAlloc(t *testing.T) {
	// Unquoted, go's number lexer already rejects an overflowing exponent at
	// parse time; the quoted form bypasses that and reaches the coercion path,
	// where it must error quickly rather than allocate a multi-GB zero string.
	// The last two have an exponent outside int32 range — these must be rejected
	// without panicking (a MinInt64-valued exponent would otherwise overflow the
	// r/zeros arithmetic and panic strings.Repeat).
	for _, conf := range []string{
		`n = "1e2147483647"`,
		`n = "1e9999999999999999999"`,
		`n = "1e2147483648"`,
		`n = "1e-9223372036854775808"`,
	} {
		cfg := mustParseCfg(t, conf)
		var s struct {
			N int64 `hocon:"n"`
		}
		if err := cfg.Unmarshal(&s); err == nil {
			t.Errorf("%s must error, got %d", conf, s.N)
		}
	}
}

func TestUnmarshalIntCoercion_AllZeroMantissaIsZero(t *testing.T) {
	cfg := mustParseCfg(t, "a = 0.0\nb = 0e2147483647\nc = -0.0")
	var s struct {
		A int64 `hocon:"a"`
		B int64 `hocon:"b"`
		C int64 `hocon:"c"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.A != 0 || s.B != 0 || s.C != 0 {
		t.Errorf("got %+v, want all 0", s)
	}
}

func TestUnmarshalIntCoercion_PlusSignAndZeroDot(t *testing.T) {
	cfg := mustParseCfg(t, `a = "+1.0"
	                        b = "5."
	                        c = ".5"`)
	var s struct {
		A int64 `hocon:"a"`
		B int64 `hocon:"b"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.A != 1 || s.B != 5 {
		t.Errorf("got %+v, want {1 5}", s)
	}
	// ".5" is not whole
	var c struct {
		C int64 `hocon:"c"`
	}
	if err := cfg.Unmarshal(&c); err == nil {
		t.Errorf(".5 must error, got %d", c.C)
	}
}

func TestUnmarshalIntCoercion_MalformedAndOutOfRangeRejected(t *testing.T) {
	// Quoted so they reach the coercion path as string scalars rather than being
	// rejected by the number lexer. Covers: non-float-like, multiple dots /
	// non-digit runs, empty mantissa, and magnitudes that overflow int64/uint64
	// (positive and the negative >int64 magnitude branch).
	for _, conf := range []string{
		`n = "abcd"`, // no '.'/'e'/'E' — hits the non-float-like early return
		`n = "1.2.3"`,
		`n = "1a.0"`,
		`n = "."`,
		`n = "12345678901234567890.0"`,  // 20-digit int part > int64 max
		`n = "-18000000000000000000.0"`, // ~1.8e19: fits uint64, exceeds int64
		`n = "999999999999999999999.0"`, // 21 digits > uint64 max (ParseUint err)
	} {
		cfg := mustParseCfg(t, conf)
		var s struct {
			N int64 `hocon:"n"`
		}
		if err := cfg.Unmarshal(&s); err == nil {
			t.Errorf("%s must error, got %d", conf, s.N)
		}
	}
}

func TestUnmarshalIntCoercion_LeadingZeroFloatForms(t *testing.T) {
	cfg := mustParseCfg(t, "a = 0100.0\nb = 0001e3")
	var s struct {
		A int64 `hocon:"a"`
		B int64 `hocon:"b"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.A != 100 || s.B != 1000 {
		t.Errorf("got %+v, want {100 1000}", s)
	}
}
