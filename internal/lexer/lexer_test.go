package lexer_test

import (
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
	types := tokenTypes("${?foo}")
	if types[0] != lexer.TokenOptSubstitution {
		t.Errorf("expected TokenOptSubstitution, got %v", types[0])
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

func TestLexer_LineCol(t *testing.T) {
	l := lexer.New("a\nb")
	tok := l.Next() // 'a' unquoted string
	if tok.Line != 1 || tok.Col != 1 {
		t.Errorf("a: line=%d col=%d, want 1,1", tok.Line, tok.Col)
	}
}
