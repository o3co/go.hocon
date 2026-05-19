package resolver_test

import (
	"os"
	"path/filepath"
	"strings"
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
	if sv, ok := v.(*resolver.ScalarVal); !ok || sv.Raw != "hello" {
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
	if sv, ok := v.(*resolver.ScalarVal); !ok || sv.Raw != "1" {
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
	if !ok || sv.Raw != "0.0.0.0" {
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
	if !ok || sv.Type != resolver.ScalarNull {
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
	if !ok || sv.Raw != "2" {
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
	if !ok || sv.Raw != "42" {
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
	if sv := v.(*resolver.ScalarVal); sv.Raw != "from-conf" {
		t.Errorf("expected from-conf, got %v", sv.Raw)
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
	if sv := v.(*resolver.ScalarVal); sv.Raw != "from-json" {
		t.Errorf("expected from-json, got %v", sv.Raw)
	}
}

func TestResolver_IncludeProbeProperties(t *testing.T) {
	dir := t.TempDir()
	// .properties files do not use quote delimiters; value is the literal text.
	writeFile(t, filepath.Join(dir, "sub.properties"), `z = from-props`)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	v, ok := obj.(*resolver.ObjectVal).Get("z")
	if !ok {
		t.Fatal("z not found")
	}
	if sv := v.(*resolver.ScalarVal); sv.Raw != "from-props" {
		t.Errorf("expected from-props, got %v", sv.Raw)
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
	if sv := v.(*resolver.ScalarVal); sv.Raw != "conf" {
		t.Errorf("expected conf (later override), got %v", sv.Raw)
	}
	// y exists only in .conf: should be present.
	v2, ok := o.Get("y")
	if !ok {
		t.Fatal("y not found — merge missed .conf-only key")
	}
	if sv := v2.(*resolver.ScalarVal); sv.Raw != "only-conf" {
		t.Errorf("expected only-conf, got %v", sv.Raw)
	}
}

func TestResolver_IncludeMergeAllWithProperties(t *testing.T) {
	// Keys unique to .properties are preserved after merge.
	dir := t.TempDir()
	// .properties files do not use quote delimiters; value is the literal text.
	writeFile(t, filepath.Join(dir, "sub.properties"), `p = from-props`)
	writeFile(t, filepath.Join(dir, "sub.conf"), `c = "from-conf"`)

	res := resolveWithDir(t, `a { include "sub" }`, dir)
	obj, _ := res.Root.Get("a")
	o := obj.(*resolver.ObjectVal)

	v1, ok1 := o.Get("p")
	if !ok1 {
		t.Fatal("p not found")
	}
	if sv := v1.(*resolver.ScalarVal); sv.Raw != "from-props" {
		t.Errorf("expected from-props, got %v", sv.Raw)
	}

	v2, ok2 := o.Get("c")
	if !ok2 {
		t.Fatal("c not found")
	}
	if sv := v2.(*resolver.ScalarVal); sv.Raw != "from-conf" {
		t.Errorf("expected from-conf, got %v", sv.Raw)
	}
}

func TestResolver_IncludeWithExtensionNoProbe(t *testing.T) {
	// When an explicit extension is given, no probing should occur.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.conf"), `x = "direct"`)

	res := resolveWithDir(t, `a { include "sub.conf" }`, dir)
	obj, _ := res.Root.Get("a")
	v, _ := obj.(*resolver.ObjectVal).Get("x")
	if sv := v.(*resolver.ScalarVal); sv.Raw != "direct" {
		t.Errorf("expected direct, got %v", sv.Raw)
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
		s := sv.Raw
		if sv.Type != resolver.ScalarString {
			t.Errorf("key %q: expected ScalarString type, got %v (%v)", tc.key, sv.Type, sv.Raw)
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
	if sv.Type != resolver.ScalarString {
		t.Errorf("expected ScalarString type, got %v (%v)", sv.Type, sv.Raw)
	}
}

func TestResolver_IncludePropertiesNestedObject(t *testing.T) {
	// Dotted keys in .properties files expand into nested ObjectVal hierarchy.
	// All leaf values must be strings per .properties spec.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `
inner.a = 42
inner.b = true
`)
	res := resolveWithDir(t, `x { include "sub" }`, dir)
	obj, _ := res.Root.Get("x")
	inner, ok := obj.(*resolver.ObjectVal).Get("inner")
	if !ok {
		t.Fatal("inner not found")
	}
	o := inner.(*resolver.ObjectVal)

	v1, _ := o.Get("a")
	if sv := v1.(*resolver.ScalarVal); sv.Raw != "42" {
		t.Errorf("nested a: expected string \"42\", got %T %v", sv.Raw, sv.Raw)
	}
	v2, _ := o.Get("b")
	if sv := v2.(*resolver.ScalarVal); sv.Raw != "true" {
		t.Errorf("nested b: expected string \"true\", got %T %v", sv.Raw, sv.Raw)
	}
}

func TestResolver_IncludePropertiesArray(t *testing.T) {
	// Standard .properties files do not support array syntax.
	// Values that look like arrays are treated as literal strings.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.properties"), `
list = one,two,three
`)

	res := resolveWithDir(t, `x { include "sub" }`, dir)
	obj, _ := res.Root.Get("x")

	// Value is a literal string — comma-separated values are the caller's responsibility to split.
	v, ok := obj.(*resolver.ObjectVal).Get("list")
	if !ok {
		t.Fatal("list not found")
	}
	sv := v.(*resolver.ScalarVal)
	if sv.Raw != "one,two,three" || sv.Type != resolver.ScalarString {
		t.Errorf("list: expected string \"one,two,three\", got %v (type %v)", sv.Raw, sv.Type)
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

// TestResolver_IncludeOptionalNonEnoentErrorPropagates verifies that a non-required
// include whose ReadFile fails with a non-ENOENT error (e.g. "is a directory")
// propagates the error instead of silently returning an empty object.
func TestResolver_IncludeOptionalNonEnoentErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with the target name + ".conf" — reading a directory is not ENOENT.
	subDir := filepath.Join(dir, "subdir.conf")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ast, err := parser.Parse(`include "subdir.conf"`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Error("expected error when ReadFile fails with non-ENOENT, got nil")
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

func TestResolver_ObjectConcatenation(t *testing.T) {
	// HOCON spec: `a = {x: 1} {y: 2}` should deep-merge into {x:1, y:2}
	res := resolve(t, `a = {x: 1} {y: 2}`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	o, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("expected ObjectVal, got %T", v)
	}
	xv, ok := o.Get("x")
	if !ok {
		t.Error("x missing after object concatenation")
	} else if sv, ok := xv.(*resolver.ScalarVal); !ok || sv.Raw != "1" {
		t.Errorf("expected x=1, got %v", xv)
	}
	yv, ok := o.Get("y")
	if !ok {
		t.Error("y missing after object concatenation")
	} else if sv, ok := yv.(*resolver.ScalarVal); !ok || sv.Raw != "2" {
		t.Errorf("expected y=2, got %v", yv)
	}
}

func TestResolver_ObjectConcatenationDeepMerge(t *testing.T) {
	// HOCON spec: nested object concatenation should deep-merge
	res := resolve(t, `a = {x: {nested: 1}} {x: {other: 2}}`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	o, ok := v.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("expected ObjectVal, got %T", v)
	}
	xv, ok := o.Get("x")
	if !ok {
		t.Fatal("x missing after deep merge")
	}
	xo, ok := xv.(*resolver.ObjectVal)
	if !ok {
		t.Fatalf("expected x to be ObjectVal, got %T", xv)
	}
	if nv, ok := xo.Get("nested"); !ok {
		t.Error("nested missing after deep merge")
	} else if sv, ok := nv.(*resolver.ScalarVal); !ok || sv.Raw != "1" {
		t.Errorf("expected nested=1, got %v", nv)
	}
	if ov, ok := xo.Get("other"); !ok {
		t.Error("other missing after deep merge")
	} else if sv, ok := ov.(*resolver.ScalarVal); !ok || sv.Raw != "2" {
		t.Errorf("expected other=2, got %v", ov)
	}
}

func TestResolver_CircularIncludeDetection(t *testing.T) {
	// circular_a.conf includes circular_b.conf which includes circular_a.conf.
	// This must produce a ResolveError with "circular include" rather than
	// hanging forever or stack-overflowing.
	baseDir := filepath.Join("..", "..", "testdata", "hocon")
	ast, err := parser.Parse(`include "circular_a.conf"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: baseDir})
	if err == nil {
		t.Fatal("expected circular include error, got nil")
	}
	re, ok := err.(*resolver.ResolveError)
	if !ok {
		t.Fatalf("expected *ResolveError, got %T: %v", err, err)
	}
	if !strings.Contains(re.Message, "circular include") {
		t.Errorf("expected message containing \"circular include\", got %q", re.Message)
	}
}

func TestResolver_CircularIncludeDetected(t *testing.T) {
	// Two files that include each other must produce a circular include error,
	// not an infinite loop. Use relative paths so Clean+Abs normalization is exercised.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.conf"), `include "b.conf"`)
	writeFile(t, filepath.Join(dir, "b.conf"), `include "a.conf"`)

	ast, err := parser.Parse(`include "a.conf"`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Fatal("expected circular include error, got nil")
	}
	if re, ok := err.(*resolver.ResolveError); ok {
		if re.Message == "" {
			t.Error("expected non-empty error message")
		}
	}
}

func TestResolver_CircularIncludeSelfDetected(t *testing.T) {
	// A file that includes itself must be detected as circular.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "self.conf"), `include "self.conf"`)

	ast, err := parser.Parse(`include "self.conf"`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if err == nil {
		t.Fatal("expected circular include error for self-include, got nil")
	}
}

func TestResolver_ObjectConcatenationKeyOrder(t *testing.T) {
	src := `a = {x: 1} {y: 2}`
	res := resolve(t, src)
	aVal, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	aObj := aVal.(*resolver.ObjectVal)
	keys := aObj.Keys()
	if len(keys) != 2 || keys[0] != "x" || keys[1] != "y" {
		t.Errorf("expected keys [x, y], got %v", keys)
	}
}

func TestResolver_IncludeRelativizeSubstitutions(t *testing.T) {
	// When a file is included into a nested scope, substitution paths in
	// the included file must be relativized so they resolve against the
	// parent tree. For example, ${x} in child.conf (referenced as y = ${x})
	// becomes ${wrapper.x} when included as `wrapper { include "child.conf" }`.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "child.conf"), `
x = 10
y = ${x}
`)

	// Simple nesting: wrapper { include "child.conf" }
	res := resolveWithDir(t, `wrapper { include "child.conf" }`, dir)
	wv, ok := res.Root.Get("wrapper")
	if !ok {
		t.Fatal("wrapper not found")
	}
	wo := wv.(*resolver.ObjectVal)

	xv, ok := wo.Get("x")
	if !ok {
		t.Fatal("x not found in wrapper")
	}
	if sv := xv.(*resolver.ScalarVal); sv.Raw != "10" {
		t.Errorf("expected x=10, got %v", sv.Raw)
	}

	yv, ok := wo.Get("y")
	if !ok {
		t.Fatal("y not found in wrapper")
	}
	if sv := yv.(*resolver.ScalarVal); sv.Raw != "10" {
		t.Errorf("expected y=10 (resolved from ${x}), got %v", sv.Raw)
	}
}

func TestResolver_IncludeRelativizeDeepNesting(t *testing.T) {
	// Double nesting: bar { nested { include "child.conf" } }
	// ${x} should resolve to ${bar.nested.x}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "child.conf"), `
x = 10
y = ${x}
`)

	res := resolveWithDir(t, `bar { nested { include "child.conf" } }`, dir)
	bv, ok := res.Root.Get("bar")
	if !ok {
		t.Fatal("bar not found")
	}
	nv, ok := bv.(*resolver.ObjectVal).Get("nested")
	if !ok {
		t.Fatal("nested not found")
	}
	no := nv.(*resolver.ObjectVal)

	yv, ok := no.Get("y")
	if !ok {
		t.Fatal("y not found in bar.nested")
	}
	if sv := yv.(*resolver.ScalarVal); sv.Raw != "10" {
		t.Errorf("expected y=10, got %v", sv.Raw)
	}
}

func TestResolver_IncludeRelativizeMultiSegmentKey(t *testing.T) {
	// Multi-segment key: a.b { include "child.conf" }
	// The parser keeps Key = ["a", "b"] without decomposing into nested
	// objects, so pathPrefix must include all segments.
	// child.conf references ${ext} which is NOT in child.conf — it stays
	// as a placeholder and must be relativized to ${a.b.ext} for correct
	// resolution against the parent tree.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "child.conf"), `
val = ${ext}
`)

	res := resolveWithDir(t, `
a.b {
  ext = "hello"
  include "child.conf"
}
`, dir)
	av, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	bv, ok := av.(*resolver.ObjectVal).Get("b")
	if !ok {
		t.Fatal("b not found in a")
	}
	bo := bv.(*resolver.ObjectVal)

	vv, ok := bo.Get("val")
	if !ok {
		t.Fatal("val not found in a.b")
	}
	if sv := vv.(*resolver.ScalarVal); sv.Raw != "hello" {
		t.Errorf("expected val='hello' (resolved from ${ext}), got %v", sv.Raw)
	}
}

func TestResolver_IncludeRelativizeFallbackToParent(t *testing.T) {
	// When a substitution in an included file cannot be resolved within
	// the include scope, it should fall back to the root level.
	// child.conf has ${bar} which is not in child.conf but is at root.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "child.conf"), `
a = ${bar}
`)

	res := resolveWithDir(t, `
bar = "from-root"
wrapper { include "child.conf" }
`, dir)

	wv, ok := res.Root.Get("wrapper")
	if !ok {
		t.Fatal("wrapper not found")
	}
	av, ok := wv.(*resolver.ObjectVal).Get("a")
	if !ok {
		t.Fatal("a not found in wrapper")
	}
	if sv := av.(*resolver.ScalarVal); sv.Raw != "from-root" {
		t.Errorf("expected a='from-root', got %v", sv.Raw)
	}
}

// lookupObj navigates into a nested ObjectVal using a dot-separated key.
// Each segment is looked up as a direct key first (for quoted keys like "a.b"),
// falling back to nested traversal.
func lookupObj(t *testing.T, res *resolver.Result, key string) *resolver.ObjectVal {
	t.Helper()
	// Try direct key first (handles quoted keys like "a.b")
	v, ok := res.Root.Get(key)
	if ok {
		obj, isObj := v.(*resolver.ObjectVal)
		if isObj {
			return obj
		}
	}
	// Fall back to dot-separated traversal
	parts := strings.Split(key, ".")
	cur := res.Root
	for _, p := range parts {
		val, found := cur.Get(p)
		if !found {
			t.Fatalf("key %q not found while looking up %q", p, key)
		}
		obj, isObj := val.(*resolver.ObjectVal)
		if !isObj {
			t.Fatalf("value at %q is %T, not ObjectVal", p, val)
		}
		cur = obj
	}
	return cur
}

func assertScalar(t *testing.T, obj *resolver.ObjectVal, key string, expected string) {
	t.Helper()
	v, ok := obj.Get(key)
	if !ok {
		t.Fatalf("key %q not found", key)
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		t.Fatalf("key %q: expected ScalarVal, got %T", key, v)
	}
	if sv.Raw != expected {
		t.Errorf("key %q: expected %q, got %q", key, expected, sv.Raw)
	}
}

func TestResolver_IncludeRelativizeQuotedKeyWithDots(t *testing.T) {
	// When a file is included under a quoted key containing dots (like "a.b"),
	// substitutions referencing external values must be correctly relativized.
	// The path prefix ["a.b"] must NOT be joined with "." naively — that would
	// produce "a.b.ext" which splits into ["a","b","ext"] instead of ["a.b","ext"].
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "child.conf"), `val = ${ext}`)

	res := resolveWithDir(t, `
"a.b" {
  ext = "hello"
  include "child.conf"
}
`, dir)
	ab := lookupObj(t, res, "a.b")
	assertScalar(t, ab, "ext", "hello")
	assertScalar(t, ab, "val", "hello")
}

func TestResolver_EnvFallbackQuotedKeyPrefix(t *testing.T) {
	// Env var fallback for substitutions under a quoted-dot key prefix.
	// ${MY_VAR} included under "a.b" is relativized to "a.b".MY_VAR.
	// The env lookup must try the original key MY_VAR (not "a.b".MY_VAR).
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "child.conf"), `val = ${MY_VAR}`)
	t.Setenv("MY_VAR", "ok")
	res := resolveWithDir(t, `"a.b" { include "child.conf" }`, dir)
	ab := lookupObj(t, res, "a.b")
	assertScalar(t, ab, "val", "ok")
}

func TestResolver_FileIncludeResolvesFromCWD(t *testing.T) {
	// include file("path") must resolve relative to the process working
	// directory, NOT relative to the including file's directory (BaseDir).
	// Bare include "path" continues to resolve relative to BaseDir.

	// Set up two directories: baseDir (where the .conf file lives) and cwdDir (process CWD).
	baseDir := t.TempDir()
	cwdDir := t.TempDir()

	// Put a file in cwdDir that should be found by file() include.
	writeFile(t, filepath.Join(cwdDir, "cwd-only.conf"), `cwd_val = "from-cwd"`)

	// Put a file in baseDir that should be found by bare include.
	writeFile(t, filepath.Join(baseDir, "base-only.conf"), `base_val = "from-base"`)

	// Put a file in baseDir that has the same name as one in cwdDir but different content.
	// This tests that file() does NOT pick up the baseDir version.
	writeFile(t, filepath.Join(baseDir, "cwd-only.conf"), `cwd_val = "WRONG-from-base"`)

	// Change CWD to cwdDir for this test.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwdDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	src := `
include "base-only.conf"
include file("cwd-only.conf")
`
	ast, parseErr := parser.Parse(src)
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	res, resolveErr := resolver.Resolve(ast, resolver.Options{BaseDir: baseDir})
	if resolveErr != nil {
		t.Fatalf("resolve: %v", resolveErr)
	}

	// Bare include should resolve from baseDir.
	v, ok := res.Root.Get("base_val")
	if !ok {
		t.Fatal("base_val not found — bare include failed")
	}
	if sv := v.(*resolver.ScalarVal); sv.Raw != "from-base" {
		t.Errorf("base_val: expected from-base, got %s", sv.Raw)
	}

	// file() include should resolve from CWD, not baseDir.
	v2, ok2 := res.Root.Get("cwd_val")
	if !ok2 {
		t.Fatal("cwd_val not found — file() include failed")
	}
	if sv := v2.(*resolver.ScalarVal); sv.Raw != "from-cwd" {
		t.Errorf("cwd_val: expected from-cwd, got %s", sv.Raw)
	}
}

func TestResolver_FileIncludeMissingSilentlySkipped(t *testing.T) {
	// include file("nonexistent.conf") with Required=false should be silently skipped.
	// Use a temp directory as CWD so the test doesn't depend on the real CWD's contents.
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	writeFile(t, filepath.Join(dir, "main.conf"), "base = 1\ninclude file(\"nonexistent.conf\")\n")

	src := `
base = 1
include file("nonexistent.conf")
`
	ast, parseErr := parser.Parse(src)
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	res, resolveErr := resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if resolveErr != nil {
		t.Fatalf("resolve: %v — file() include of missing file should be silently skipped", resolveErr)
	}
	v, ok := res.Root.Get("base")
	if !ok {
		t.Fatal("base not found")
	}
	if sv := v.(*resolver.ScalarVal); sv.Raw != "1" {
		t.Errorf("expected 1, got %s", sv.Raw)
	}
}

// -----------------------------------------------------------------------------
// Spec compliance Phase 2: concatenation + += rules (S10, S13b).
// -----------------------------------------------------------------------------

// TestSpecS10_4_MixingArrayAndObjectInConcatIsError verifies that concatenating
// an array with an object (or vice versa) is a resolver error. Spec L385.
// Status: ✅ (fixed in fix/s10-concat-type-check — Phase 6 #3b).
func TestSpecS10_4_MixingArrayAndObjectInConcatIsError(t *testing.T) {
	cases := []string{
		`a = [1, 2] {x: 1}`,
		`a = {x: 1} [1, 2]`,
	}
	for _, src := range cases {
		if _, err := resolveErr(src); err == nil {
			t.Errorf("expected resolve error for %q (array+object concat), got nil", src)
		}
	}
}

// TestSpecS10_14_WhitespaceAroundSubstitutionIsIgnored verifies that whitespace
// between a substitution and an adjacent array is ignored for concat purposes.
// Spec L440. Status: ✅
func TestSpecS10_14_WhitespaceAroundSubstitutionIsIgnored(t *testing.T) {
	res := resolve(t, "arr = [1]\na = ${arr}   [2]")
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal for whitespace-padded subst concat, got %T", v)
	}
	if len(arr.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr.Elements))
	}
}

// TestSpecS10_19_SubstResolvedObjectPlusLiteralArrayIsError verifies that when a
// substitution resolves to an object, concatenating it with a literal array is a
// resolve error. Spec L385-389.
// Status: ✅ (fixed in fix/s10-concat-type-check — Phase 6 #3b).
func TestSpecS10_19_SubstResolvedObjectPlusLiteralArrayIsError(t *testing.T) {
	src := "obj = {x: 1}\na = ${obj} [1, 2]"
	if _, err := resolveErr(src); err == nil {
		t.Error("expected resolve error: substitution-resolved object + literal array should be an error")
	}
}

// -----------------------------------------------------------------------------
// Spec compliance Phase 6 #3b: S10.4 / S10.13 / S10.19 type-check tightening.
// -----------------------------------------------------------------------------

// TestSpecS10_4_ArrayPlusObjectErrors verifies that array+object concat errors.
// Spec L385. Status: ✅ (Phase 6 #3b).
func TestSpecS10_4_ArrayPlusObjectErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"literal array+object", `a = [1] { b: 2 }`},
		{"literal object+array", `a = { b: 2 } [1]`},
		{"subst obj + literal array", "obj = { b: 2 }\na = [1] ${obj}"},
		{"literal array + subst obj", "arr = [1]\na = ${arr} { b: 2 }"},
		{"empty array + object", `a = [] {b:1}`},
		{"array + empty object", `a = [1] {}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveErr(tc.src); err == nil {
				t.Errorf("expected resolve error for %q, got nil", tc.src)
			}
		})
	}
}

// TestSpecS15_StillBridgesNumericKeyedObject is a regression guard ensuring the
// Phase 6 #2 numericObjectToArray success path still works after S10.4 tightening.
// Spec S15. Status: ✅.
func TestSpecS15_StillBridgesNumericKeyedObject(t *testing.T) {
	// Numeric-keyed object converts to array; array concat succeeds.
	res := resolve(t, `obj = {"0":"x","1":"y"}
a = [1] ${obj}`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements (1,x,y), got %d", len(arr.Elements))
	}
}

// TestSpecS10_13_ArrayPlusScalarErrors verifies that array+scalar and
// scalar+array in concat both error. Spec L373. Status: ✅ (Phase 6 #3b).
func TestSpecS10_13_ArrayPlusScalarErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"array+scalar", `a = [1, 2] 3`},
		{"scalar+array", `a = 3 [1, 2]`},
		{"string+array via subst", "arr = [1]\na = x ${arr}"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveErr(tc.src); err == nil {
				t.Errorf("expected resolve error for %q (array+scalar), got nil", tc.src)
			}
		})
	}
}

// TestSpecS10_13_ArrayPlusArrayStillWorks is a regression guard ensuring that
// array+array concat remains valid after S10.13 tightening.
func TestSpecS10_13_ArrayPlusArrayStillWorks(t *testing.T) {
	res := resolve(t, `a = [1] [2]`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	if len(arr.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr.Elements))
	}
}

// TestSpecS10_13_ObjectPlusScalarErrors verifies that object+scalar and
// scalar+object in concat both error. Spec L373. Status: ✅ (Phase 6 #3b).
func TestSpecS10_13_ObjectPlusScalarErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"object+scalar", `a = { b: 1 } x`},
		{"scalar+object", `a = x { b: 1 }`},
		{"scalar+subst object", "obj = { b: 1 }\na = x ${obj}"},
		{"object+subst scalar", "s = foo\na = { b: 1 } ${s}"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveErr(tc.src); err == nil {
				t.Errorf("expected resolve error for %q (object+scalar), got nil", tc.src)
			}
		})
	}
}

// TestSpecS10_OptionalMissingSuppressesPair verifies that a missing ${?missing}
// at the end reduces the concat to the single remaining value without error.
// Spec S13 optional-omission. Status: ✅.
func TestSpecS10_OptionalMissingSuppressesPair(t *testing.T) {
	res := resolve(t, `a = [1] ${?missing}`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	if len(arr.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(arr.Elements))
	}
}

// TestSpecS10_OptionalMissingMidConcat verifies that a missing ${?missing} in
// the middle of a concat is omitted, leaving the remaining neighbours to be
// type-checked against each other. Spec S13 optional-omission + S10.4.
// Status: ✅ (Phase 6 #3b).
func TestSpecS10_OptionalMissingMidConcat(t *testing.T) {
	// [1] (omitted) {b:2} → [1] {b:2} → array+object → ERROR.
	if _, err := resolveErr(`a = [1] ${?missing} { b: 2 }`); err == nil {
		t.Error("expected resolve error: optional omitted mid-concat leaves array+object pair, which must error")
	}
}

// TestSpecS13b_2_PlusEqualsOnStringPriorValueIsError verifies that += on a key
// whose prior value is a non-array string is a resolve error. Spec L732.
// Status: ✅
func TestSpecS13b_2_PlusEqualsOnStringPriorValueIsError(t *testing.T) {
	if _, err := resolveErr("a = hello\na += world"); err == nil {
		t.Error("expected resolve error for += on string prior value, got nil")
	}
}

// TestSpecS13b_2_PlusEqualsOnIntPriorValueIsError verifies that += on a key
// whose prior value is an integer is a resolve error. Spec L732.
// Status: ✅
func TestSpecS13b_2_PlusEqualsOnIntPriorValueIsError(t *testing.T) {
	if _, err := resolveErr("a = 42\na += foo"); err == nil {
		t.Error("expected resolve error for += on int prior value, got nil")
	}
}

// TestSpecS13b_2_PlusEqualsOnObjectPriorValueIsError verifies that += on a key
// whose prior value is an object is a resolve error. Spec L732.
// Status: ✅
func TestSpecS13b_2_PlusEqualsOnObjectPriorValueIsError(t *testing.T) {
	if _, err := resolveErr("a = {x: 1}\na += foo"); err == nil {
		t.Error("expected resolve error for += on object prior value, got nil")
	}
}

// resolveErr is like resolve but returns the error (if any) rather than
// calling t.Fatalf on parse failure.
func resolveErr(src string) (*resolver.Result, error) {
	ast, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	return resolver.Resolve(ast, resolver.Options{})
}

// -----------------------------------------------------------------------------
// Spec compliance Phase 3: substitutions & includes (S13, S13a, S14a, S14b).
// -----------------------------------------------------------------------------

// TestSpecS13_13_OptionalUndefinedInStringConcatBecomesEmpty verifies that an
// optional substitution that is undefined contributes an empty string when used
// inside a string concatenation. Spec HOCON.md L636.
// e.g. `x = "pre"${?missing}"post"` → x == "prepost". Status: ✅
func TestSpecS13_13_OptionalUndefinedInStringConcatBecomesEmpty(t *testing.T) {
	res := resolve(t, `x = "pre"${?missing}"post"`)
	v, ok := res.Root.Get("x")
	if !ok {
		t.Fatal("x not found in resolved config")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		t.Fatalf("expected ScalarVal for x, got %T", v)
	}
	if sv.Raw != "prepost" {
		t.Errorf("expected x == %q, got %q", "prepost", sv.Raw)
	}
}

// TestSpecS13a_10_SubstMemoizedByInstance notes that memoization by instance
// (not by path) is an internal resolver property that cannot be observed from
// the public API without implementation-specific hooks. This test is left as a
// documentation placeholder; the behavior cannot be black-box verified.
// Spec HOCON.md L885. Status: 🤷 — not externally observable.
func TestSpecS13a_10_SubstMemoizedByInstance(t *testing.T) {
	t.Skip("S13a.10: memoization-by-instance is an internal invariant not observable via the public API")
}

// TestSpecS13a_13_OptionalSelfRefUndefinedBecomesEmpty verifies that
// `a = ${?a}foo` resolves to "foo" when `a` has no prior value; the
// look-back substitution ${?a} is undefined and contributes nothing.
// Spec HOCON.md L841. Status: ✅ — fixed in fix/s13a.13-self-ref-lookback (#68).
func TestSpecS13a_13_OptionalSelfRefUndefinedBecomesEmpty(t *testing.T) {
	res := resolve(t, `a = ${?a}foo`)
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatal("a not found in resolved config")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		t.Fatalf("expected ScalarVal for a, got %T", v)
	}
	if sv.Raw != "foo" {
		t.Errorf("expected a == %q, got %q", "foo", sv.Raw)
	}
}

// TestSpecS14b_1_ArrayRootIncludeIsError verifies that when an included file's
// root is an array (not an object), it is rejected as a resolve error.
// Spec HOCON.md L993. Status: ✅
func TestSpecS14b_1_ArrayRootIncludeIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "array-root.conf")
	if err := os.WriteFile(path, []byte("[1, 2, 3]"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// HOCON strings use '/' as a path separator across platforms; convert
	// Windows backslashes so the include argument parses correctly.
	src := `include "` + filepath.ToSlash(path) + `"`
	ast, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, resolveErr := resolver.Resolve(ast, resolver.Options{BaseDir: dir})
	if resolveErr == nil {
		t.Error("expected error when included file has array root, got nil")
	}
}

// TestSpecS13a_13_SelfRefLookback is a table-driven suite verifying the full
// matrix of S13a.13 self-referential look-back cases at the resolver layer.
// Spec HOCON.md L837–L854. Fixed in #68.
func TestSpecS13a_13_SelfRefLookback(t *testing.T) {
	type strCase struct {
		name string
		src  string
		key  string
		want string
	}
	strCases := []strCase{
		{
			name: "optional_no_prior_trailing_literal",
			src:  `a = ${?a}foo`,
			key:  "a",
			want: "foo",
		},
		{
			name: "optional_no_prior_leading_literal",
			src:  `a = bar${?a}`,
			key:  "a",
			want: "bar",
		},
		{
			name: "optional_no_prior_both_sides",
			src:  `a = bar${?a}foo`,
			key:  "a",
			want: "barfoo",
		},
		{
			name: "optional_with_prior",
			src:  "a = \"x\"\na = ${?a}foo",
			key:  "a",
			want: "xfoo",
		},
		{
			name: "required_with_prior",
			src:  "a = \"x\"\na = ${a}foo",
			key:  "a",
			want: "xfoo",
		},
	}
	for _, tc := range strCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := resolve(t, tc.src)
			v, ok := res.Root.Get(tc.key)
			if !ok {
				t.Fatalf("%s: key %q not found", tc.name, tc.key)
			}
			sv, ok := v.(*resolver.ScalarVal)
			if !ok {
				t.Fatalf("%s: expected ScalarVal, got %T", tc.name, v)
			}
			if sv.Raw != tc.want {
				t.Errorf("%s: got %q, want %q", tc.name, sv.Raw, tc.want)
			}
		})
	}

	// Required self-ref with no prior must return an error.
	t.Run("required_no_prior_errors", func(t *testing.T) {
		_, err := resolveErr(`a = ${a}foo`)
		if err == nil {
			t.Error("expected error for required self-ref with no prior, got nil")
		}
	})

	// Array cases.
	t.Run("array_optional_no_prior", func(t *testing.T) {
		res := resolve(t, `a = ${?a} [2]`)
		v, ok := res.Root.Get("a")
		if !ok {
			t.Fatal("key a not found")
		}
		arr, ok := v.(*resolver.ArrayVal)
		if !ok {
			t.Fatalf("expected ArrayVal, got %T", v)
		}
		if len(arr.Elements) != 1 {
			t.Fatalf("expected 1 element, got %d: %v", len(arr.Elements), v)
		}
		sv, ok := arr.Elements[0].(*resolver.ScalarVal)
		if !ok {
			t.Fatalf("element 0: expected ScalarVal, got %T", arr.Elements[0])
		}
		if sv.Raw != "2" {
			t.Errorf("element 0: got %q, want %q", sv.Raw, "2")
		}
	})

	t.Run("array_optional_with_prior", func(t *testing.T) {
		res := resolve(t, "a = [1]\na = ${?a} [2]")
		v, ok := res.Root.Get("a")
		if !ok {
			t.Fatal("key a not found")
		}
		arr, ok := v.(*resolver.ArrayVal)
		if !ok {
			t.Fatalf("expected ArrayVal, got %T", v)
		}
		if len(arr.Elements) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(arr.Elements))
		}
	})

	// Nested path cases.
	t.Run("nested_optional_no_prior", func(t *testing.T) {
		res := resolve(t, `foo.a = ${?foo.a}bar`)
		fooVal, ok := res.Root.Get("foo")
		if !ok {
			t.Fatal("key foo not found")
		}
		obj, ok := fooVal.(*resolver.ObjectVal)
		if !ok {
			t.Fatalf("expected ObjectVal for foo, got %T", fooVal)
		}
		aVal, ok := obj.Get("a")
		if !ok {
			t.Fatal("key foo.a not found")
		}
		sv, ok := aVal.(*resolver.ScalarVal)
		if !ok {
			t.Fatalf("expected ScalarVal for foo.a, got %T", aVal)
		}
		if sv.Raw != "bar" {
			t.Errorf("foo.a: got %q, want %q", sv.Raw, "bar")
		}
	})

	t.Run("nested_optional_with_prior", func(t *testing.T) {
		res := resolve(t, "foo.a = \"x\"\nfoo.a = ${?foo.a}bar")
		fooVal, ok := res.Root.Get("foo")
		if !ok {
			t.Fatal("key foo not found")
		}
		obj, ok := fooVal.(*resolver.ObjectVal)
		if !ok {
			t.Fatalf("expected ObjectVal for foo, got %T", fooVal)
		}
		aVal, ok := obj.Get("a")
		if !ok {
			t.Fatal("key foo.a not found")
		}
		sv, ok := aVal.(*resolver.ScalarVal)
		if !ok {
			t.Fatalf("expected ScalarVal for foo.a, got %T", aVal)
		}
		if sv.Raw != "xbar" {
			t.Errorf("foo.a: got %q, want %q", sv.Raw, "xbar")
		}
	})
}

// -----------------------------------------------------------------------------
// S13c — env-var list expansion unit tests (Steps 3–6).
// NOTE: no t.Parallel() — t.Setenv mutates the process environment.
// -----------------------------------------------------------------------------

// Step 3 / Step 4: resolveEnvList happy path — ${X[]} with X_0 and X_1 set
// returns an ArrayVal with two ScalarString elements.
func TestResolveEnvList_Basic(t *testing.T) {
	t.Setenv("S13C_RESOLVER_X_0", "a")
	t.Setenv("S13C_RESOLVER_X_1", "b")
	res, err := resolveErr(`x = ${S13C_RESOLVER_X[]}`)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	v, ok := res.Root.Get("x")
	if !ok {
		t.Fatal("key x not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T (%v)", v, v)
	}
	if len(arr.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr.Elements))
	}
	for i, want := range []string{"a", "b"} {
		sv, ok := arr.Elements[i].(*resolver.ScalarVal)
		if !ok {
			t.Errorf("element[%d]: expected ScalarVal, got %T", i, arr.Elements[i])
			continue
		}
		if sv.Raw != want {
			t.Errorf("element[%d]: want %q, got %q", i, want, sv.Raw)
		}
	}
}

// Step 5: empty-string element is preserved (stop = key absent, not value empty).
// ev10 analogue: _0="" and _1=b → ["","b"].
func TestResolveEnvList_EmptyStringElement(t *testing.T) {
	t.Setenv("S13C_RESOLVER_ES_0", "")
	t.Setenv("S13C_RESOLVER_ES_1", "b")
	res, err := resolveErr(`x = ${S13C_RESOLVER_ES[]}`)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	v, ok := res.Root.Get("x")
	if !ok {
		t.Fatal("key x not found")
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		t.Fatalf("expected ArrayVal, got %T", v)
	}
	if len(arr.Elements) != 2 {
		t.Fatalf("expected 2 elements (\"\" and \"b\"), got %d", len(arr.Elements))
	}
	sv0, ok := arr.Elements[0].(*resolver.ScalarVal)
	if !ok {
		t.Fatalf("element[0]: expected ScalarVal, got %T", arr.Elements[0])
	}
	if sv0.Raw != "" {
		t.Errorf("element[0]: want empty string, got %q", sv0.Raw)
	}
	sv1, ok := arr.Elements[1].(*resolver.ScalarVal)
	if !ok {
		t.Fatalf("element[1]: expected ScalarVal, got %T", arr.Elements[1])
	}
	if sv1.Raw != "b" {
		t.Errorf("element[1]: want \"b\", got %q", sv1.Raw)
	}
}

// Step 5: optional list with no env vars set → key removed (nil resolved value).
func TestResolveEnvList_OptionalEmpty(t *testing.T) {
	res, err := resolveErr(`x = ${?S13C_RESOLVER_OPT_NOENV[]}`)
	if err != nil {
		t.Fatalf("expected success (optional), got error: %v", err)
	}
	_, ok := res.Root.Get("x")
	if ok {
		t.Error("expected key x to be absent (optional list, no env), but it was present")
	}
}

// Step 6: S13c.5 — when listSuffix=true and no _0 env var is set, the bare
// scalar env var X must NOT be consulted as fallback.
// Required: ResolveError. Optional: key removed.
func TestResolveEnvList_NoScalarFallback_Required(t *testing.T) {
	// Set only the bare scalar key, NOT _0.
	t.Setenv("S13C_RESOLVER_NSF", "scalar-value")
	_, err := resolveErr(`x = ${S13C_RESOLVER_NSF[]}`)
	if err == nil {
		t.Fatal("expected ResolveError when listSuffix=true and no _0 env var (S13c.5 no scalar fallback), got success")
	}
}

func TestResolveEnvList_NoScalarFallback_Optional(t *testing.T) {
	// Set only the bare scalar key, NOT _0.
	t.Setenv("S13C_RESOLVER_NSFO", "scalar-value")
	res, err := resolveErr(`x = ${?S13C_RESOLVER_NSFO[]}`)
	if err != nil {
		t.Fatalf("expected success (optional), got error: %v", err)
	}
	_, ok := res.Root.Get("x")
	if ok {
		t.Error("expected key x to be absent (optional list, no _0 env, S13c.5), but it was present")
	}
}
