package parser_test

import (
	"testing"

	"github.com/o3co/go.hocon/hocon/internal/parser"
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

func TestParser_UnsupportedIncludeURL(t *testing.T) {
	_, err := parser.Parse(`include url("http://example.com/foo.conf")`)
	if err == nil {
		t.Fatal("expected error for unsupported include form")
	}
}
