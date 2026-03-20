package resolver_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/resolver"
)

func resolve(t *testing.T, src string) *resolver.Result {
	t.Helper()
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return res
}

func TestResolver_SimpleKV(t *testing.T) {
	res := resolve(t, `key = "hello"`)
	v, ok := res.Root.Get("key")
	if !ok {
		t.Fatal("key not found")
	}
	if sv, ok := v.(*resolver.ScalarVal); !ok || sv.V != "hello" {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestResolver_DuplicateKeyMerge(t *testing.T) {
	res := resolve(t, "a { x=1 }\na { y=2 }")
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	o := obj.(*resolver.ObjectVal)
	if _, ok := o.Get("x"); !ok {
		t.Error("x missing after merge")
	}
	if _, ok := o.Get("y"); !ok {
		t.Error("y missing after merge")
	}
}

func TestResolver_Substitution(t *testing.T) {
	res := resolve(t, "x=1\ny=${x}")
	v, _ := res.Root.Get("y")
	if sv, ok := v.(*resolver.ScalarVal); !ok || sv.V != int64(1) {
		t.Errorf("substitution failed: %v", v)
	}
}

func TestResolver_OptionalSubstitutionMissing(t *testing.T) {
	res := resolve(t, "y=${?missing}")
	_, ok := res.Root.Get("y")
	if ok {
		t.Error("optional substitution of missing key should remove field")
	}
}

func TestResolver_OptionalSubstitutionFallback(t *testing.T) {
	// When an optional substitution references an undefined variable,
	// the prior value of that key must be preserved (not dropped).
	res := resolve(t, "host=\"0.0.0.0\"\nhost=${?HOST_UNSET_XYZ}")
	v, ok := res.Root.Get("host")
	if !ok {
		t.Fatal("prior value should be preserved when optional substitution is unset")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.V != "0.0.0.0" {
		t.Errorf("expected prior value \"0.0.0.0\", got %v", v)
	}
}

func TestResolver_CircularRef(t *testing.T) {
	ast, _ := parser.Parse("a=${b}\nb=${a}")
	_, err := resolver.Resolve(ast, resolver.Options{})
	if err == nil {
		t.Fatal("expected circular reference error")
	}
}

func TestResolver_SelfReference(t *testing.T) {
	res := resolve(t, `path=["/usr/bin"]
path=${path}["/usr/local/bin"]`)
	v, _ := res.Root.Get("path")
	arr := v.(*resolver.ArrayVal)
	if len(arr.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(arr.Elements))
	}
}

func TestResolver_PlusEquals(t *testing.T) {
	res := resolve(t, "arr=[1]\narr+=[2]")
	v, _ := res.Root.Get("arr")
	arr := v.(*resolver.ArrayVal)
	if len(arr.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(arr.Elements))
	}
}

func TestResolver_NullValue(t *testing.T) {
	res := resolve(t, "key=null")
	v, ok := res.Root.Get("key")
	if !ok {
		t.Fatal("key missing")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.V != nil {
		t.Errorf("expected null ScalarVal, got %v", v)
	}
}

func TestResolver_DuplicateScalarLastWins(t *testing.T) {
	// Later scalar definition must override earlier one.
	res := resolve(t, "a=1\na=2")
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.V != int64(2) {
		t.Errorf("expected last value 2, got %v", v)
	}
}

// --- Object assignment modes ---

func TestResolver_ObjectBracesMerge(t *testing.T) {
	// `key { ... }` syntax merges into existing object.
	res := resolve(t, "a { x=1 }\na { y=2 }")
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	o := obj.(*resolver.ObjectVal)
	if _, ok := o.Get("x"); !ok {
		t.Error("x should be preserved after brace-merge")
	}
	if _, ok := o.Get("y"); !ok {
		t.Error("y should be added by brace-merge")
	}
}

func TestResolver_ObjectEqualsObjectMerges(t *testing.T) {
	// Per HOCON spec: if both old and new values are objects, they are merged
	// even when the `=` operator is used.
	res := resolve(t, "a = { x=1 }\na = { y=2 }")
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	o := obj.(*resolver.ObjectVal)
	if _, ok := o.Get("x"); !ok {
		t.Error("x should be preserved: object = object merges per HOCON spec")
	}
	if _, ok := o.Get("y"); !ok {
		t.Error("y should be present after = assignment")
	}
}

func TestResolver_ObjectEqualsScalarReplaces(t *testing.T) {
	// When the previous value is a non-object, `=` replaces it.
	res := resolve(t, "a = \"hello\"\na = { y=2 }")
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	o, ok := obj.(*resolver.ObjectVal)
	if !ok {
		t.Fatal("a should be an object")
	}
	if _, ok := o.Get("y"); !ok {
		t.Error("y should be present after replacing scalar with object")
	}
}

func TestResolver_ObjectEqualsReplacesWithScalar(t *testing.T) {
	// Object replaced by a scalar via `=`.
	res := resolve(t, "a = { x=1 }\na = 42")
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.V != int64(42) {
		t.Errorf("expected scalar 42, got %v", v)
	}
}

func TestResolver_ObjectPlusEqualsAppendsArray(t *testing.T) {
	// `key += [...]` appends elements to the existing array.
	res := resolve(t, "a=[1,2]\na+=[3]")
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr := v.(*resolver.ArrayVal)
	if len(arr.Elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func resolveWithDir(t *testing.T, src, baseDir string) *resolver.Result {
	t.Helper()
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := resolver.Resolve(ast, resolver.Options{BaseDir: baseDir})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return res
}

func TestResolver_IncludeProbeConf(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sub.conf"), []byte(`x = "from-conf"`), 0o644)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	v, ok := obj.(*resolver.ObjectVal).Get("x")
	if !ok {
		t.Fatal("x not found")
	}
	if sv := v.(*resolver.ScalarVal); sv.V != "from-conf" {
		t.Errorf("expected from-conf, got %v", sv.V)
	}
}

func TestResolver_IncludeProbeJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sub.json"), []byte(`{"y": "from-json"}`), 0o644)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	v, ok := obj.(*resolver.ObjectVal).Get("y")
	if !ok {
		t.Fatal("y not found")
	}
	if sv := v.(*resolver.ScalarVal); sv.V != "from-json" {
		t.Errorf("expected from-json, got %v", sv.V)
	}
}

func TestResolver_IncludeProbeProperties(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sub.properties"), []byte(`z = "from-props"`), 0o644)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	v, ok := obj.(*resolver.ObjectVal).Get("z")
	if !ok {
		t.Fatal("z not found")
	}
	if sv := v.(*resolver.ScalarVal); sv.V != "from-props" {
		t.Errorf("expected from-props, got %v", sv.V)
	}
}

func TestResolver_IncludeProbeOrder(t *testing.T) {
	// When both .properties and .conf exist, .properties is found first.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sub.properties"), []byte(`x = "props"`), 0o644)
	os.WriteFile(filepath.Join(dir, "sub.conf"), []byte(`x = "conf"`), 0o644)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, _ := res.Root.Get("a")
	v, _ := obj.(*resolver.ObjectVal).Get("x")
	if sv := v.(*resolver.ScalarVal); sv.V != "props" {
		t.Errorf("expected props (first probe), got %v", sv.V)
	}
}

func TestResolver_IncludeWithExtensionNoProbe(t *testing.T) {
	// When an explicit extension is given, no probing should occur.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "sub.conf"), []byte(`x = "direct"`), 0o644)

	res := resolveWithDir(t, `a { include "sub.conf" }`, dir)
	obj, _ := res.Root.Get("a")
	v, _ := obj.(*resolver.ObjectVal).Get("x")
	if sv := v.(*resolver.ScalarVal); sv.V != "direct" {
		t.Errorf("expected direct, got %v", sv.V)
	}
}

func TestResolver_IncludeProbeNotFound(t *testing.T) {
	dir := t.TempDir()
	ast, err := parser.Parse(`a { include "nonexistent" }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Fatal("expected error for missing include, got nil")
	}
}
