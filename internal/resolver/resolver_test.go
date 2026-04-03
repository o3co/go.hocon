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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolver_IncludeProbeConf(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.conf"), `x = "from-conf"`)

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
	writeFile(t, filepath.Join(dir, "sub.json"), `{"y": "from-json"}`)

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
	writeFile(t, filepath.Join(dir, "sub.properties"), `z = "from-props"`)

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

func TestResolver_IncludeMergeAll(t *testing.T) {
	// When multiple extensions exist, all are loaded and merged.
	// Later formats (.conf) override earlier ones (.properties).
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `x = "props"`)
	writeFile(t, filepath.Join(dir, "sub.conf"), `x = "conf"
y = "only-conf"`)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, _ := res.Root.Get("a")
	o := obj.(*resolver.ObjectVal)

	// x exists in both: .conf wins (parsed last per spec).
	v, _ := o.Get("x")
	if sv := v.(*resolver.ScalarVal); sv.V != "conf" {
		t.Errorf("expected conf (later override), got %v", sv.V)
	}
	// y exists only in .conf: should be present.
	v2, ok := o.Get("y")
	if !ok {
		t.Fatal("y not found — merge missed .conf-only key")
	}
	if sv := v2.(*resolver.ScalarVal); sv.V != "only-conf" {
		t.Errorf("expected only-conf, got %v", sv.V)
	}
}

func TestResolver_IncludeMergeAllWithProperties(t *testing.T) {
	// Keys unique to .properties are preserved after merge.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `p = "from-props"`)
	writeFile(t, filepath.Join(dir, "sub.conf"), `c = "from-conf"`)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, _ := res.Root.Get("a")
	o := obj.(*resolver.ObjectVal)

	v1, ok1 := o.Get("p")
	if !ok1 {
		t.Fatal("p not found")
	}
	if sv := v1.(*resolver.ScalarVal); sv.V != "from-props" {
		t.Errorf("expected from-props, got %v", sv.V)
	}

	v2, ok2 := o.Get("c")
	if !ok2 {
		t.Fatal("c not found")
	}
	if sv := v2.(*resolver.ScalarVal); sv.V != "from-conf" {
		t.Errorf("expected from-conf, got %v", sv.V)
	}
}

func TestResolver_IncludeWithExtensionNoProbe(t *testing.T) {
	// When an explicit extension is given, no probing should occur.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.conf"), `x = "direct"`)

	res := resolveWithDir(t, `a { include "sub.conf" }`, dir)
	obj, _ := res.Root.Get("a")
	v, _ := obj.(*resolver.ObjectVal).Get("x")
	if sv := v.(*resolver.ScalarVal); sv.V != "direct" {
		t.Errorf("expected direct, got %v", sv.V)
	}
}

func TestResolver_IncludePropertiesValuesAreStrings(t *testing.T) {
	dir := t.TempDir()
	// .properties values should remain strings even if they look like bool/int/float/null.
	writeFile(t, filepath.Join(dir, "sub.properties"), `
a = true
b = 42
c = 3.14
d = null
`)
	res := resolveWithDir(t, `x { include "sub" }`, dir)
	obj, _ := res.Root.Get("x")
	o := obj.(*resolver.ObjectVal)

	cases := []struct{ key, want string }{
		{"a", "true"},
		{"b", "42"},
		{"c", "3.14"},
		{"d", "null"},
	}
	for _, tc := range cases {
		v, ok := o.Get(tc.key)
		if !ok {
			t.Errorf("key %q not found", tc.key)
			continue
		}
		sv := v.(*resolver.ScalarVal)
		s, ok := sv.V.(string)
		if !ok {
			t.Errorf("key %q: expected string type, got %T (%v)", tc.key, sv.V, sv.V)
			continue
		}
		if s != tc.want {
			t.Errorf("key %q: expected %q, got %q", tc.key, tc.want, s)
		}
	}
}

func TestResolver_IncludePropertiesExplicitExtension(t *testing.T) {
	// Even with explicit .properties extension, values should be strings.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `val = true`)

	res := resolveWithDir(t, `x { include "sub.properties" }`, dir)
	obj, _ := res.Root.Get("x")
	v, _ := obj.(*resolver.ObjectVal).Get("val")
	sv := v.(*resolver.ScalarVal)
	if _, ok := sv.V.(string); !ok {
		t.Errorf("expected string, got %T (%v)", sv.V, sv.V)
	}
}

func TestResolver_IncludePropertiesNestedObject(t *testing.T) {
	// Nested objects in .properties files should also have string values.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `
inner {
  a = 42
  b = true
}
`)
	res := resolveWithDir(t, `x { include "sub" }`, dir)
	obj, _ := res.Root.Get("x")
	inner, _ := obj.(*resolver.ObjectVal).Get("inner")
	o := inner.(*resolver.ObjectVal)

	v1, _ := o.Get("a")
	if sv := v1.(*resolver.ScalarVal); sv.V != "42" {
		t.Errorf("nested a: expected string \"42\", got %T %v", sv.V, sv.V)
	}
	v2, _ := o.Get("b")
	if sv := v2.(*resolver.ScalarVal); sv.V != "true" {
		t.Errorf("nested b: expected string \"true\", got %T %v", sv.V, sv.V)
	}
}

func TestResolver_IncludePropertiesArray(t *testing.T) {
	// Arrays in .properties files should have string elements,
	// including nested objects and arrays.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `
arr = [1, true, 3.14, null]
nested = [{a = 42}, [false]]
`)

	res := resolveWithDir(t, `x { include "sub" }`, dir)
	obj, _ := res.Root.Get("x")

	// Simple array
	v, _ := obj.(*resolver.ObjectVal).Get("arr")
	arr := v.(*resolver.ArrayVal)
	expected := []string{"1", "true", "3.14", "null"}
	for i, want := range expected {
		sv := arr.Elements[i].(*resolver.ScalarVal)
		if s, ok := sv.V.(string); !ok || s != want {
			t.Errorf("arr[%d]: expected string %q, got %T %v", i, want, sv.V, sv.V)
		}
	}

	// Nested array with object and sub-array
	v2, _ := obj.(*resolver.ObjectVal).Get("nested")
	nested := v2.(*resolver.ArrayVal)
	// First element: object {a = 42} → a should be string "42"
	innerObj := nested.Elements[0].(*resolver.ObjectVal)
	va, _ := innerObj.Get("a")
	if sv := va.(*resolver.ScalarVal); sv.V != "42" {
		t.Errorf("nested[0].a: expected string \"42\", got %T %v", sv.V, sv.V)
	}
	// Second element: array [false] → should be string "false"
	innerArr := nested.Elements[1].(*resolver.ArrayVal)
	sv := innerArr.Elements[0].(*resolver.ScalarVal)
	if s, ok := sv.V.(string); !ok || s != "false" {
		t.Errorf("nested[1][0]: expected string \"false\", got %T %v", sv.V, sv.V)
	}
}

func TestResolver_IncludeExplicitExtensionNotFound(t *testing.T) {
	// Non-required include with explicit extension: missing file is silently ignored per HOCON spec.
	dir := t.TempDir()
	ast, err := parser.Parse(`a { include "missing.conf" }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err != nil {
		t.Fatalf("non-required missing include should not error: %v", err)
	}
}

func TestResolver_IncludeRequiredExplicitExtensionNotFound(t *testing.T) {
	// required() include with explicit extension: missing file must error.
	dir := t.TempDir()
	ast, err := parser.Parse(`a { include required("missing.conf") }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Fatal("expected error for missing required include file")
	}
}

func TestResolver_IncludeProbeNotFound(t *testing.T) {
	// Non-required extensionless include: no files found should silently return empty per HOCON spec.
	dir := t.TempDir()
	ast, err := parser.Parse(`a { include "nonexistent" }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err != nil {
		t.Fatalf("non-required missing include should not error: %v", err)
	}
}

func TestResolver_IncludeRequiredProbeNotFound(t *testing.T) {
	// required() extensionless include: no files found must error.
	dir := t.TempDir()
	ast, err := parser.Parse(`a { include required("nonexistent") }`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Fatal("expected error for missing required extensionless include")
	}
}

func TestResolver_IncludeProbingPropagatesParseError(t *testing.T) {
	// A parse error in a file that EXISTS (during extension probing) must propagate,
	// not be silently swallowed as if the file were missing.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.conf"), `{ invalid = }`)

	ast, err := parser.Parse(`include "broken"`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Error("expected parse error from broken include file to propagate, got nil")
	}
}
