package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
)

func mustParse(t *testing.T, src string) *parser.ObjectNode {
	t.Helper()
	node, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	return node
}

func TestParser_SimpleKV(t *testing.T) {
	obj := mustParse(t, `key = "value"`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	f := obj.Fields[0]
	if len(f.Key) != 1 || f.Key[0] != "key" {
		t.Errorf("unexpected key: %v", f.Key)
	}
	sc, ok := f.Value.(*parser.ScalarNode)
	if !ok {
		t.Fatalf("expected ScalarNode, got %T", f.Value)
	}
	if sc.Value != "value" {
		t.Errorf("unexpected value: %v", sc.Value)
	}
}

func TestParser_DotNotation(t *testing.T) {
	obj := mustParse(t, `a.b = 1`)
	f := obj.Fields[0]
	if len(f.Key) != 2 || f.Key[0] != "a" || f.Key[1] != "b" {
		t.Errorf("unexpected key: %v", f.Key)
	}
}

func TestParser_NestedObject(t *testing.T) {
	obj := mustParse(t, `a { b = 2 }`)
	if len(obj.Fields) == 0 {
		t.Fatal("expected fields")
	}
	inner, ok := obj.Fields[0].Value.(*parser.ObjectNode)
	if !ok {
		t.Fatalf("expected ObjectNode, got %T", obj.Fields[0].Value)
	}
	if len(inner.Fields) != 1 {
		t.Errorf("expected 1 inner field, got %d", len(inner.Fields))
	}
}

func TestParser_Array(t *testing.T) {
	obj := mustParse(t, `arr = [1, 2, 3]`)
	arr, ok := obj.Fields[0].Value.(*parser.ArrayNode)
	if !ok {
		t.Fatalf("expected ArrayNode, got %T", obj.Fields[0].Value)
	}
	if len(arr.Elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestParser_Substitution(t *testing.T) {
	obj := mustParse(t, `x = ${foo}`)
	sub, ok := obj.Fields[0].Value.(*parser.SubstNode)
	if !ok {
		t.Fatalf("expected SubstNode, got %T", obj.Fields[0].Value)
	}
	if sub.Path != "foo" || sub.Optional {
		t.Errorf("unexpected: %+v", sub)
	}
}

func TestParser_OptionalSubstitution(t *testing.T) {
	obj := mustParse(t, `x = ${?foo}`)
	sub, ok := obj.Fields[0].Value.(*parser.SubstNode)
	if !ok {
		t.Fatalf("expected SubstNode, got %T", obj.Fields[0].Value)
	}
	if !sub.Optional {
		t.Error("expected Optional=true")
	}
}

func TestParser_Include(t *testing.T) {
	obj := mustParse(t, `include "other.conf"`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field (include), got %d", len(obj.Fields))
	}
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Path != "other.conf" {
		t.Errorf("unexpected path: %s", inc.Path)
	}
}

func TestParser_PlusEquals(t *testing.T) {
	obj := mustParse(t, `arr += 1`)
	if !obj.Fields[0].Append {
		t.Error("expected Append=true for +=")
	}
}

func TestParser_NullBoolNumbers(t *testing.T) {
	obj := mustParse(t, "a=null\nb=true\nc=42\nd=3.14")
	checks := []struct {
		idx  int
		want any
	}{
		{0, nil},
		{1, true},
		{2, int64(42)},
		{3, float64(3.14)},
	}
	for _, tc := range checks {
		sc, ok := obj.Fields[tc.idx].Value.(*parser.ScalarNode)
		if !ok {
			t.Errorf("[%d] expected ScalarNode, got %T", tc.idx, obj.Fields[tc.idx].Value)
			continue
		}
		if sc.Value != tc.want {
			t.Errorf("[%d] got %v (%T), want %v", tc.idx, sc.Value, sc.Value, tc.want)
		}
	}
}

func TestBracedRootObjectConcat(t *testing.T) {
	obj := mustParse(t, "{ a = 1 } { b = 2 }")
	hasA, hasB := false, false
	for _, f := range obj.Fields {
		if len(f.Key) > 0 && f.Key[0] == "a" {
			hasA = true
		}
		if len(f.Key) > 0 && f.Key[0] == "b" {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Errorf("expected both a and b in merged root, got fields: %v", obj.Fields)
	}
}

func TestBracedRootWithTrailingFields(t *testing.T) {
	obj := mustParse(t, "{ a = 1 }\nb = 2")
	hasA, hasB := false, false
	for _, f := range obj.Fields {
		if len(f.Key) > 0 && f.Key[0] == "a" {
			hasA = true
		}
		if len(f.Key) > 0 && f.Key[0] == "b" {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Errorf("expected both a and b, got fields: %v", obj.Fields)
	}
}

func TestBracedRootMultipleObjects(t *testing.T) {
	obj := mustParse(t, "{ a = 1 }\n{ b = 2 }\n{ c = 3 }")
	if len(obj.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(obj.Fields))
	}
}

func TestBracedRootWithTrailingCommentsOK(t *testing.T) {
	// Trailing comments after a braced root should be fine
	inputs := []string{
		"{ a = 1 }\n# comment",
		"{ a = 1 }\n// comment",
		"{ a = 1 }\n",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			_, err := parser.Parse(input)
			if err != nil {
				t.Errorf("unexpected error for valid input %q: %v", input, err)
			}
		})
	}
}

func TestBracedRootParseError(t *testing.T) {
	// Braced root with a syntax error inside — exercises the error return
	// in parseRoot when parseObject fails (e.g., unclosed brace, bad field).
	tests := []string{
		`{ a = 1`,        // missing closing brace
		`{ a = 1; = bad`, // invalid field inside braced root
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parser.Parse(input)
			if err == nil {
				t.Errorf("expected error for malformed braced root: %s", input)
			}
		})
	}
}

func TestParserUnterminatedString(t *testing.T) {
	_, err := parser.Parse(`a = "unterminated`)
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("expected error to mention 'unterminated', got: %v", err)
	}
}

func TestParserUnterminatedSubstitution(t *testing.T) {
	_, err := parser.Parse(`a = ${unclosed`)
	if err == nil {
		t.Fatal("expected error for unterminated substitution")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("expected error to mention 'unterminated', got: %v", err)
	}
}

func TestParserUnterminatedTripleQuote(t *testing.T) {
	_, err := parser.Parse(`a = """unterminated`)
	if err == nil {
		t.Fatal("expected error for unterminated triple-quoted string")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("expected error to mention 'unterminated', got: %v", err)
	}
}

func TestParser_UnsupportedIncludeURL(t *testing.T) {
	_, err := parser.Parse(`include url("http://example.com/foo.conf")`)
	if err == nil {
		t.Fatal("expected error for unsupported include form")
	}
}

func TestParser_UnsupportedIncludeClasspath(t *testing.T) {
	_, err := parser.Parse(`include classpath("reference.conf")`)
	if err == nil {
		t.Fatal("expected error for unsupported include classpath() form")
	}
}

func TestParser_IncludeRequired(t *testing.T) {
	obj := mustParse(t, `include required("base.conf")`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field (include), got %d", len(obj.Fields))
	}
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Path != "base.conf" {
		t.Errorf("unexpected path: %s", inc.Path)
	}
	if !inc.Required {
		t.Error("expected Required=true for include required(...)")
	}
}

func TestParser_IncludeRequiredFile(t *testing.T) {
	obj := mustParse(t, `include required(file("base.conf"))`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field (include), got %d", len(obj.Fields))
	}
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Path != "base.conf" {
		t.Errorf("unexpected path: %s", inc.Path)
	}
	if !inc.Required {
		t.Error("expected Required=true for include required(file(...))")
	}
}

func TestParser_IncludeNotRequired(t *testing.T) {
	obj := mustParse(t, `include "base.conf"`)
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Required {
		t.Error("expected Required=false for plain include")
	}
}

func TestParser_IncludeRequiredUrlNotSupported(t *testing.T) {
	_, err := parser.Parse(`include required(url("http://example.com"))`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' in error, got: %s", err.Error())
	}
}

func TestParser_IncludeRequiredClasspathNotSupported(t *testing.T) {
	_, err := parser.Parse(`include required(classpath("reference.conf"))`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' in error, got: %s", err.Error())
	}
}

// TestParser_IncludeQuotedUrlNotError verifies that a quoted filename that
// happens to be "url" is treated as a plain file path, not an unsupported form.
func TestParser_IncludeQuotedUrlNotError(t *testing.T) {
	// include "url" — "url" is a quoted string, so it must be accepted as a filename.
	obj := mustParse(t, `include "url"`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Path != "url" {
		t.Errorf("unexpected path: %s", inc.Path)
	}
}

// TestParser_IncludeRequiredQuotedUrlNotError verifies that include required("url")
// is accepted as a valid file path include, not rejected as url(...) form.
func TestParser_IncludeRequiredQuotedUrlNotError(t *testing.T) {
	obj := mustParse(t, `include required("url")`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Path != "url" {
		t.Errorf("unexpected path: %s", inc.Path)
	}
	if !inc.Required {
		t.Error("expected Required=true")
	}
}

// TestParser_IncludeRequiredQuotedClasspathNotError verifies that include required("classpath")
// is accepted as a valid file path include, not rejected as classpath(...) form.
func TestParser_IncludeRequiredQuotedClasspathNotError(t *testing.T) {
	obj := mustParse(t, `include required("classpath")`)
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if inc.Path != "classpath" {
		t.Errorf("unexpected path: %s", inc.Path)
	}
	if !inc.Required {
		t.Error("expected Required=true")
	}
}

func TestParser_ErrorCarriesLineCol(t *testing.T) {
	// Unclosed brace on line 1 — parser should return an error with line/col info.
	_, err := parser.Parse("{ a = 1")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *parser.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *parser.Error, got %T: %v", err, err)
	}
	if pe.Line == 0 {
		t.Error("expected Line > 0 in parser.Error")
	}
	if pe.Col == 0 {
		t.Error("expected Col > 0 in parser.Error")
	}
}

func TestBracedRootTrailingGarbage(t *testing.T) {
	tests := []string{
		`{ a = 1 } }`,
		`{ a = 1 } garbage`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parser.Parse(input)
			if err == nil {
				t.Errorf("expected error for trailing content: %s", input)
			}
		})
	}
}
