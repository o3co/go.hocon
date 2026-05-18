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
	if sc.Raw != "value" || sc.ValueType != "string" {
		t.Errorf("unexpected value: Raw=%v, ValueType=%v", sc.Raw, sc.ValueType)
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
	if inc.IsFile {
		t.Error("expected IsFile=false for bare include")
	}
}

func TestParser_IncludeFileIsFile(t *testing.T) {
	obj := mustParse(t, `include file("other.conf")`)
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
	if !inc.IsFile {
		t.Error("expected IsFile=true for include file(...)")
	}
}

func TestParser_IncludeRequiredFileIsFile(t *testing.T) {
	obj := mustParse(t, `include required(file("base.conf"))`)
	inc, ok := obj.Fields[0].Value.(*parser.IncludeNode)
	if !ok {
		t.Fatalf("expected IncludeNode, got %T", obj.Fields[0].Value)
	}
	if !inc.IsFile {
		t.Error("expected IsFile=true for include required(file(...))")
	}
	if !inc.Required {
		t.Error("expected Required=true")
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
		idx      int
		wantRaw  string
		wantType string
	}{
		{0, "null", "null"},
		{1, "true", "boolean"},
		{2, "42", "number"},
		{3, "3.14", "number"},
	}
	for _, tc := range checks {
		sc, ok := obj.Fields[tc.idx].Value.(*parser.ScalarNode)
		if !ok {
			t.Errorf("[%d] expected ScalarNode, got %T", tc.idx, obj.Fields[tc.idx].Value)
			continue
		}
		if sc.Raw != tc.wantRaw || sc.ValueType != tc.wantType {
			t.Errorf("[%d] got Raw=%q ValueType=%q, want Raw=%q ValueType=%q", tc.idx, sc.Raw, sc.ValueType, tc.wantRaw, tc.wantType)
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

// -----------------------------------------------------------------------------
// Spec compliance Phase 1 (issue #57): parser-level comma rules (S5.2–S5.6).
//
// Convention: tests assert spec-correct behavior. Where the impl currently
// violates spec, use t.Skipf("spec violation, see #NN") at the top of the
// test. See lexer/lexer_test.go for the full Phase 1 convention comment.
// -----------------------------------------------------------------------------

// TestSpecS5_2_SingleTrailingCommaInArrayAllowed verifies that a single
// trailing comma in an array is silently ignored. Spec L155.
// Status: ✅
func TestSpecS5_2_SingleTrailingCommaInArrayAllowed(t *testing.T) {
	obj, err := parser.Parse("list = [1, 2, 3,]")
	if err != nil {
		t.Fatalf("expected no error for trailing comma in array, got: %v", err)
	}
	if len(obj.Fields) == 0 {
		t.Fatal("expected at least one field")
	}
	arr, ok := obj.Fields[0].Value.(*parser.ArrayNode)
	if !ok {
		t.Fatalf("expected ArrayNode, got %T", obj.Fields[0].Value)
	}
	if len(arr.Elements) != 3 {
		t.Errorf("expected 3 elements (trailing comma ignored), got %d", len(arr.Elements))
	}
}

// TestSpecS5_2_SingleTrailingCommaInObjectAllowed verifies that a single
// trailing comma in an object is silently ignored. Spec L155.
// Status: ✅
func TestSpecS5_2_SingleTrailingCommaInObjectAllowed(t *testing.T) {
	obj, err := parser.Parse("{ a = 1, b = 2, }")
	if err != nil {
		t.Fatalf("expected no error for trailing comma in object, got: %v", err)
	}
	if len(obj.Fields) != 2 {
		t.Errorf("expected 2 fields (trailing comma ignored), got %d", len(obj.Fields))
	}
}

// TestSpecS5_3_TwoTrailingCommasInArrayInvalid verifies that [1,2,3,,] is
// rejected. Spec L160. Status: ✅
func TestSpecS5_3_TwoTrailingCommasInArrayInvalid(t *testing.T) {
	if _, err := parser.Parse("list = [1,2,3,,]"); err == nil {
		t.Error("expected error for double trailing comma [1,2,3,,], got nil")
	}
}

// TestSpecS5_4_LeadingCommaInArrayInvalid verifies that [,1,2,3] is rejected.
// Spec L161. Status: ✅
func TestSpecS5_4_LeadingCommaInArrayInvalid(t *testing.T) {
	if _, err := parser.Parse("list = [,1,2,3]"); err == nil {
		t.Error("expected error for leading comma [,1,2,3], got nil")
	}
}

// TestSpecS5_5_ConsecutiveCommasInArrayInvalid verifies that [1,,2,3] is
// rejected. Spec L162. Status: ✅
func TestSpecS5_5_ConsecutiveCommasInArrayInvalid(t *testing.T) {
	if _, err := parser.Parse("list = [1,,2,3]"); err == nil {
		t.Error("expected error for consecutive commas [1,,2,3], got nil")
	}
}

// TestSpecS5_6_CommaRulesApplyToObjects verifies that the same leading/double
// comma rules that apply to arrays also apply to object fields. Spec L163.
// Status: ✅
func TestSpecS5_6_CommaRulesApplyToObjects(t *testing.T) {
	invalid := []struct {
		name string
		src  string
	}{
		{"leading comma in object", "{ ,a = 1, b = 2 }"},
		{"double comma in object", "{ a = 1,, b = 2 }"},
	}
	for _, tc := range invalid {
		if _, err := parser.Parse(tc.src); err == nil {
			t.Errorf("%s: expected error for %q, got nil", tc.name, tc.src)
		}
	}
}

// -----------------------------------------------------------------------------
// Spec compliance Phase 2: concatenation, path expressions, += (S3, S10, S11, S12, S13b).
// -----------------------------------------------------------------------------

// TestSpecS3_2_RootNonObjectNonArrayInvalid verifies that a bare scalar at the
// root (not enclosed in [] or {}) is rejected. Spec L131.
// Status: ✅ — parser rejects bare scalar/bool/null as key-less value.
func TestSpecS3_2_RootNonObjectNonArrayInvalid(t *testing.T) {
	invalid := []struct {
		name string
		src  string
	}{
		{"bare int", "42"},
		{"bare bool", "true"},
		{"bare string", "hello"},
		{"bare null", "null"},
	}
	for _, tc := range invalid {
		if _, err := parser.Parse(tc.src); err == nil {
			t.Errorf("%s: expected error for bare root value %q, got nil", tc.name, tc.src)
		}
	}
}

// TestSpecS10_7_ConcatDoesNotSpanNewline verifies that value concatenation
// stops at a newline; the token on the next line starts a new field.
// Spec L335. Status: ✅ — parser stops concat at TokenNewline.
func TestSpecS10_7_ConcatDoesNotSpanNewline(t *testing.T) {
	// "foo\nbar" must NOT be treated as a concat; "bar" on its own line must
	// cause a parse error (no separator/assignment found).
	if _, err := parser.Parse("a = foo\nbar"); err == nil {
		t.Error("expected parse error: bare word 'bar' on next line should not concat with 'foo'")
	}
}

// TestSpecS10_7_ConcatSameLineOK verifies that concatenation within a single
// line is accepted. Spec L335.
// Status: ✅
func TestSpecS10_7_ConcatSameLineOK(t *testing.T) {
	obj, err := parser.Parse(`a = foo bar`)
	if err != nil {
		t.Fatalf("expected no error for same-line concat, got: %v", err)
	}
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
}

// TestSpecS10_8_QuotedKeyWithSpaceAllowed verifies the trivial baseline:
// quoted keys with embedded whitespace are accepted as a single-element path.
// (This does NOT exercise the S10.8 spec rule itself; see
// TestSpecS10_8_UnquotedConcatInKey below.)
func TestSpecS10_8_QuotedKeyWithSpaceAllowed(t *testing.T) {
	obj, err := parser.Parse(`"foo bar" = 42`)
	if err != nil {
		t.Fatalf("expected no error for quoted key, got: %v", err)
	}
	if len(obj.Fields) != 1 || obj.Fields[0].Key[0] != "foo bar" {
		t.Errorf("expected key [foo bar], got %v", func() interface{} {
			if len(obj.Fields) > 0 {
				return obj.Fields[0].Key
			}
			return nil
		}())
	}
}

// TestSpecS10_8_UnquotedConcatInKey pins the S10.8 spec rule: unquoted
// adjacent tokens separated by whitespace form a single concatenated key.
// Per HOCON L317/L556: `a b c : 42` is equivalent to `"a b c" : 42`.
// Status: ❌ — parser rejects unquoted multi-token keys (see #65).
func TestSpecS10_8_UnquotedConcatInKey(t *testing.T) {
	t.Skipf("spec violation, see #65")
	// Spec example (L556): `a b c : 42` must parse as key "a b c" → 42.
	obj, err := parser.Parse(`a b = 1`)
	if err != nil {
		t.Fatalf("spec L317/L556: 'a b = 1' must be accepted as key 'a b', got error: %v", err)
	}
	if len(obj.Fields) != 1 || obj.Fields[0].Key[0] != "a b" {
		t.Errorf("expected key ['a b'], got %v", func() interface{} {
			if len(obj.Fields) > 0 {
				return obj.Fields[0].Key
			}
			return nil
		}())
	}
}

// TestSpecS11_4_TokenFloatKey verifies the spec assertion (L496) that key
// `10.0foo` parses as path ["10", "0foo"]. Closed by #81-followup as a side
// effect of TokenFloat key-position support with adjacent-token concat.
// Status: ✅ — see #62.
func TestSpecS11_4_TokenFloatKey(t *testing.T) {
	// Spec L496: "10.0foo" → path segments ["10", "0foo"]
	obj, err := parser.Parse(`10.0foo = x`)
	if err != nil {
		t.Fatalf("spec says 10.0foo should parse as path [10, 0foo], got error: %v", err)
	}
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	want := []string{"10", "0foo"}
	if len(obj.Fields[0].Key) != 2 || obj.Fields[0].Key[0] != want[0] || obj.Fields[0].Key[1] != want[1] {
		t.Errorf("expected key %v, got %v", want, obj.Fields[0].Key)
	}
}

// TestSpecS11_5_Foo10DotZeroPathSplit verifies that "foo10.0" parses as path
// ["foo10", "0"]. Spec L498. Status: ✅
func TestSpecS11_5_Foo10DotZeroPathSplit(t *testing.T) {
	obj, err := parser.Parse(`foo10.0 = x`)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	want := []string{"foo10", "0"}
	if len(obj.Fields[0].Key) != 2 || obj.Fields[0].Key[0] != want[0] || obj.Fields[0].Key[1] != want[1] {
		t.Errorf("expected key path %v, got %v", want, obj.Fields[0].Key)
	}
}

// TestSpecS11_8_BoolLiteralKeyStringifies pins the S11.8 spec rule: a boolean
// literal in key position must be stringified to its string form ("true" /
// "false"). Per HOCON L504: "if you have a path expression then it must always
// be converted to a string, so `true` becomes the string \"true\"."
// Status: ❌ — parser rejects TokenBool as key (stricter than spec, see #66).
func TestSpecS11_8_BoolLiteralKeyStringifies(t *testing.T) {
	t.Skipf("spec violation, see #66")
	// Spec L504: `true = 42` must parse as key "true" → 42.
	obj, err := parser.Parse(`true = 42`)
	if err != nil {
		t.Fatalf("spec L504: boolean literal in key must be stringified, got error: %v", err)
	}
	if len(obj.Fields) != 1 || obj.Fields[0].Key[0] != "true" {
		t.Errorf("expected key [\"true\"], got %v", func() interface{} {
			if len(obj.Fields) > 0 {
				return obj.Fields[0].Key
			}
			return nil
		}())
	}
}

// TestSpecS11_9_SubstitutionNotAllowedInPathExpr verifies that a substitution
// ${foo} used as a key is rejected. Spec L479. Status: ✅
func TestSpecS11_9_SubstitutionNotAllowedInPathExpr(t *testing.T) {
	if _, err := parser.Parse(`${foo} = x`); err == nil {
		t.Error("expected parse error for substitution in key/path, got nil")
	}
}

// TestSpecS12_5_IncludeMayNotBeginKeyPath verifies that `include` alone as a
// key (i.e. the literal token "include") is reserved and must not begin a path.
// Spec L570. Status: ✅ — parser treats `include` as directive keyword; bare
// `include = x` fails because no filename follows.
func TestSpecS12_5_IncludeReservedAsKeyStart(t *testing.T) {
	if _, err := parser.Parse(`include = x`); err == nil {
		t.Error("expected parse error: 'include' used as key start should be rejected")
	}
}

// TestSpecS12_5_IncludeDotFooRejected pins the S12.5 spec rule: `include` may
// NOT begin a path expression in a key, regardless of whether it stands alone
// or is the prefix of a dotted path. Per HOCON L570.
// Status: ✅ fixed in go.hocon#67.
func TestSpecS12_5_IncludeDotFooRejected(t *testing.T) {
	// Spec L570: `include` reserved as start of a path expression.
	if _, err := parser.Parse(`include.foo = x`); err == nil {
		t.Error("expected parse error: 'include.foo' begins with reserved 'include', must be rejected")
	}
}

// TestSpecS12_5_IncludeDotFooRejected_ErrorType verifies that the error raised
// for `include.foo = 1` is a *parser.Error with a message containing "reserved".
func TestSpecS12_5_IncludeDotFooRejected_ErrorType(t *testing.T) {
	_, err := parser.Parse(`include.foo = 1`)
	if err == nil {
		t.Fatal("expected parse error")
	}
	var pe *parser.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *parser.Error, got %T", err)
	}
	if !strings.Contains(pe.Message, "reserved") {
		t.Errorf("expected 'reserved' in error message, got: %s", pe.Message)
	}
}

// TestSpecS12_5_IncludeNestedObjectBodyRejected verifies that include.bar inside
// a nested object literal is also rejected per S12.5.
func TestSpecS12_5_IncludeNestedObjectBodyRejected(t *testing.T) {
	_, err := parser.Parse(`a = { include.bar = 1 }`)
	if err == nil {
		t.Fatal("expected parse error for include.bar in nested object")
	}
}

// TestSpecS12_5_IncludePlusEqualsRejected verifies that `include += [1]` is
// rejected (TokenInclude dispatches to parseInclude which errors on `+=`).
func TestSpecS12_5_IncludePlusEqualsRejected(t *testing.T) {
	_, err := parser.Parse(`include += [1]`)
	if err == nil {
		t.Fatal("expected parse error for include += [1]")
	}
}

// TestSpecS12_5_IncludeObjectBodyRejected verifies that `include { x = 1 }` is
// rejected (TokenInclude dispatches to parseInclude which errors on `{`).
func TestSpecS12_5_IncludeObjectBodyRejected(t *testing.T) {
	_, err := parser.Parse(`include { x = 1 }`)
	if err == nil {
		t.Fatal("expected parse error for include { x = 1 }")
	}
}

// Unit C — Quoted-form bypass + non-initial regression guards (S12.5 positive)

// TestSpecS12_5_QuotedIncludeAllowed verifies that `"include" = 1` is a valid
// field write when the key is quoted. Quoted form bypasses the reservation rule.
func TestSpecS12_5_QuotedIncludeAllowed(t *testing.T) {
	obj, err := parser.Parse(`"include" = 1`)
	if err != nil {
		t.Fatalf("expected no error for quoted include key, got: %v", err)
	}
	if len(obj.Fields) != 1 || obj.Fields[0].Key[0] != "include" {
		t.Errorf("expected key [include], got %v", obj.Fields)
	}
}

// TestSpecS12_5_QuotedIncludeDottedAllowed verifies that `"include".foo = 1` is
// valid: the first segment is quoted, so the reservation rule does not fire.
func TestSpecS12_5_QuotedIncludeDottedAllowed(t *testing.T) {
	obj, err := parser.Parse(`"include".foo = 1`)
	if err != nil {
		t.Fatalf("expected no error for quoted include dotted key, got: %v", err)
	}
	want := []string{"include", "foo"}
	if len(obj.Fields) != 1 || len(obj.Fields[0].Key) != 2 ||
		obj.Fields[0].Key[0] != want[0] || obj.Fields[0].Key[1] != want[1] {
		t.Errorf("expected key %v, got %v", want, obj.Fields[0].Key)
	}
}

// TestSpecS12_5_NonInitialIncludeAllowed verifies that `foo.include = 1` is
// valid: `include` is the second path element, not the first.
func TestSpecS12_5_NonInitialIncludeAllowed(t *testing.T) {
	obj, err := parser.Parse(`foo.include = 1`)
	if err != nil {
		t.Fatalf("expected no error for foo.include, got: %v", err)
	}
	want := []string{"foo", "include"}
	if len(obj.Fields) != 1 || len(obj.Fields[0].Key) != 2 ||
		obj.Fields[0].Key[0] != want[0] || obj.Fields[0].Key[1] != want[1] {
		t.Errorf("expected key %v, got %v", want, obj.Fields[0].Key)
	}
}

// Unit D — Substitution path not affected (ir14)

// TestSpecS12_5_SubstitutionIncludePathAllowed verifies that ${include} in a
// value position is NOT subject to the reservation rule (substitution paths
// are syntactically unrestricted per spec Non-Goals).
func TestSpecS12_5_SubstitutionIncludePathAllowed(t *testing.T) {
	_, err := parser.Parse("\"include\" = \"v\"\na = ${include}")
	if err != nil {
		t.Fatalf("expected no parse error for ${include} in value position, got: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Spec compliance Phase 3: substitutions & includes (S13, S13a, S14a, S14b).
// -----------------------------------------------------------------------------

// TestSpecS13_3_OptionalSubstNoWhitespaceBeforeQ verifies that `${?` is exactly
// 3 chars with no whitespace before `?`; `${ ?foo}` must NOT behave like
// `${?foo}`. Spec HOCON.md L584. Status: ✅
func TestSpecS13_3_OptionalSubstNoWhitespaceBeforeQ(t *testing.T) {
	// ${?foo} must parse successfully as an optional substitution.
	if _, err := parser.Parse(`x = ${?foo}`); err != nil {
		t.Fatalf("expected ${?foo} to parse OK, got: %v", err)
	}
	// ${ ?foo} must NOT be semantically equivalent to ${?foo}. Pin the current
	// impl behaviour (parse error) directly — the comment used to permit a
	// "parses differently" outcome but the assertion required err != nil, which
	// was inconsistent. If the parser later starts accepting ${ ?foo}, this test
	// must be revisited: confirm the parsed shape is NOT an optional substitution
	// (e.g. ? became part of the path) before relaxing the error check.
	if _, err := parser.Parse(`x = ${ ?foo}`); err == nil {
		t.Error("expected parse error for ${ ?foo} (whitespace before ?), got nil")
	}
}

// TestSpecS13_5_NoSubstInQuotedString verifies that substitutions are NOT
// parsed inside quoted strings; `x = "${foo}"` has a string value of `${foo}`.
// Spec HOCON.md L593. Status: ✅
func TestSpecS13_5_NoSubstInQuotedString(t *testing.T) {
	ast, err := parser.Parse(`x = "${foo}"`)
	if err != nil {
		t.Fatalf("expected no parse error for x = \"${foo}\", got: %v", err)
	}
	if len(ast.Fields) == 0 {
		t.Fatal("expected 1 field")
	}
	// The field value must be a string-typed scalar carrying the literal `${foo}`
	// — assert shape AND contents to rule out wrapped/concat encodings (e.g.
	// ConcatNode{[ScalarNode("${foo}")]}, or ScalarNode with the wrong type tag)
	// that would also pass a bare "not SubstNode" check.
	sc, ok := ast.Fields[0].Value.(*parser.ScalarNode)
	if !ok {
		t.Fatalf("expected *parser.ScalarNode, got %T", ast.Fields[0].Value)
	}
	if sc.Raw != "${foo}" {
		t.Errorf("expected scalar raw = %q, got %q", "${foo}", sc.Raw)
	}
	if sc.ValueType != "string" {
		t.Errorf("expected scalar value type = %q, got %q", "string", sc.ValueType)
	}
}

// TestSpecS13_16_SubstOnlyInFieldValuesNotKeys verifies that a substitution
// cannot appear in key position; `${foo} = 1` must be a parse error.
// Spec HOCON.md L644. Status: ✅
func TestSpecS13_16_SubstOnlyInFieldValuesNotKeys(t *testing.T) {
	if _, err := parser.Parse(`${foo} = 1`); err == nil {
		t.Error("expected parse error for substitution in key position, got nil")
	}
}

// TestSpecS14a_6_UnquotedIncludeNonStartOfKeyIsLiteral verifies that when
// `include` appears after a dot (i.e. it is NOT the start of a key path), it
// is treated as a normal identifier, not a directive keyword.
// Spec HOCON.md L962. Status: ✅
func TestSpecS14a_6_UnquotedIncludeNonStartOfKeyIsLiteral(t *testing.T) {
	ast, err := parser.Parse(`x.include = 1`)
	if err != nil {
		t.Fatalf("expected no parse error for x.include = 1, got: %v", err)
	}
	if len(ast.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(ast.Fields))
	}
	// Key must be ["x", "include"] — two path segments.
	key := ast.Fields[0].Key
	if len(key) != 2 || key[0] != "x" || key[1] != "include" {
		t.Errorf("expected key [x include], got %v", key)
	}
}

// TestSpecS14a_8_NoValueConcatOnIncludeArg verifies that value concatenation is
// not allowed on an include argument; `include "a.conf" "b.conf"` must be a
// parse error. Spec HOCON.md L957. Status: ✅
func TestSpecS14a_8_NoValueConcatOnIncludeArg(t *testing.T) {
	if _, err := parser.Parse(`include "a.conf" "b.conf"`); err == nil {
		t.Error("expected parse error for include with multiple filenames, got nil")
	}
}

// TestSpecS14a_9_NoSubstInIncludeArg verifies that substitutions are not
// allowed as the include argument; `include ${path}` must be a parse error.
// Spec HOCON.md L959. Status: ✅
func TestSpecS14a_9_NoSubstInIncludeArg(t *testing.T) {
	if _, err := parser.Parse(`include ${path}`); err == nil {
		t.Error("expected parse error for include with substitution argument, got nil")
	}
}
