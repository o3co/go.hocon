package lexer_test

import (
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/lexer"
)

func tokenTypes(src string) []lexer.TokenType {
	l := lexer.New(src)
	var types []lexer.TokenType
	for {
		tok := l.Next()
		types = append(types, tok.Type)
		if tok.Type == lexer.TokenEOF {
			break
		}
	}
	return types
}

func TestLexer_BraceColon(t *testing.T) {
	types := tokenTypes(`{ key: "val" }`)
	want := []lexer.TokenType{
		lexer.TokenLBrace,
		lexer.TokenString, // key (unquoted)
		lexer.TokenColon,
		lexer.TokenString, // "val"
		lexer.TokenRBrace,
		lexer.TokenEOF,
	}
	if len(types) != len(want) {
		t.Fatalf("got %v, want %v", types, want)
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("token[%d] = %v, want %v", i, types[i], w)
		}
	}
}

func TestLexer_Comment(t *testing.T) {
	types := tokenTypes("# comment\nkey = val")
	// comment is skipped; newline after comment emitted
	for _, tt := range types {
		if tt == lexer.TokenInvalid {
			t.Fatal("unexpected TokenInvalid")
		}
	}
}

func TestLexer_Substitution(t *testing.T) {
	types := tokenTypes("${foo.bar}")
	if types[0] != lexer.TokenSubstitution {
		t.Errorf("expected TokenSubstitution, got %v", types[0])
	}
}

func TestLexer_OptSubstitution(t *testing.T) {
	l := lexer.New("${?foo}")
	tok := l.Next()
	if tok.Type != lexer.TokenSubstitution {
		t.Errorf("expected TokenSubstitution, got %v", tok.Type)
	}
	if tok.Subst == nil || !tok.Subst.Optional {
		t.Errorf("expected Subst.Optional=true, got %+v", tok.Subst)
	}
}

func TestLexer_Numbers(t *testing.T) {
	tests := []struct {
		src  string
		want lexer.TokenType
	}{
		{"42", lexer.TokenInt},
		{"3.14", lexer.TokenFloat},
		{"1e5", lexer.TokenFloat},
	}
	for _, tc := range tests {
		types := tokenTypes(tc.src)
		if types[0] != tc.want {
			t.Errorf("src=%q: got %v, want %v", tc.src, types[0], tc.want)
		}
	}
}

func TestReadNumberScientific(t *testing.T) {
	tests := []struct {
		input string
		want  string
		tt    lexer.TokenType
	}{
		{"1.5e3", "1.5e3", lexer.TokenFloat},
		{"1.5E3", "1.5E3", lexer.TokenFloat},
		{"1.5e+3", "1.5e+3", lexer.TokenFloat},
		{"1.5e-3", "1.5e-3", lexer.TokenFloat},
		{"2.0E10", "2.0E10", lexer.TokenFloat},
		{"3e5", "3e5", lexer.TokenFloat},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			l := lexer.New(tc.input)
			tok := l.Next()
			if tok.Value != tc.want {
				t.Errorf("got value %q, want %q", tok.Value, tc.want)
			}
			if tok.Type != tc.tt {
				t.Errorf("got type %v, want %v", tok.Type, tc.tt)
			}
		})
	}
}

func TestLexer_PlusEquals(t *testing.T) {
	types := tokenTypes("+=")
	if types[0] != lexer.TokenPlusEquals {
		t.Errorf("expected TokenPlusEquals, got %v", types[0])
	}
}

func TestLexer_TripleQuoted(t *testing.T) {
	src := `"""hello\nworld"""`
	l := lexer.New(src)
	tok := l.Next()
	if tok.Type != lexer.TokenString {
		t.Fatalf("expected TokenString, got %v", tok.Type)
	}
	// backslash not processed — literal content
	if tok.Value != `hello\nworld` {
		t.Errorf("expected raw content, got %q", tok.Value)
	}
}

// tokenize collects all tokens (including EOF) from the input.
func tokenize(src string) []lexer.Token {
	l := lexer.New(src)
	var tokens []lexer.Token
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Type == lexer.TokenEOF {
			break
		}
	}
	return tokens
}

func TestUnterminatedString(t *testing.T) {
	tests := []string{
		`a = "unterminated`,
		`a = "no close`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			tokens := tokenize(input)
			hasError := false
			for _, tok := range tokens {
				if tok.Type == lexer.TokenError {
					hasError = true
				}
			}
			if !hasError {
				t.Errorf("expected error token for unterminated string in: %s", input)
			}
		})
	}
}

func TestUnterminatedSubstitution(t *testing.T) {
	tokens := tokenize(`a = ${unclosed`)
	hasError := false
	for _, tok := range tokens {
		if tok.Type == lexer.TokenError {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected error token for unterminated substitution")
	}
}

func TestUnterminatedTripleQuotedString(t *testing.T) {
	tests := []string{
		`a = """unterminated`,
		"a = \"\"\"line1\nline2",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			tokens := tokenize(input)
			hasError := false
			for _, tok := range tokens {
				if tok.Type == lexer.TokenError {
					hasError = true
				}
			}
			if !hasError {
				t.Errorf("expected error token for unterminated triple-quoted string in: %s", input)
			}
		})
	}
}

func TestLexer_LineCol(t *testing.T) {
	l := lexer.New("a\nb")
	tok := l.Next() // 'a' unquoted string
	if tok.Line != 1 || tok.Col != 1 {
		t.Errorf("a: line=%d col=%d, want 1,1", tok.Line, tok.Col)
	}
}

func TestUnicodeEscape(t *testing.T) {
	tests := []struct{ input, want string }{
		{`"\u0041"`, "A"},
		{`"\u00e9"`, "é"},
	}
	for _, tc := range tests {
		tokens := tokenize(tc.input)
		if tokens[0].Value != tc.want {
			t.Errorf("tokenize(%s) = %q, want %q", tc.input, tokens[0].Value, tc.want)
		}
	}
}

func TestUnicodeEscapeInvalid(t *testing.T) {
	tests := []string{`"\uZZZZ"`, `"\u41"`, `"\u"`}
	for _, input := range tests {
		tokens := tokenize(input)
		hasError := false
		for _, tok := range tokens {
			if tok.Type == lexer.TokenError {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("expected error for %s", input)
		}
	}
}

func TestUnquotedParenthesesProduceSeparateTokens(t *testing.T) {
	// Parentheses are forbidden in unquoted strings so that
	// `include file(...)` / `include required(...)` can be parsed.
	// They should produce dedicated LParen/RParen tokens.
	tokens := tokenize("key = foo(bar)")
	hasLParen := false
	hasRParen := false
	for _, tok := range tokens {
		if tok.Type == lexer.TokenLParen {
			hasLParen = true
		}
		if tok.Type == lexer.TokenRParen {
			hasRParen = true
		}
	}
	if !hasLParen || !hasRParen {
		t.Errorf("parentheses should produce LParen/RParen tokens, got: %v", tokens)
	}
}

func TestUnquotedStarForbidden(t *testing.T) {
	tokens := tokenize("key = foo*bar")
	for _, tok := range tokens {
		if tok.Type == lexer.TokenString && strings.Contains(tok.Value, "*") {
			t.Errorf("* should be forbidden in unquoted strings, got token: %q", tok.Value)
		}
	}
}

// substSegments is a test helper that tokenizes input and returns the Segments
// from the first TokenSubstitution token found.
func substSegments(t *testing.T, input string) []lexer.Segment {
	t.Helper()
	toks, err := lexer.Tokenize(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range toks {
		if tk.Type == lexer.TokenSubstitution {
			return tk.Subst.Segments
		}
	}
	t.Fatalf("no subst token in %q", input)
	return nil
}

func TestSegmentPositionUnquoted(t *testing.T) {
	segs := substSegments(t, "${foo.bar}")
	if segs[0].Text != "foo" || segs[0].Line != 1 || segs[0].Col != 3 {
		t.Errorf("seg0 = %+v", segs[0])
	}
	if segs[1].Text != "bar" || segs[1].Line != 1 || segs[1].Col != 7 {
		t.Errorf("seg1 = %+v", segs[1])
	}
}

func TestSegmentPositionQuoted(t *testing.T) {
	segs := substSegments(t, `${"a"."b"}`)
	if segs[0].Text != "a" || segs[0].Col != 3 {
		t.Errorf("seg0 = %+v", segs[0])
	}
	if segs[1].Text != "b" || segs[1].Line != 1 || segs[1].Col != 7 {
		t.Errorf("seg1 = %+v", segs[1])
	}
}

func TestSegmentPositionMultiline(t *testing.T) {
	segs := substSegments(t, "x=1\ny=${foo}")
	if segs[0].Line != 2 || segs[0].Col != 5 {
		t.Errorf("seg0 = %+v", segs[0])
	}
}

func TestSegmentPositionWSConcat(t *testing.T) {
	// Whitespace between simple values is preserved in the segment text.
	segs := substSegments(t, `${"a" "b"}`)
	if len(segs) != 1 || segs[0].Text != "a b" || segs[0].Col != 3 {
		t.Errorf("segs = %+v", segs)
	}
}

func TestSegmentPositionEmptyKey(t *testing.T) {
	segs := substSegments(t, `${""}`)
	if len(segs) != 1 || segs[0].Text != "" || segs[0].Col != 3 {
		t.Errorf("segs = %+v", segs)
	}
}

func TestErrorPositionInsideSubstBody(t *testing.T) {
	// Goal 2: invalid escape error points inside ${...} body.
	l := lexer.New(`x=${"a\xb"}`)
	var errTok *lexer.Token
	for {
		tok := l.Next()
		if tok.Type == lexer.TokenError {
			errTok = &tok
			break
		}
		if tok.Type == lexer.TokenEOF {
			break
		}
	}
	if errTok == nil {
		t.Fatal("expected error token")
	}
	if errTok.Line != 1 {
		t.Errorf("expected line 1 in error, got line %d", errTok.Line)
	}
	if errTok.Col < 7 || errTok.Col > 8 {
		t.Errorf("expected col 7 or 8 in error, got col %d", errTok.Col)
	}
}

func TestErrorPositionEmptyPath(t *testing.T) {
	l := lexer.New("x=${}")
	var errTok *lexer.Token
	for {
		tok := l.Next()
		if tok.Type == lexer.TokenError {
			errTok = &tok
			break
		}
		if tok.Type == lexer.TokenEOF {
			break
		}
	}
	if errTok == nil {
		t.Fatal("expected error token")
	}
	if errTok.Col < 3 || errTok.Col > 4 {
		t.Errorf("expected col 3 or 4 in error, got col %d", errTok.Col)
	}
}

func TestUnknownEscapeError(t *testing.T) {
	tests := []string{`"hello\qworld"`, `"\a"`}
	for _, input := range tests {
		tokens := tokenize(input)
		hasError := false
		for _, tok := range tokens {
			if tok.Type == lexer.TokenError {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("expected error for unknown escape in %s", input)
		}
	}
}

// -----------------------------------------------------------------------------
// Spec compliance Phase 1 (issue #57): lexer-level rules.
//
// Each test is annotated with its xx.hocon spec checklist ID (S<n>.<m>).
// Where the current implementation diverges from spec, the test body calls
//
//	t.Skipf("spec violation, see #NN")
//
// as its first statement. This wires the spec-correct assertion while keeping
// CI green (skipped tests are reported but not failed). When the underlying
// bug is fixed, remove the t.Skipf call and promote the
// docs/spec-compliance.md status row from ❌ / ⚠️ to ✅ or ⚠️ (partial).
// -----------------------------------------------------------------------------

// TestSpecS2_3_CommentMarkersInQuotedString verifies that // and # inside a
// quoted string are treated as literal characters, not comment starts.
// Spec L126.
func TestSpecS2_3_CommentMarkersInQuotedString(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"http://example.com"`, "http://example.com"},
		{`"# not a comment"`, "# not a comment"},
	}
	for _, tc := range cases {
		toks := tokenize(tc.src)
		if len(toks) == 0 {
			t.Fatalf("src=%q: no tokens", tc.src)
		}
		tok := toks[0]
		if tok.Type != lexer.TokenString {
			t.Errorf("src=%q: got type %v, want TokenString", tc.src, tok.Type)
		}
		if !tok.IsQuoted {
			t.Errorf("src=%q: IsQuoted=false, want true", tc.src)
		}
		if tok.Value != tc.want {
			t.Errorf("src=%q: value=%q, want %q", tc.src, tok.Value, tc.want)
		}
	}
}

// TestSpecS6_1_UnicodeCategoryZsIsWhitespace verifies that Unicode Zs-category
// characters (e.g. em space U+2003) are treated as token separators.
// Spec L170.
// Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_1_UnicodeCategoryZsIsWhitespace(t *testing.T) {
	// Em space (U+2003, Zs) between two unquoted tokens should act as a
	// separator, producing two separate TokenString tokens with no error.
	src := "a\u2003b"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("src=%q: got unexpected error token: %q", src, tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("src=%q: got string tokens %v, want [a b]", src, strings_)
	}
}

// TestSpecS6_1_UnicodeCategoryZlIsWhitespace verifies that line separator
// (U+2028, Zl) is treated as whitespace. Spec L170.
// Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_1_UnicodeCategoryZlIsWhitespace(t *testing.T) {
	// Line separator (U+2028, Zl) should separate two unquoted tokens with no error.
	src := "a\u2028b"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("src=%q: got unexpected error token: %q", src, tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("src=%q: got string tokens %v, want [a b]", src, strings_)
	}
}

// TestSpecS6_1_UnicodeCategoryZpIsWhitespace verifies that paragraph separator
// (U+2029, Zp) is treated as whitespace. Spec L170 covers Zs/Zl/Zp; this is the
// Zp half. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_1_UnicodeCategoryZpIsWhitespace(t *testing.T) {
	// Paragraph separator (U+2029, Zp) should separate two unquoted tokens with no error.
	src := "a\u2029b"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("src=%q: got unexpected error token: %q", src, tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("src=%q: got string tokens %v, want [a b]", src, strings_)
	}
}

// TestSpecS6_2_NBSPIsWhitespace verifies that NBSP (U+00A0) is whitespace.
// Spec L171. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_2_NBSPIsWhitespace(t *testing.T) {
	src := "a\u00a0b"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("NBSP: got unexpected error token: %q", tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("NBSP: got string tokens %v, want [a b]", strings_)
	}
}

// TestSpecS6_2_FigureSpaceIsWhitespace verifies figure space (U+2007).
// Spec L171. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_2_FigureSpaceIsWhitespace(t *testing.T) {
	src := "a\u2007b"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("figure space: got unexpected error token: %q", tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("figure space: got string tokens %v, want [a b]", strings_)
	}
}

// TestSpecS6_2_NarrowNBSPIsWhitespace verifies narrow no-break space (U+202F).
// Spec L171. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_2_NarrowNBSPIsWhitespace(t *testing.T) {
	src := "a\u202fb"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("narrow NBSP: got unexpected error token: %q", tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("narrow NBSP: got string tokens %v, want [a b]", strings_)
	}
}

// TestSpecS6_4_TabIsWhitespace verifies tab (0x09) is a whitespace separator.
// Spec L174. Status: ✅
func TestSpecS6_4_TabIsWhitespace(t *testing.T) {
	src := "a\tb"
	toks := tokenize(src)
	var strToks []lexer.Token
	for _, tok := range toks {
		if tok.Type == lexer.TokenString {
			strToks = append(strToks, tok)
		}
	}
	if len(strToks) != 2 || strToks[0].Value != "a" || strToks[1].Value != "b" {
		t.Errorf("tab: got string tokens %v, want [a b]", strToks)
	}
}

// TestSpecS6_4_CRIsWhitespace verifies carriage return (0x0D) is whitespace.
// Spec L174. Status: ✅
func TestSpecS6_4_CRIsWhitespace(t *testing.T) {
	src := "a\rb"
	toks := tokenize(src)
	var strToks []lexer.Token
	for _, tok := range toks {
		if tok.Type == lexer.TokenString {
			strToks = append(strToks, tok)
		}
	}
	if len(strToks) != 2 || strToks[0].Value != "a" || strToks[1].Value != "b" {
		t.Errorf("CR: got string tokens %v, want [a b]", strToks)
	}
}

// TestSpecS6_4_VtabIsWhitespace verifies vertical tab (0x0B) is whitespace.
// Spec L174. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_4_VtabIsWhitespace(t *testing.T) {
	src := "a\x0bb"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("vtab: got unexpected error token: %q", tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("vtab: got string tokens %v, want [a b]", strings_)
	}
}

// TestSpecS6_4_FFIsWhitespace verifies form feed (0x0C) is whitespace.
// Spec L174. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_4_FFIsWhitespace(t *testing.T) {
	src := "a\x0cb"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("FF: got unexpected error token: %q", tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("FF: got string tokens %v, want [a b]", strings_)
	}
}

// TestSpecS6_4_SeparatorsAreWhitespace verifies FS/GS/RS/US (0x1C-0x1F) are
// whitespace. Spec L174. Status: ✅ fixed in fix/s6-whitespace-expansion (was: ❌ #59)
func TestSpecS6_4_SeparatorsAreWhitespace(t *testing.T) {
	// FS=0x1C, GS=0x1D, RS=0x1E, US=0x1F — all must act as whitespace
	for _, ch := range []rune{'\x1c', '\x1d', '\x1e', '\x1f'} {
		src := "a" + string(ch) + "b"
		toks := tokenize(src)
		var strings_ []string
		for _, tok := range toks {
			if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
				t.Errorf("U+%04X: got unexpected error token: %q", ch, tok.Value)
			}
			if tok.Type == lexer.TokenString {
				strings_ = append(strings_, tok.Value)
			}
		}
		if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
			t.Errorf("U+%04X: got string tokens %v, want [a b]", ch, strings_)
		}
	}
}

// TestSpecS8_6_DigitStartUnquotedRejected verifies that an unquoted string
// starting with a digit (e.g. "123abc") is rejected. Spec L270.
// Status: ❌ spec violation — lexer tokenizes "123abc" as TokenInt("123") +
// TokenString("abc") instead of rejecting. See issue #60.
func TestSpecS8_6_DigitStartUnquotedRejected(t *testing.T) {
	t.Skipf("spec violation, see #60")
	// "x = 123abc": the value "123abc" starts with a digit and is not a valid
	// JSON number, so it should be rejected (parse error).
	_, err := lexer.Tokenize("123abc")
	if err == nil {
		t.Error("expected error for digit-starting unquoted string '123abc', got nil")
	}
}

// TestSpecS8_6_HyphenStartUnquotedRejected verifies that "-foo" (not a valid
// JSON number) is rejected. Spec L270.
// Status: ❌ spec violation — lexer tokenizes "-foo" as TokenInt("-") +
// TokenString("foo"). See issue #60.
func TestSpecS8_6_HyphenStartUnquotedRejected(t *testing.T) {
	t.Skipf("spec violation, see #60")
	// "-foo" starts with '-' and is not a valid JSON number, so it should
	// be rejected. "-123" is a valid number and is not tested here.
	_, err := lexer.Tokenize("-foo")
	if err == nil {
		t.Error("expected error for hyphen-starting non-number '-foo', got nil")
	}
}

// TestSpecS8_7_BackslashRejectedInUnquoted verifies that a backslash in an
// unquoted string position is rejected (no escape decoding in unquoted strings).
// Spec L253. Status: ✅
func TestSpecS8_7_BackslashRejectedInUnquoted(t *testing.T) {
	// tokenize("a\n") — the backslash terminates the unquoted run via
	// isUnquotedForbidden, then the lexer hits the bare '\' and emits an error.
	_, err := lexer.Tokenize(`a\n`)
	if err == nil {
		t.Error("expected error for backslash in unquoted string, got nil")
	}
}

// TestSpecS6_3_BOMMidstreamIsWhitespace verifies that BOM (U+FEFF) appearing
// mid-stream acts as a whitespace separator rather than leaking into an unquoted
// string run or producing an error. Spec §Whitespace (L173): BOM must be treated
// as whitespace anywhere, not only at start-of-input.
// Status: ✅ fixed in fix/s6-whitespace-expansion
func TestSpecS6_3_BOMMidstreamIsWhitespace(t *testing.T) {
	src := "a\uFEFFb"
	toks := tokenize(src)
	var strings_ []string
	for _, tok := range toks {
		if tok.Type == lexer.TokenError || tok.Type == lexer.TokenInvalid {
			t.Errorf("BOM mid-stream: got unexpected error token: %q", tok.Value)
		}
		if tok.Type == lexer.TokenString {
			strings_ = append(strings_, tok.Value)
		}
	}
	if len(strings_) != 2 || strings_[0] != "a" || strings_[1] != "b" {
		t.Errorf("BOM mid-stream: got string tokens %v, want [a b]", strings_)
	}
}

// TestSpecS6_LFStillEmitsNewline is a regression guard verifying that after
// predicate centralization, LF (U+000A) still emits a TokenNewline token and
// is NOT silently consumed as inter-token whitespace. Spec L183.
func TestSpecS6_LFStillEmitsNewline(t *testing.T) {
	toks := tokenize("a\nb")
	var sawNewline bool
	for _, tok := range toks {
		if tok.Type == lexer.TokenNewline {
			sawNewline = true
			break
		}
	}
	if !sawNewline {
		t.Fatal("expected TokenNewline for LF but none found")
	}
}

// TestSpecS8_8_ControlCharsAllowedInUnquoted verifies that control characters
// not in the forbidden set (e.g. SOH 0x01, BEL 0x07) are allowed inside
// unquoted strings. Spec L280. Status: ✅
func TestSpecS8_8_ControlCharsAllowedInUnquoted(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"SOH (0x01)", "foo\x01bar", "foo\x01bar"},
		{"BEL (0x07)", "foo\x07bar", "foo\x07bar"},
	}
	for _, tc := range cases {
		toks := tokenize(tc.src)
		if len(toks) == 0 {
			t.Fatalf("%s: no tokens", tc.name)
		}
		tok := toks[0]
		if tok.Type != lexer.TokenString {
			t.Errorf("%s: type=%v, want TokenString", tc.name, tok.Type)
		}
		if tok.Value != tc.want {
			t.Errorf("%s: value=%q, want %q", tc.name, tok.Value, tc.want)
		}
	}
}

// TestSpecS6_SubstBodyNBSPBeforeDot verifies that NBSP (U+00A0) inside ${...}
// before a dot acts as inter-segment whitespace (discarded) rather than being
// absorbed into the segment text. Spec §D: all three whitespace sites must route
// through isHoconWhitespace. Status: RED — isUnquotedSubstChar uses hardcoded set.
func TestSpecS6_SubstBodyNBSPBeforeDot(t *testing.T) {
	nbsp := string(rune(0x00A0))
	input := "${foo" + nbsp + ".bar}"
	segs := substSegments(t, input)
	if len(segs) != 2 {
		t.Fatalf("NBSP before dot: got %d segments, want 2: %v", len(segs), segs)
	}
	if segs[0].Text != "foo" {
		t.Errorf("NBSP before dot: seg[0].Text=%q, want %q", segs[0].Text, "foo")
	}
	if segs[1].Text != "bar" {
		t.Errorf("NBSP before dot: seg[1].Text=%q, want %q", segs[1].Text, "bar")
	}
}

// TestSpecS6_SubstBodyZlBeforeDot verifies that line separator (U+2028, Zl)
// inside ${...} before a dot acts as inter-segment whitespace (discarded) rather
// than being absorbed into the segment text. Status: RED — isUnquotedSubstChar
// uses hardcoded set.
func TestSpecS6_SubstBodyZlBeforeDot(t *testing.T) {
	zl := string(rune(0x2028))
	input := "${foo" + zl + ".bar}"
	segs := substSegments(t, input)
	if len(segs) != 2 {
		t.Fatalf("Zl before dot: got %d segments, want 2: %v", len(segs), segs)
	}
	if segs[0].Text != "foo" {
		t.Errorf("Zl before dot: seg[0].Text=%q, want %q", segs[0].Text, "foo")
	}
	if segs[1].Text != "bar" {
		t.Errorf("Zl before dot: seg[1].Text=%q, want %q", segs[1].Text, "bar")
	}
}

// TestSpecS6_SubstBodyVtabBeforeDot verifies that vertical tab (U+000B) inside
// ${...} before a dot acts as inter-segment whitespace (discarded) rather than
// being absorbed into the segment text. Status: RED — isUnquotedSubstChar uses
// hardcoded set.
func TestSpecS6_SubstBodyVtabBeforeDot(t *testing.T) {
	input := "${foo\x0b.bar}"
	segs := substSegments(t, input)
	if len(segs) != 2 {
		t.Fatalf("vtab before dot: got %d segments, want 2: %v", len(segs), segs)
	}
	if segs[0].Text != "foo" {
		t.Errorf("vtab before dot: seg[0].Text=%q, want %q", segs[0].Text, "foo")
	}
	if segs[1].Text != "bar" {
		t.Errorf("vtab before dot: seg[1].Text=%q, want %q", segs[1].Text, "bar")
	}
}

// TestSpecS6_SubstBodyBOMBeforeDot verifies that BOM (U+FEFF) inside ${...}
// before a dot acts as inter-segment whitespace (discarded) rather than being
// absorbed into the segment text. Status: RED — isUnquotedSubstChar uses hardcoded
// set.
func TestSpecS6_SubstBodyBOMBeforeDot(t *testing.T) {
	bom := string(rune(0xFEFF))
	input := "${foo" + bom + ".bar}"
	segs := substSegments(t, input)
	if len(segs) != 2 {
		t.Fatalf("BOM before dot: got %d segments, want 2: %v", len(segs), segs)
	}
	if segs[0].Text != "foo" {
		t.Errorf("BOM before dot: seg[0].Text=%q, want %q", segs[0].Text, "foo")
	}
	if segs[1].Text != "bar" {
		t.Errorf("BOM before dot: seg[1].Text=%q, want %q", segs[1].Text, "bar")
	}
}

// TestSpecS6_CR_InsideSubstBody pins the behavior that CR (U+000D) inside
// ${...} is consumed as inter-segment whitespace, not as a newline that would
// terminate the substitution with an error. CR satisfies isHoconWhitespace but
// NOT isHoconNewline, so it is handled by the non-newline whitespace case.
//
// Effective behavior: ${foo\rbar} produces one segment with text "foo\rbar"
// (CR is accumulated as pending whitespace and concatenated into the segment).
// 3-way convergent with ts.hocon and rs.hocon per spec §F.
//
// This test also pins that the refactor removing the dead `ch == '\r'` arm
// from the isHoconNewline case (commit 3) did not regress CR handling.
func TestSpecS6_CR_InsideSubstBody(t *testing.T) {
	// ${foo\rbar}: no dot, CR is whitespace, produces one segment "foo\rbar".
	segs := substSegments(t, "${foo\rbar}")
	if len(segs) != 1 {
		t.Fatalf("CR inside subst: got %d segments, want 1: %v", len(segs), segs)
	}
	want := "foo\rbar"
	if segs[0].Text != want {
		t.Errorf("CR inside subst: seg[0].Text=%q, want %q", segs[0].Text, want)
	}
}
