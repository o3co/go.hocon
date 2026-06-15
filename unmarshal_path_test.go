package hocon_test

// go.hocon 1.8 — UnmarshalPath (any node at a path) + int-coercion parity with
// rs.hocon (whole-number-only float truncation, overflow-checked) + any-target
// and numeric-object->slice support.

import (
	"errors"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

func mustParseUnresolved(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := hocon.ParseStringWithOptions(src, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("parse(unresolved) error: %v", err)
	}
	return cfg
}

func TestUnmarshalPath_ObjectSubtree(t *testing.T) {
	cfg := mustParseCfg(t, `server { host="h", port=1 }`)
	var s ServerCfg
	if err := cfg.UnmarshalPath("server", &s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.Host != "h" || s.Port != 1 {
		t.Errorf("got %+v", s)
	}
}

func TestUnmarshalPath_ListAtPath(t *testing.T) {
	cfg := mustParseCfg(t, `ports = [1, 2, 3]`)
	var v []int
	if err := cfg.UnmarshalPath("ports", &v); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(v) != 3 || v[0] != 1 || v[2] != 3 {
		t.Errorf("got %v", v)
	}
}

func TestUnmarshalPath_ScalarAtPath(t *testing.T) {
	cfg := mustParseCfg(t, `port = 8080`)
	var n int
	if err := cfg.UnmarshalPath("port", &n); err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 8080 {
		t.Errorf("got %d", n)
	}
}

func TestUnmarshalPath_Missing(t *testing.T) {
	cfg := mustParseCfg(t, `a = 1`)
	var n int
	err := cfg.UnmarshalPath("nope", &n)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if errors.Is(err, hocon.ErrNotResolved) {
		t.Error("missing path must not be classified as not-resolved")
	}
}

func TestUnmarshalPath_NestedUnresolved(t *testing.T) {
	cfg := mustParseUnresolved(t, `obj { x = ${b} }`)
	var s struct {
		X int `hocon:"x"`
	}
	err := cfg.UnmarshalPath("obj", &s)
	if !errors.Is(err, hocon.ErrNotResolved) {
		t.Errorf("nested placeholder must wrap ErrNotResolved, got %v", err)
	}
}

func TestUnmarshalPath_EmptyPath_Errors(t *testing.T) {
	cfg := mustParseCfg(t, `a = 1`)
	var n int
	if err := cfg.UnmarshalPath("", &n); err == nil {
		t.Error("empty path must error")
	}
}

func TestUnmarshalPath_NonPointer(t *testing.T) {
	cfg := mustParseCfg(t, `port = 1`)
	var n int
	if err := cfg.UnmarshalPath("port", n); err == nil {
		t.Error("non-pointer target must error, not panic")
	}
}

func TestUnmarshalPath_AnyTarget(t *testing.T) {
	cfg := mustParseCfg(t, `obj { a = 1 }`)
	var x any
	if err := cfg.UnmarshalPath("obj", &x); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := x.(map[string]any); !ok {
		t.Fatalf("expected map[string]any, got %T", x)
	}
}

func TestUnmarshalPath_NumericObjectToSlice(t *testing.T) {
	cfg := mustParseCfg(t, `items { "0" = "a", "1" = "b" }`)
	var v []string
	if err := cfg.UnmarshalPath("items", &v); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(v) != 2 || v[0] != "a" || v[1] != "b" {
		t.Errorf("got %v", v)
	}
}

func TestUnmarshalPath_NonEmptyInterface_Errors(t *testing.T) {
	cfg := mustParseCfg(t, `s = "x"`)
	var e error // non-empty interface — a scalar is not assignable to it
	if err := cfg.UnmarshalPath("s", &e); err == nil {
		t.Error("non-empty interface target must return an error, not panic")
	}
}

func TestUnmarshalPath_NullAnyResetsToNil(t *testing.T) {
	cfg := mustParseCfg(t, `x = null`)
	var v any = "preset"
	if err := cfg.UnmarshalPath("x", &v); err != nil {
		t.Fatalf("err: %v", err)
	}
	if v != nil {
		t.Errorf("null must reset an any target to nil, got %v", v)
	}
}

func TestUnmarshalPath_NonNumericObjectToSlice_Errors(t *testing.T) {
	// A non-numeric-keyed object is not convertible to an array.
	cfg := mustParseCfg(t, `m { foo = "a", bar = "b" }`)
	var v []string
	if err := cfg.UnmarshalPath("m", &v); err == nil {
		t.Error("non-numeric object -> slice must error")
	}
}

func TestUnmarshalPath_ScalarToSlice_Errors(t *testing.T) {
	// A scalar (neither array nor object) is not a slice.
	cfg := mustParseCfg(t, `n = 5`)
	var v []int
	if err := cfg.UnmarshalPath("n", &v); err == nil {
		t.Error("scalar -> slice must error")
	}
}

func TestUnmarshal_IntCoercion_NonWholeRejected(t *testing.T) {
	cfg := mustParseCfg(t, `n = 1.5`)
	var s struct {
		N int `hocon:"n"`
	}
	if err := cfg.Unmarshal(&s); err == nil {
		t.Error("1.5 -> int must error (whole-number-only truncation), not silently become 1")
	}
}

func TestUnmarshal_IntCoercion_WholeAndQuoted(t *testing.T) {
	cfg := mustParseCfg(t, "a = 1.0\nb = 1e3\nc = \"1e3\"")
	var s struct {
		A int `hocon:"a"`
		B int `hocon:"b"`
		C int `hocon:"c"`
	}
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.A != 1 || s.B != 1000 || s.C != 1000 {
		t.Errorf("got %+v", s)
	}
}

func TestUnmarshal_IntCoercion_NonWholeRejected_Nested(t *testing.T) {
	// The strict coercion flows through unmarshalScalar for nested fields and
	// slice elements too, not just top-level fields.
	cfg := mustParseCfg(t, "obj { n = 1.5 }\nxs = [1, 2.5]")
	var s1 struct {
		Obj struct {
			N int `hocon:"n"`
		} `hocon:"obj"`
	}
	if err := cfg.Unmarshal(&s1); err == nil {
		t.Error("nested 1.5 -> int must error")
	}
	var s2 struct {
		Xs []int `hocon:"xs"`
	}
	if err := cfg.Unmarshal(&s2); err == nil {
		t.Error("slice element 2.5 -> int must error")
	}
}

func TestUnmarshal_IntOverflow_Rejected(t *testing.T) {
	cfg := mustParseCfg(t, `n = 100000`)
	var s struct {
		N int8 `hocon:"n"`
	}
	if err := cfg.Unmarshal(&s); err == nil {
		t.Error("100000 -> int8 must error (overflow), not silently wrap")
	}
}
