// Copyright 2026 o3co Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package lexer

import (
	"strings"
	"unicode"
)

// TokenType identifies the type of a lexed token.
type TokenType int

const (
	TokenInvalid         TokenType = iota // zero value sentinel
	TokenString                           // quoted, unquoted, or triple-quoted string
	TokenInt                              // integer literal
	TokenFloat                            // float literal (has . or e/E)
	TokenBool                             // true / false
	TokenNull                             // null
	TokenLBrace                           // {
	TokenRBrace                           // }
	TokenLBracket                         // [
	TokenRBracket                         // ]
	TokenLParen                           // (
	TokenRParen                           // )
	TokenComma                            // ,
	TokenColon                            // :
	TokenEquals                           // =
	TokenPlusEquals                       // +=
	TokenSubstitution                     // ${path}
	TokenOptSubstitution                  // ${?path}
	TokenInclude                          // include keyword
	TokenNewline                          // \n
	TokenEOF
)

// Token is a single lexed unit.
type Token struct {
	Type           TokenType
	Value          string
	Line           int
	Col            int
	IsQuoted       bool // true for quoted strings (single or triple-quoted)
	PrecedingSpace bool // true if whitespace preceded this token (for concatenation)
}

// Lexer tokenizes HOCON input.
type Lexer struct {
	src          []rune
	pos          int
	line         int
	col          int
	skippedSpace bool // set by skipWhitespaceAndComments
}

// New returns a Lexer for the given input.
func New(src string) *Lexer {
	return &Lexer{src: []rune(src), pos: 0, line: 1, col: 1}
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
		if ch == '\n' {
			return // newlines are significant tokens
		}
		if ch == ' ' || ch == '\t' || ch == '\r' {
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

func (l *Lexer) readString(line, col int) Token {
	l.advance() // consume first "
	// check for triple-quote
	if l.pos+1 < len(l.src) && l.src[l.pos] == '"' && l.src[l.pos+1] == '"' {
		l.advance() // second "
		l.advance() // third "
		return l.readTripleQuoted(line, col)
	}
	// regular quoted string
	var sb strings.Builder
	for {
		ch, ok := l.peek()
		if !ok || ch == '\n' {
			break
		}
		l.advance()
		if ch == '"' {
			break
		}
		if ch == '\\' {
			next, ok2 := l.peek()
			if !ok2 {
				break
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
			default:
				sb.WriteRune('\\')
				sb.WriteRune(next)
			}
			continue
		}
		sb.WriteRune(ch)
	}
	return Token{Type: TokenString, Value: sb.String(), Line: line, Col: col, IsQuoted: true}
}

func (l *Lexer) readTripleQuoted(line, col int) Token {
	var sb strings.Builder
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
	return Token{Type: TokenString, Value: sb.String(), Line: line, Col: col, IsQuoted: true}
}

func (l *Lexer) readSubstitution(line, col int) Token {
	l.advance() // $
	ch, ok := l.peek()
	if !ok || ch != '{' {
		return Token{Type: TokenInvalid, Line: line, Col: col}
	}
	l.advance() // {
	optional := false
	if next, ok2 := l.peek(); ok2 && next == '?' {
		optional = true
		l.advance()
	}
	var sb strings.Builder
	for {
		ch2, ok2 := l.peek()
		if !ok2 || ch2 == '}' {
			if ok2 {
				l.advance()
			}
			break
		}
		l.advance()
		sb.WriteRune(ch2)
	}
	tt := TokenSubstitution
	if optional {
		tt = TokenOptSubstitution
	}
	return Token{Type: tt, Value: sb.String(), Line: line, Col: col}
}

func (l *Lexer) readNumber(line, col int) Token {
	var sb strings.Builder
	isFloat := false
	if ch, _ := l.peek(); ch == '-' {
		sb.WriteRune(l.advance())
	}
	for {
		ch, ok := l.peek()
		if !ok {
			break
		}
		if ch >= '0' && ch <= '9' {
			sb.WriteRune(l.advance())
		} else if (ch == '.' || ch == 'e' || ch == 'E') && !isFloat {
			isFloat = true
			sb.WriteRune(l.advance())
		} else if (ch == '+' || ch == '-') && sb.Len() > 0 {
			// exponent sign
			prev := rune(sb.String()[sb.Len()-1])
			if prev == 'e' || prev == 'E' {
				sb.WriteRune(l.advance())
			} else {
				break
			}
		} else {
			break
		}
	}
	tt := TokenInt
	if isFloat {
		tt = TokenFloat
	}
	return Token{Type: tt, Value: sb.String(), Line: line, Col: col}
}

// unquotedForbidden are characters that terminate an unquoted string.
// Parentheses are included so that `include file(...)` can be parsed correctly.
const unquotedForbidden = `$"{}[]:=,+#\^?!@&()`

func isUnquotedForbidden(ch rune) bool {
	return unicode.IsSpace(ch) || strings.ContainsRune(unquotedForbidden, ch)
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
