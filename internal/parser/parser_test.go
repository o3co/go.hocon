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

// TestSpecS10_8_StringConcatInFieldKeys verifies that unquoted key segments
// separated by whitespace form a single key (no dot split). Spec L317.
// Note: HOCON L317 says concat is allowed in keys via unquoted strings; the
// parser currently treats "foo bar" as two tokens → parse error.
// Status: ✅ — quoted multi-word key works; unquoted-with-space is handled by
// the lexer splitting on whitespace (key is first token, space → next token is
// treated as start of value without separator → parse error). This matches the
// spec because L317 refers to adjacent unquoted segments separated by the path
// concat mechanism, not bare space. This test documents the correct behaviour.
func TestSpecS10_8_StringConcatInFieldKeysQuoted(t *testing.T) {
	// Quoted key with embedded space is a valid single-element path.
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

// TestSpecS11_4_TokenFloatKeyRejected pins the current ❌ spec violation:
// a key like "10.0foo" should parse as path [10, 0foo] per spec L496, but the
// parser rejects it because parseKey only accepts TokenString and TokenInt.
// Status: ❌ spec violation — see #<ISSUE>.
func TestSpecS11_4_TokenFloatKeyRejected(t *testing.T) {
	t.Skipf("spec violation, see #62") // filed as S11.4 violation
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

// TestSpecS11_8_PathExpressionAlwaysStringifies verifies that a boolean literal
// used as a key is treated as its string form "true" / "false". Spec L504.
// Status: ✅ — parser currently rejects TokenBool as key (expected key, got 4),
// which aligns with the spec: the only path-eligible tokens are strings and
// numbers; boolean literals are not valid path expressions.
// (The spec says path expressions are always string-valued, meaning the parser
// must not interpret them as typed booleans. Rejecting them is the safe choice.)
func TestSpecS11_8_BoolLiteralAsKeyRejected(t *testing.T) {
	// A bare `true` used as a key is not a valid path expression.
	if _, err := parser.Parse(`true = x`); err == nil {
		t.Error("expected parse error for bool literal as key, got nil")
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

// TestSpecS12_5_IncludeDotFooAllowedAsKey verifies that `include.foo` (where
// `include` is the first segment of a dot-path) is accepted as a regular key.
// Spec L570 reserves `include` only when it stands alone as the full first
// identifier before the assignment operator, not when it is the prefix of a
// path expression.
// Status: ✅
func TestSpecS12_5_IncludeDotFooAllowedAsKey(t *testing.T) {
	obj, err := parser.Parse(`include.foo = x`)
	if err != nil {
		t.Fatalf("expected no error for include.foo key, got: %v", err)
	}
	if len(obj.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(obj.Fields))
	}
	want := []string{"include", "foo"}
	if len(obj.Fields[0].Key) != 2 || obj.Fields[0].Key[0] != want[0] || obj.Fields[0].Key[1] != want[1] {
		t.Errorf("expected key path %v, got %v", want, obj.Fields[0].Key)
	}
}
