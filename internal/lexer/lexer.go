// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package lexer

import (
	"fmt"
	"strconv"
	"strings"
)

// TokenType identifies the type of a lexed token.
type TokenType int

const (
	TokenInvalid      TokenType = iota // zero value sentinel
	TokenString                        // quoted, unquoted, or triple-quoted string
	TokenInt                           // integer literal
	TokenFloat                         // float literal (has . or e/E)
	TokenBool                          // true / false
	TokenNull                          // null
	TokenLBrace                        // {
	TokenRBrace                        // }
	TokenLBracket                      // [
	TokenRBracket                      // ]
	TokenLParen                        // (
	TokenRParen                        // )
	TokenComma                         // ,
	TokenColon                         // :
	TokenEquals                        // =
	TokenPlusEquals                    // +=
	TokenSubstitution                  // ${path} or ${?path} — check tok.Subst.Optional for optional
	TokenInclude                       // include keyword
	TokenNewline                       // \n
	TokenEOF
	TokenError // lexer error (e.g. unterminated string)
)

// Segment is a single path segment inside a substitution body, with its
// source position (1-based line and column of the opening character).
type Segment struct {
	Text string
	Line int
	Col  int
}

// SubstPayload carries the parsed segments and optional flag for a substitution token.
type SubstPayload struct {
	Segments []Segment
	Optional bool
}

// Token is a single lexed unit.
type Token struct {
	Type           TokenType
	Value          string
	Line           int
	Col            int
	IsQuoted       bool          // true for quoted strings (single or triple-quoted)
	PrecedingSpace bool          // true if whitespace preceded this token (for concatenation)
	Subst          *SubstPayload // non-nil only when Type == TokenSubstitution
}

// Lexer tokenizes HOCON input.
type Lexer struct {
	src          []rune
	pos          int
	line         int
	col          int
	skippedSpace bool // set by skipWhitespaceAndComments
}

// Tokenize lexes the entire input and returns all tokens up to (and including)
// EOF, or an error if a TokenError is encountered. Convenience wrapper over New.
func Tokenize(src string) ([]Token, error) {
	l := New(src)
	var tokens []Token
	for {
		tok := l.Next()
		if tok.Type == TokenError {
			return nil, fmt.Errorf("lex error at line %d col %d: %s", tok.Line, tok.Col, tok.Value)
		}
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens, nil
}

// New returns a Lexer for the given input.
// A leading UTF-8 BOM (U+FEFF) is silently stripped.
func New(src string) *Lexer {
	runes := []rune(src)
	if len(runes) > 0 && runes[0] == '\uFEFF' {
		runes = runes[1:]
	}
	return &Lexer{src: runes, pos: 0, line: 1, col: 1}
}

func (l *Lexer) peek() (rune, bool) {
	if l.pos >= len(l.src) {
		return 0, false
	}
	return l.src[l.pos], true
}

func (l *Lexer) advance() rune {
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

// Next returns the next token.
func (l *Lexer) Next() Token {
	l.skipWhitespaceAndComments()
	hadSpace := l.skippedSpace

	tok := l.nextToken()
	tok.PrecedingSpace = hadSpace
	return tok
}

func (l *Lexer) nextToken() Token {
	line, col := l.line, l.col

	ch, ok := l.peek()
	if !ok {
		return Token{Type: TokenEOF, Line: line, Col: col}
	}

	switch {
	case ch == '\n':
		l.advance()
		return Token{Type: TokenNewline, Line: line, Col: col}

	case ch == '{':
		l.advance()
		return Token{Type: TokenLBrace, Value: "{", Line: line, Col: col}
	case ch == '}':
		l.advance()
		return Token{Type: TokenRBrace, Value: "}", Line: line, Col: col}
	case ch == '[':
		l.advance()
		return Token{Type: TokenLBracket, Value: "[", Line: line, Col: col}
	case ch == ']':
		l.advance()
		return Token{Type: TokenRBracket, Value: "]", Line: line, Col: col}
	case ch == '(':
		l.advance()
		return Token{Type: TokenLParen, Value: "(", Line: line, Col: col}
	case ch == ')':
		l.advance()
		return Token{Type: TokenRParen, Value: ")", Line: line, Col: col}
	case ch == ',':
		l.advance()
		return Token{Type: TokenComma, Value: ",", Line: line, Col: col}
	case ch == ':':
		l.advance()
		return Token{Type: TokenColon, Value: ":", Line: line, Col: col}
	case ch == '=':
		l.advance()
		return Token{Type: TokenEquals, Value: "=", Line: line, Col: col}
	case ch == '+':
		l.advance()
		if next, ok2 := l.peek(); ok2 && next == '=' {
			l.advance()
			return Token{Type: TokenPlusEquals, Value: "+=", Line: line, Col: col}
		}
		// bare '+' — treated as start of unquoted string
		return l.readUnquoted("+", line, col)

	case ch == '$':
		return l.readSubstitution(line, col)

	case ch == '"':
		return l.readString(line, col)

	case ch == '-' || (ch >= '0' && ch <= '9'):
		return l.readNumber(line, col)

	default:
		return l.readUnquotedOrKeyword(line, col)
	}
}

func (l *Lexer) skipWhitespaceAndComments() {
	l.skippedSpace = false
	for {
		ch, ok := l.peek()
		if !ok {
			return
		}
		if isHoconNewline(ch) {
			return // newlines are significant tokens; emitted by Next() caller
		}
		if isHoconWhitespace(ch) {
			l.skippedSpace = true
			l.advance()
			continue
		}
		if ch == '#' {
			for {
				c, ok2 := l.peek()
				if !ok2 || c == '\n' {
					break
				}
				l.advance()
			}
			continue
		}
		if ch == '/' {
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
				for {
					c, ok2 := l.peek()
					if !ok2 || c == '\n' {
						break
					}
					l.advance()
				}
				continue
			}
		}
		return
	}
}

// readQuotedStringBody reads the body of a quoted string. The opening '"' has
// already been consumed. It reads until the closing '"' and returns the decoded
// text. On error it returns a non-nil error token. startLine/startCol are the
// position of the opening '"' (for error reporting).
//
// This is a shared helper used by both top-level readString and parseSubstBody.
func (l *Lexer) readQuotedStringBody(startLine, startCol int) (string, *Token) {
	var sb strings.Builder
	for {
		ch, ok := l.peek()
		if !ok || ch == '\n' {
			errTok := Token{Type: TokenError, Value: "unterminated string", Line: startLine, Col: startCol}
			return "", &errTok
		}
		if ch == '"' {
			l.advance() // consume closing '"'
			return sb.String(), nil
		}
		if ch == '\\' {
			l.advance()         // consume '\'
			escCol := l.col - 1 // column of the backslash
			next, ok2 := l.peek()
			if !ok2 {
				errTok := Token{Type: TokenError, Value: "unterminated string", Line: startLine, Col: startCol}
				return "", &errTok
			}
			l.advance()
			switch next {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case '/':
				sb.WriteByte('/')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'u':
				hex := make([]rune, 0, 4)
				for i := 0; i < 4; i++ {
					hch, hok := l.peek()
					if !hok {
						errTok := Token{Type: TokenError, Value: "invalid unicode escape: unexpected end", Line: startLine, Col: startCol}
						return "", &errTok
					}
					if !isHexDigit(hch) {
						errTok := Token{Type: TokenError, Value: fmt.Sprintf("invalid unicode escape: non-hex char '%c'", hch), Line: startLine, Col: startCol}
						return "", &errTok
					}
					hex = append(hex, l.advance())
				}
				codePoint, _ := strconv.ParseInt(string(hex), 16, 32)
				r := rune(codePoint)
				// Reject surrogate codepoints — they are not valid Unicode scalar values.
				// Go's rune / string is UTF-8; surrogates cannot be encoded. Match rs.hocon behavior.
				if r >= 0xD800 && r <= 0xDFFF {
					errTok := Token{Type: TokenError, Value: "invalid unicode escape: surrogate codepoint", Line: startLine, Col: startCol}
					return "", &errTok
				}
				sb.WriteRune(r)
			default:
				errTok := Token{Type: TokenError, Value: fmt.Sprintf("invalid escape sequence: \\%c", next), Line: startLine, Col: escCol}
				return "", &errTok
			}
			continue
		}
		l.advance()
		sb.WriteRune(ch)
	}
}

func (l *Lexer) readString(line, col int) Token {
	l.advance() // consume first "
	// check for triple-quote
	if l.pos+1 < len(l.src) && l.src[l.pos] == '"' && l.src[l.pos+1] == '"' {
		l.advance() // second "
		l.advance() // third "
		return l.readTripleQuoted(line, col)
	}
	// regular quoted string — use shared body reader
	body, errTok := l.readQuotedStringBody(line, col)
	if errTok != nil {
		return *errTok
	}
	return Token{Type: TokenString, Value: body, Line: line, Col: col, IsQuoted: true}
}

func (l *Lexer) readTripleQuoted(line, col int) Token {
	var sb strings.Builder
	closed := false
	for {
		ch, ok := l.peek()
		if !ok {
			break
		}
		if ch == '"' {
			// Count consecutive quotes
			quoteCount := 0
			startPos := l.pos
			for l.pos < len(l.src) && l.src[l.pos] == '"' {
				quoteCount++
				l.pos++
			}
			// Update line/col tracking
			l.col += quoteCount

			if quoteCount >= 3 {
				// The last 3 quotes are the closing delimiter;
				// any extras are content.
				extra := quoteCount - 3
				for i := 0; i < extra; i++ {
					sb.WriteByte('"')
				}
				closed = true
				break
			}
			// Fewer than 3 quotes — they are content
			_ = startPos
			for i := 0; i < quoteCount; i++ {
				sb.WriteByte('"')
			}
			continue
		}
		// normalize \r\n and standalone \r to \n
		if ch == '\r' {
			l.advance()
			// if followed by \n, skip the \r — the \n will be written next iteration
			if next, ok2 := l.peek(); ok2 && next == '\n' {
				continue
			}
			sb.WriteByte('\n')
			continue
		}
		// handle newline tracking
		if ch == '\n' {
			l.advance()
			sb.WriteByte('\n')
			continue
		}
		l.advance()
		sb.WriteRune(ch)
	}
	if !closed {
		return Token{Type: TokenError, Value: "unterminated triple-quoted string", Line: line, Col: col}
	}
	return Token{Type: TokenString, Value: sb.String(), Line: line, Col: col, IsQuoted: true}
}

// isUnquotedSubstChar returns true if ch is allowed inside a ${...} body
// as an unquoted character. Mirrors rs.hocon's is_unquoted_subst_char.
//
// Whitespace is delegated to isHoconWhitespace so that all three
// whitespace-check sites in the subst-body path machine route through the
// same predicate (the main loop, parseSubstBody skip, and this function).
func isUnquotedSubstChar(ch rune) bool {
	if isHoconWhitespace(ch) {
		return false
	}
	switch ch {
	case '"', '\\',
		'{', '}', '[', ']',
		':', '=', ',', '+', '#',
		'`', '^', '?', '!', '@', '*', '&',
		'$', '.':
		return false
	}
	return true
}

// parseSubstBody implements the Appendix A state machine for ${...} body tokenization.
// Called after '$' and '{' have both been consumed.
// startLine/startCol are the position of the '$' (for error reporting).
func (l *Lexer) parseSubstBody(startLine, startCol int) Token {
	// START: optional sigil
	optional := false
	if ch, ok := l.peek(); ok && ch == '?' {
		l.advance()
		optional = true
	}

	// COLLECT state
	var curText strings.Builder
	curStarted := false
	var curLine, curCol int

	pendingWs := ""
	var segments []Segment
	lastDotLine, lastDotCol := 0, 0
	hasLastDot := false

	for {
		ch, ok := l.peek()
		if !ok {
			// EOF — unterminated
			return Token{Type: TokenError, Value: "unterminated substitution", Line: startLine, Col: startCol}
		}

		switch {
		case ch == '}':
			l.advance()
			// trailing WS is discarded
			goto done

		case ch == '"':
			qLine := startLine // substitutions cannot span newlines
			qCol := l.col
			if curStarted {
				curText.WriteString(pendingWs)
			}
			pendingWs = ""
			l.advance() // consume opening '"'
			body, errTok := l.readQuotedStringBody(qLine, qCol)
			if errTok != nil {
				return *errTok
			}
			curText.WriteString(body)
			if !curStarted {
				curLine = qLine
				curCol = qCol
				curStarted = true
			}

		case isUnquotedSubstChar(ch):
			// S8.6 (HOCON.md L270–276) also applies to unquoted path segments
			// inside ${...}: a segment beginning with '-' must be followed by a
			// digit. Gate on `!curStarted` so the check fires only at segment
			// start — a `-` that follows a quoted fragment in the same segment
			// (e.g. ${"a"-foo} resolving the key "a-foo" via quoted/unquoted
			// concat) is not policed, mirroring how the existing ${"a"x} flow
			// builds "ax". Mirrors ts.hocon PR #97 and rs.hocon PR #86.
			if ch == '-' && !curStarted {
				next, _ := func() (rune, bool) {
					if l.pos+1 >= len(l.src) {
						return 0, false
					}
					return l.src[l.pos+1], true
				}()
				if next < '0' || next > '9' {
					after := "EOF"
					if next != 0 {
						after = fmt.Sprintf("%q", next)
					}
					return Token{
						Type:  TokenError,
						Value: fmt.Sprintf("unquoted path segment cannot begin with '-' unless followed by a digit (got '-' then %s, HOCON.md L270-276)", after),
						Line:  startLine,
						Col:   l.col,
					}
				}
			}
			uCol := l.col
			if curStarted {
				curText.WriteString(pendingWs)
			}
			pendingWs = ""
			if !curStarted {
				curLine = startLine // always same line as ${ (no newlines inside subst)
				curCol = uCol
				curStarted = true
			}
			// Read the unquoted run
			for {
				c, ok2 := l.peek()
				if !ok2 || !isUnquotedSubstChar(c) {
					break
				}
				curText.WriteRune(l.advance())
			}

		case ch == '.':
			dotCol := l.col
			pendingWs = ""
			if !curStarted {
				return Token{Type: TokenError, Value: "empty segment in path", Line: startLine, Col: dotCol}
			}
			segments = append(segments, Segment{Text: curText.String(), Line: curLine, Col: curCol})
			curText.Reset()
			curStarted = false
			curLine = 0
			curCol = 0
			lastDotLine = startLine
			lastDotCol = dotCol
			hasLastDot = true
			l.advance()

		case isHoconWhitespace(ch) && !isHoconNewline(ch):
			// Non-newline whitespace inside ${...} is accumulated as pending
			// inter-segment whitespace. col advances; line is unchanged.
			pendingWs += string(ch)
			l.advance()

		case isHoconNewline(ch):
			// LF terminates a substitution (unterminated).
			//
			// History: before fix/s6-whitespace-expansion, the subst-body whitespace
			// case matched only ' ' and '\t', so CR (U+000D) fell through to a dedicated
			// `case ch == '\n' || ch == '\r'` arm and was rejected as "unterminated
			// substitution". After the fix, the whitespace case matches all of
			// isHoconWhitespace && !isHoconNewline, which includes CR. CR is now
			// consumed there before this case is reached, making the old explicit CR
			// arm dead code — which is why it was removed.
			return Token{Type: TokenError, Value: "unterminated substitution", Line: startLine, Col: startCol}

		default:
			return Token{Type: TokenError, Value: fmt.Sprintf("unexpected character in substitution path: %c", ch), Line: startLine, Col: l.col}
		}
	}

done:
	if curStarted {
		segments = append(segments, Segment{Text: curText.String(), Line: curLine, Col: curCol})
	} else if len(segments) == 0 {
		// ${}
		return Token{Type: TokenError, Value: "empty substitution path", Line: startLine, Col: startCol}
	} else {
		// trailing dot: ${foo.}
		errLine, errCol := startLine, startCol
		if hasLastDot {
			errLine, errCol = lastDotLine, lastDotCol
		}
		return Token{Type: TokenError, Value: "empty segment in path", Line: errLine, Col: errCol}
	}

	// Build the Value string from segments (dot-joined, for backward compat with resolver)
	// The resolver now reads Subst.Segments directly, but Value is kept for debugging.
	parts := make([]string, len(segments))
	for i, s := range segments {
		parts[i] = s.Text
	}

	return Token{
		Type:  TokenSubstitution,
		Value: strings.Join(parts, "."),
		Line:  startLine,
		Col:   startCol,
		Subst: &SubstPayload{Segments: segments, Optional: optional},
	}
}

func (l *Lexer) readSubstitution(line, col int) Token {
	l.advance() // $
	ch, ok := l.peek()
	if !ok || ch != '{' {
		return Token{Type: TokenInvalid, Line: line, Col: col}
	}
	l.advance() // {

	return l.parseSubstBody(line, col)
}

// readNumber lexes a number per the HOCON.md §Number grammar (which mirrors
// JSON's number grammar): `int frac? exp?` with optional leading `-`. The
// implementation uses **greedy-with-backtrack**: the fractional and exponent
// productions each independently backtrack to the last valid number end if
// the production cannot be fully consumed (e.g. `1.x` returns number `1` and
// leaves `.x` for the next-token pass; `1ex` returns number `1` and leaves
// `ex`). Per HOCON.md L270-276, a leading `-` MUST be followed by a digit;
// if not, the lexer returns a TokenError rather than falling back to an
// unquoted string (which would silently coerce `-foo` to "-foo" — the spec
// non-compliance this PR closes for cluster 3c). See docs/spec-compliance.md
// §S8.6 for the rationale and the remaining gaps (us13, us15).
func (l *Lexer) readNumber(line, col int) Token {
	startPos := l.pos
	startCol := col

	// Optional leading '-'
	if ch, _ := l.peek(); ch == '-' {
		l.advance()
	}

	// Integer part — REQUIRED. Per JSON number grammar:
	// int = '0' | [1-9][0-9]*
	// (We accept '0[0-9]*' to match Lightbend behavior; the spec says JSON
	// numbers reject leading-zero forms like "01", but Lightbend's parser
	// silently accepts them and downstream callers expect this. The strict
	// us13 case `01` is documented as a known gap under #60.)
	if d, ok := l.peek(); ok && d >= '0' && d <= '9' {
		for {
			c, ok2 := l.peek()
			if !ok2 || c < '0' || c > '9' {
				break
			}
			l.advance()
		}
	} else {
		// No integer digits consumed. Caller dispatch invariant (lexer.go
		// ~L188-189) routes readNumber only when the leading char is `-` or a
		// digit; the digit case was handled above, so we must have consumed
		// `-` here. Per HOCON.md L270-276 this is a lex error.
		next := "EOF"
		if c, ok := l.peek(); ok {
			next = fmt.Sprintf("%q", c)
		}
		return Token{
			Type:  TokenError,
			Value: fmt.Sprintf("unquoted string cannot begin with '-' unless followed by a digit (got '-' then %s, HOCON.md L270-276)", next),
			Line:  line,
			Col:   startCol,
		}
	}

	lastValidEnd := l.pos
	lastValidCol := l.col
	hasDot := false
	hasExp := false

	// Try fractional part (greedy with backtrack).
	if ch, ok := l.peek(); ok && ch == '.' {
		savePos := l.pos
		saveCol := l.col
		l.advance() // consume '.'
		if d, ok2 := l.peek(); ok2 && d >= '0' && d <= '9' {
			for {
				c, ok3 := l.peek()
				if !ok3 || c < '0' || c > '9' {
					break
				}
				l.advance()
			}
			lastValidEnd = l.pos
			lastValidCol = l.col
			hasDot = true
		} else {
			// Backtrack: '.' not followed by digit, leave '.' for next token.
			l.pos = savePos
			l.col = saveCol
		}
	}

	// Try exponent part (greedy with backtrack).
	if ch, ok := l.peek(); ok && (ch == 'e' || ch == 'E') {
		savePos := l.pos
		saveCol := l.col
		l.advance() // consume 'e'/'E'
		if s, ok2 := l.peek(); ok2 && (s == '+' || s == '-') {
			l.advance()
		}
		if d, ok3 := l.peek(); ok3 && d >= '0' && d <= '9' {
			for {
				c, ok4 := l.peek()
				if !ok4 || c < '0' || c > '9' {
					break
				}
				l.advance()
			}
			lastValidEnd = l.pos
			lastValidCol = l.col
			hasExp = true
		} else {
			// Backtrack: 'e' not followed by digit (with optional sign), leave for next token.
			l.pos = savePos
			l.col = saveCol
		}
	}

	// Position should already be at lastValidEnd (last successful consume), but
	// restore explicitly in case the exponent backtrack left us short.
	l.pos = lastValidEnd
	l.col = lastValidCol

	tt := TokenInt
	if hasDot || hasExp {
		tt = TokenFloat
	}
	return Token{Type: tt, Value: string(l.src[startPos:lastValidEnd]), Line: line, Col: startCol}
}

// isHoconWhitespace reports whether r is a HOCON whitespace character per
// HOCON.md §Whitespace (L165-184). The set is:
//
//	ASCII control whitespace: HT VT FF CR FS GS RS US (0x09, 0x0B-0x0D, 0x1C-0x1F)
//	Unicode Zs category:      SP NBSP and all other Zs members
//	Unicode Zl (line sep):    U+2028
//	Unicode Zp (para sep):    U+2029
//	BOM:                      U+FEFF
//
// Note: LF (0x0A) is also in HOCON_WS but is the ONLY newline character and
// must be handled before this predicate in the main lexer loop. See isHoconNewline.
//
// Note: Go's unicode.IsSpace includes U+0085 (NEL) which HOCON does not, and
// excludes U+001C-U+001F which HOCON does include. Do not substitute IsSpace here.
func isHoconWhitespace(r rune) bool {
	switch {
	case r == '\t', r == '\n', r == '\v', r == '\f', r == '\r':
		return true
	case r >= 0x1C && r <= 0x1F:
		return true
	case r == ' ', r == 0xA0, r == 0xFEFF:
		return true
	case r == 0x1680:
		return true
	case r >= 0x2000 && r <= 0x200A:
		return true
	case r == 0x2028, r == 0x2029, r == 0x202F, r == 0x205F:
		return true
	case r == 0x3000:
		return true
	}
	return false
}

// isHoconNewline reports whether r is the HOCON newline character.
// Per HOCON.md L182-184, "newline" means exclusively ASCII LF (0x000A).
// Unicode line separator (U+2028) and paragraph separator (U+2029) are
// whitespace but NOT newlines in HOCON.
func isHoconNewline(r rune) bool { return r == '\n' }

// unquotedForbidden are characters that terminate an unquoted string.
// Per spec: $"{}[]:=,+#\^?!@*& plus all whitespace.
// Parentheses are not in the spec but are included so that
// `include file(...)` / `include required(...)` can be parsed correctly.
const unquotedForbidden = `$"{}[]:=,+#\^?!@*&()`

func isUnquotedForbidden(ch rune) bool {
	return isHoconWhitespace(ch) || strings.ContainsRune(unquotedForbidden, ch)
}

func isHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func (l *Lexer) readUnquoted(prefix string, line, col int) Token {
	var sb strings.Builder
	sb.WriteString(prefix)
	for {
		ch, ok := l.peek()
		if !ok || isUnquotedForbidden(ch) {
			break
		}
		sb.WriteRune(l.advance())
	}
	return Token{Type: TokenString, Value: sb.String(), Line: line, Col: col}
}

func (l *Lexer) readUnquotedOrKeyword(line, col int) Token {
	tok := l.readUnquoted("", line, col)
	if tok.Value == "" {
		// The character is forbidden in unquoted strings and has no
		// dedicated token type (e.g. *, !, @, ^, ?).  Consume it and
		// emit an error so the lexer makes progress.
		ch := l.advance()
		return Token{Type: TokenError, Value: fmt.Sprintf("unexpected character: %c", ch), Line: line, Col: col}
	}
	switch tok.Value {
	case "true", "false":
		return Token{Type: TokenBool, Value: tok.Value, Line: line, Col: col}
	case "null":
		return Token{Type: TokenNull, Value: tok.Value, Line: line, Col: col}
	case "include":
		return Token{Type: TokenInclude, Value: tok.Value, Line: line, Col: col}
	}
	return tok
}
