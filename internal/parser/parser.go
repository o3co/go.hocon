// Copyright 2026 o3co Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/o3co/go.hocon/internal/lexer"
)

// Parse parses a HOCON string and returns the root ObjectNode.
// The input may omit outer braces (root object shorthand).
func Parse(src string) (*ObjectNode, error) {
	p := &parser{lex: lexer.New(src)}
	p.advance()
	return p.parseRoot()
}

// ParseBytes is like Parse but accepts a byte slice.
func ParseBytes(src []byte) (*ObjectNode, error) {
	return Parse(string(src))
}

type parser struct {
	lex     *lexer.Lexer
	current lexer.Token
}

func (p *parser) advance() {
	p.current = p.lex.Next()
}

func (p *parser) skipNewlines() {
	for p.current.Type == lexer.TokenNewline {
		p.advance()
	}
}

func (p *parser) parseRoot() (*ObjectNode, error) {
	p.skipNewlines()
	// root may be a bare object (no braces) or an explicit { ... }
	if p.current.Type == lexer.TokenLBrace {
		return p.parseObject()
	}
	return p.parseObjectFields(false)
}

func (p *parser) parseObject() (*ObjectNode, error) {
	line, col := p.current.Line, p.current.Col
	p.advance() // consume {
	obj, err := p.parseObjectFields(true)
	if err != nil {
		return nil, err
	}
	obj.line, obj.col = line, col
	return obj, nil
}

func (p *parser) parseObjectFields(braced bool) (*ObjectNode, error) {
	obj := &ObjectNode{}
	for {
		p.skipNewlines()
		if braced && p.current.Type == lexer.TokenRBrace {
			p.advance()
			break
		}
		if p.current.Type == lexer.TokenEOF {
			if braced {
				return nil, fmt.Errorf("parse error at line %d, col %d: unexpected EOF, expected '}'", p.current.Line, p.current.Col)
			}
			break
		}
		// include directive
		if p.current.Type == lexer.TokenInclude {
			inc, err := p.parseInclude()
			if err != nil {
				return nil, err
			}
			// store include as a synthetic field with empty key
			obj.Fields = append(obj.Fields, FieldNode{
				pos:   pos{inc.line, inc.col},
				Key:   nil,
				Value: inc,
			})
			p.skipSeparator()
			continue
		}
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}
		obj.Fields = append(obj.Fields, *field)
		p.skipSeparator()
	}
	return obj, nil
}

// skipSeparator consumes an optional comma or newline separator.
func (p *parser) skipSeparator() {
	if p.current.Type == lexer.TokenComma {
		p.advance()
	}
	p.skipNewlines()
}

func (p *parser) parseInclude() (*IncludeNode, error) {
	line, col := p.current.Line, p.current.Col
	p.advance() // consume "include"
	p.skipNewlines()
	// check for unsupported forms: url(...) classpath(...)
	if p.current.Type == lexer.TokenString {
		switch p.current.Value {
		case "url", "classpath":
			return nil, fmt.Errorf("parse error at line %d, col %d: include %s(...) is not supported in v1.0", line, col, p.current.Value)
		}
	}
	// support: include "file.conf" and include file("file.conf")
	if p.current.Type == lexer.TokenString && p.current.Value == "file" {
		p.advance() // consume "file"
		// expect '('
		if p.current.Type != lexer.TokenLParen {
			return nil, fmt.Errorf("parse error at line %d, col %d: expected '(' after 'file' in include directive", line, col)
		}
		p.advance() // consume '('
		if p.current.Type != lexer.TokenString {
			return nil, fmt.Errorf("parse error at line %d, col %d: expected filename string in include file(...)", line, col)
		}
		path := p.current.Value
		p.advance() // consume path
		if p.current.Type != lexer.TokenRParen {
			return nil, fmt.Errorf("parse error at line %d, col %d: expected ')' after filename in include file(...)", line, col)
		}
		p.advance() // consume ')'
		return &IncludeNode{pos: pos{line, col}, Path: path}, nil
	}
	if p.current.Type != lexer.TokenString {
		return nil, fmt.Errorf("parse error at line %d, col %d: expected filename after include", line, col)
	}
	path := p.current.Value
	p.advance()
	return &IncludeNode{pos: pos{line, col}, Path: path}, nil
}

func (p *parser) parseField() (*FieldNode, error) {
	line, col := p.current.Line, p.current.Col
	// parse key (dot-separated path, possibly multi-segment with quoted parts)
	key, err := p.parseKey()
	if err != nil {
		return nil, err
	}
	// HOCON allows newlines between key and separator
	p.skipNewlines()
	// parse separator: : = or {
	append_ := false
	switch p.current.Type {
	case lexer.TokenColon, lexer.TokenEquals:
		p.advance()
	case lexer.TokenPlusEquals:
		append_ = true
		p.advance()
	case lexer.TokenLBrace:
		// key { ... } shorthand — value is an object
	default:
		return nil, fmt.Errorf("parse error at line %d, col %d: expected ':', '=' or '{' after key", p.current.Line, p.current.Col)
	}
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &FieldNode{pos: pos{line, col}, Key: key, Value: val, Append: append_}, nil
}

func (p *parser) parseKey() ([]string, error) {
	if p.current.Type != lexer.TokenString && p.current.Type != lexer.TokenInt {
		return nil, fmt.Errorf("parse error at line %d, col %d: expected key, got %v", p.current.Line, p.current.Col, p.current.Type)
	}

	var parts []string

	for {
		raw := p.current.Value
		isQuoted := p.current.IsQuoted
		p.advance()

		if isQuoted {
			// Quoted key segment — no dot splitting
			parts = append(parts, raw)
		} else {
			// Unquoted key — split on dots for path notation.
			// A trailing dot (e.g., "arrays.") means the next token continues the path.
			segments := strings.Split(raw, ".")
			for i, s := range segments {
				if s == "" {
					continue // skip empty segments from leading/trailing dots
				}
				parts = append(parts, s)
				_ = i
			}
			// If the raw value ends with '.', the next token is a continuation
			if strings.HasSuffix(raw, ".") {
				continue // read the next segment
			}
		}

		// Check if the next token is an unquoted string starting with '.'
		// or a quoted string preceded by a dot (like in "a"."b" patterns).
		// After a quoted segment, look if the next unquoted starts with '.'
		if p.current.Type == lexer.TokenString && !p.current.IsQuoted && strings.HasPrefix(p.current.Value, ".") {
			continue
		}
		break
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("parse error: empty key")
	}
	return parts, nil
}

func (p *parser) parseValue() (Node, error) {
	p.skipNewlines()
	first, err := p.parseSingleValue()
	if err != nil {
		return nil, err
	}
	// check for concatenation (adjacent values on same line)
	var nodes []Node
	nodes = append(nodes, first)
	for p.current.Type != lexer.TokenNewline &&
		p.current.Type != lexer.TokenEOF &&
		p.current.Type != lexer.TokenComma &&
		p.current.Type != lexer.TokenRBrace &&
		p.current.Type != lexer.TokenRBracket {
		// If there was whitespace between the previous value and this token,
		// insert a space node for proper concatenation.
		hadSpace := p.current.PrecedingSpace
		next, err2 := p.parseSingleValue()
		if err2 != nil {
			break
		}
		if hadSpace {
			nodes = append(nodes, &ScalarNode{Value: " "})
		}
		nodes = append(nodes, next)
	}
	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return &ConcatNode{Nodes: nodes}, nil
}

func (p *parser) parseSingleValue() (Node, error) {
	line, col := p.current.Line, p.current.Col
	switch p.current.Type {
	case lexer.TokenLBrace:
		return p.parseObject()
	case lexer.TokenLBracket:
		return p.parseArray()
	case lexer.TokenSubstitution:
		val := p.current.Value
		p.advance()
		return &SubstNode{pos: pos{line, col}, Path: val, Optional: false}, nil
	case lexer.TokenOptSubstitution:
		val := p.current.Value
		p.advance()
		return &SubstNode{pos: pos{line, col}, Path: val, Optional: true}, nil
	case lexer.TokenString:
		val := p.current.Value
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Value: val}, nil
	case lexer.TokenInt:
		raw := p.current.Value
		p.advance()
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse error at line %d: invalid int %q", line, raw)
		}
		return &ScalarNode{pos: pos{line, col}, Value: n}, nil
	case lexer.TokenFloat:
		raw := p.current.Value
		p.advance()
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("parse error at line %d: invalid float %q", line, raw)
		}
		return &ScalarNode{pos: pos{line, col}, Value: f}, nil
	case lexer.TokenBool:
		val := p.current.Value == "true"
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Value: val}, nil
	case lexer.TokenNull:
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Value: nil}, nil
	default:
		return nil, fmt.Errorf("parse error at line %d, col %d: unexpected token %v", line, col, p.current.Type)
	}
}

func (p *parser) parseArray() (*ArrayNode, error) {
	line, col := p.current.Line, p.current.Col
	p.advance() // consume [
	arr := &ArrayNode{pos: pos{line, col}}
	for {
		p.skipNewlines()
		if p.current.Type == lexer.TokenRBracket {
			p.advance()
			break
		}
		if p.current.Type == lexer.TokenEOF {
			return nil, fmt.Errorf("parse error at line %d: unexpected EOF in array", line)
		}
		elem, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr.Elements = append(arr.Elements, elem)
		p.skipSeparator()
	}
	return arr, nil
}
