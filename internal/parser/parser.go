// Copyright 2026 1o1 Co. Ltd.
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
	// S3.1: empty file is invalid (HOCON.md L130). After stripping newlines,
	// if only EOF remains the document has no semantic content — reject it.
	// This covers: empty string, whitespace-only, newlines-only, comment-only,
	// BOM-only, and mixed whitespace+comment inputs. The lexer strips BOM at
	// init and strips comments in skipWhitespaceAndComments, so they produce
	// no tokens other than newlines and EOF.
	if p.current.Type == lexer.TokenEOF {
		return nil, newError(1, 1, "empty file is not a valid HOCON document (HOCON.md L130)")
	}
	// root may be a bare object (no braces) or an explicit { ... }
	if p.current.Type != lexer.TokenLBrace {
		return p.parseObjectFields(false)
	}

	// Parse the first braced object, then continue merging any
	// additional content (braced objects or unbraced fields).
	// In HOCON, `{ a = 1 } { b = 2 }` and `{ a = 1 }\nb = 2`
	// are both valid — trailing content merges into the root.
	root, err := p.parseObject()
	if err != nil {
		return nil, err
	}

	for {
		p.skipNewlines()
		if p.current.Type == lexer.TokenEOF {
			break
		}
		if p.current.Type == lexer.TokenLBrace {
			// Another braced object — merge its fields
			obj, err := p.parseObject()
			if err != nil {
				return nil, err
			}
			root.Fields = append(root.Fields, obj.Fields...)
		} else {
			// Unbraced trailing fields — parse and merge
			obj, err := p.parseObjectFields(false)
			if err != nil {
				return nil, err
			}
			root.Fields = append(root.Fields, obj.Fields...)
		}
	}
	return root, nil
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
		if p.current.Type == lexer.TokenError {
			return nil, newError(p.current.Line, p.current.Col, "%s", p.current.Value)
		}
		if braced && p.current.Type == lexer.TokenRBrace {
			p.advance()
			break
		}
		if p.current.Type == lexer.TokenEOF {
			if braced {
				return nil, newError(p.current.Line, p.current.Col, "unexpected EOF, expected '}'")
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

// onlyClosingParens reports whether `s` is a non-empty string consisting
// solely of `)` characters. Used by parseInclude's post-path noise loop to
// consume trailing `)` close-paren tokens (`)`, `))`, etc.) without
// swallowing arbitrary tokens that may be the start of the next field —
// HOCON allows field separator omission on the same line, so a too-broad
// post-path skip would silently drop real data (go.hocon#101 Copilot
// review).
func onlyClosingParens(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != ')' {
			return false
		}
	}
	return true
}

// isIncludeSkipToken returns true if `tok` is an include-syntax "noise" token
// that may legitimately appear between an include directive keyword
// (`include` / `required` / `file`) and the quoted path string. With parens
// now ordinary unquoted-continue chars, the lexer produces tokens like `(`,
// `(file(`, `file(`, `file`, etc. depending on whitespace placement. The
// parser must accept any of these as skip-able WITHOUT swallowing real
// statement-boundary tokens (`,` / `}` / `=` / `:` etc.) that would otherwise
// silently mask malformed includes — see go.hocon#100 review feedback.
//
// Skip-able shapes (after trimming leading `(`):
//   - empty (just `(`)
//   - exact "file" / "url" / "classpath" (bare resource word before whitespace+paren)
//   - prefixed with "file(" / "url(" / "classpath(" (resource word with paren)
//
// Anything else (e.g. "abc", "fileX(", number, comma, equals, etc.) is malformed
// and the caller must error before treating it as path-discovery noise.
//
// url/classpath rejection itself is handled outside this helper.
func isIncludeSkipToken(tok lexer.Token) bool {
	if tok.Type != lexer.TokenString || tok.IsQuoted {
		return false
	}
	v := strings.TrimPrefix(tok.Value, "(")
	if v == "" || v == "file" || v == "url" || v == "classpath" {
		return true
	}
	return strings.HasPrefix(v, "file(") || strings.HasPrefix(v, "url(") || strings.HasPrefix(v, "classpath(")
}

// skipToIncludePath advances the parser through any include-syntax noise
// tokens (`(`, `file(`, `(file(`, etc.) until it reaches the quoted path
// string. While scanning, it detects and rejects unsupported `url(...)` /
// `classpath(...)` forms, and sets `sawFile=true` if it traversed a `file(`
// token (covering the whitespace form `required ( file("foo"))` where the
// outer-token check at the head of parseInclude misses the `file` marker —
// see go.hocon#100 multi-agent review).
//
// Returns the resolved sawFile flag, or an error if a non-skip-able token
// is encountered before a quoted path is found (e.g. `include file(42) b = "x"`
// must NOT silently bind path to `"x"`).
func (p *parser) skipToIncludePath(directiveLine, directiveCol int, directiveLabel string) (sawFile bool, err error) {
	for {
		if p.current.Type == lexer.TokenString && p.current.IsQuoted {
			return sawFile, nil
		}
		if p.current.Type == lexer.TokenEOF || p.current.Type == lexer.TokenNewline {
			return false, newError(directiveLine, directiveCol,
				"expected include path string in include %s directive", directiveLabel)
		}
		if !isIncludeSkipToken(p.current) {
			return false, newError(p.current.Line, p.current.Col,
				"unexpected token %q before include path string (include argument must be a single quoted string, or `file(...)` / `url(...)` / `classpath(...)` optionally wrapped in `required(...)`)",
				p.current.Value)
		}
		v := strings.TrimPrefix(p.current.Value, "(")
		if v == "url" || strings.HasPrefix(v, "url(") {
			return false, newError(p.current.Line, p.current.Col, "include %s url(...) is not supported in v1.0", directiveLabel)
		}
		if v == "classpath" || strings.HasPrefix(v, "classpath(") {
			return false, newError(p.current.Line, p.current.Col, "include %s classpath(...) is not supported in v1.0", directiveLabel)
		}
		if v == "file" || strings.HasPrefix(v, "file(") {
			sawFile = true
		}
		p.advance()
	}
}

func (p *parser) parseInclude() (*IncludeNode, error) {
	line, col := p.current.Line, p.current.Col
	p.advance() // consume "include"
	p.skipNewlines()

	cur := p.current
	isUnquoted := cur.Type == lexer.TokenString && !cur.IsQuoted

	var path string
	required := false
	isFile := false

	switch {
	case cur.Type == lexer.TokenString && cur.IsQuoted:
		// include "path"
		path = cur.Value
		p.advance()

	case isUnquoted && (cur.Value == "required" || strings.HasPrefix(cur.Value, "required(")):
		// include required(...)
		// Post-#34 (Option C, mirrors ts.hocon parseInclude): with parens now
		// ordinary unquoted-continue chars, the lexer produces `required(`,
		// `required(file(`, `required(url(`, etc. as a single unquoted token
		// (no whitespace before `(`). With whitespace before `(`, `required`
		// is its own token and the next token starts with `(`. Either form
		// must be accepted; non-skip-able tokens (statement boundaries,
		// unknown words, numbers) before the path raise an error rather than
		// silently swallowing later fields — see `skipToIncludePath`.
		required = true
		innerPrefix := ""
		if strings.HasPrefix(cur.Value, "required(") {
			innerPrefix = strings.TrimPrefix(cur.Value, "required(")
		}
		bareRequired := cur.Value == "required"
		p.advance() // consume `required` or `required(...)` token

		// Bare `required` (whitespace before `(`): the next token must start with `(`.
		if bareRequired {
			if p.current.Type != lexer.TokenString || p.current.IsQuoted || !strings.HasPrefix(p.current.Value, "(") {
				return nil, newError(line, col, "expected '(' after 'required' in include directive")
			}
		}

		// Reject url(...) / classpath(...) and unknown resource words inside required(...) —
		// same-token form. Use exact `file(` / `url(` / `classpath(` (or bare word) match
		// so `required(fileX(...))` is correctly rejected as unknown rather than mis-classified
		// as a file include (go.hocon#100 multi-agent review).
		if strings.HasPrefix(innerPrefix, "url(") || innerPrefix == "url" {
			return nil, newError(line, col, "include required(url(...)) is not supported in v1.0")
		}
		if strings.HasPrefix(innerPrefix, "classpath(") || innerPrefix == "classpath" {
			return nil, newError(line, col, "include required(classpath(...)) is not supported in v1.0")
		}
		if strings.HasPrefix(innerPrefix, "file(") || innerPrefix == "file" {
			isFile = true
			// Bare inner `file` (whitespace before `(`): the next token must start with `(`
			// so `include required(file "x")` is correctly rejected rather than silently
			// accepted as a file include (go.hocon#101 Copilot review).
			if innerPrefix == "file" {
				if p.current.Type != lexer.TokenString || p.current.IsQuoted || !strings.HasPrefix(p.current.Value, "(") {
					return nil, newError(line, col, "expected '(' after 'file' in include required(file(...))")
				}
			}
		} else if innerPrefix != "" {
			return nil, newError(line, col,
				"include required(...) inner resource %q is not recognised — must be a quoted string or `file(...)` / `url(...)` / `classpath(...)`",
				innerPrefix)
		}

		sawFile, err := p.skipToIncludePath(line, col, "required(...)")
		if err != nil {
			return nil, err
		}
		if sawFile {
			isFile = true
		}
		path = p.current.Value
		p.advance()
		// Consume only trailing `)` close-paren noise — see `onlyClosingParens`
		// docstring for why this is narrower than a skip-until-newline loop.
		for p.current.Type == lexer.TokenString && !p.current.IsQuoted && onlyClosingParens(p.current.Value) {
			p.advance()
		}

	case isUnquoted && (cur.Value == "file" || strings.HasPrefix(cur.Value, "file(")):
		// include file(...)
		isFile = true
		bareFile := cur.Value == "file"
		p.advance()
		// Bare `file` (whitespace before `(`): the next token must start with `(`
		// so `include file "x"` is correctly rejected rather than silently accepted
		// as a file include (mirrors the bare-`required` check above; go.hocon#101
		// Copilot review).
		if bareFile {
			if p.current.Type != lexer.TokenString || p.current.IsQuoted || !strings.HasPrefix(p.current.Value, "(") {
				return nil, newError(line, col, "expected '(' after 'file' in include directive")
			}
		}
		if _, err := p.skipToIncludePath(line, col, "file(...)"); err != nil {
			return nil, err
		}
		path = p.current.Value
		p.advance()
		for p.current.Type == lexer.TokenString && !p.current.IsQuoted && onlyClosingParens(p.current.Value) {
			p.advance()
		}

	case isUnquoted && (cur.Value == "url" || strings.HasPrefix(cur.Value, "url(")):
		return nil, newError(line, col, "include url(...) is not supported in v1.0")

	case isUnquoted && (cur.Value == "classpath" || strings.HasPrefix(cur.Value, "classpath(")):
		return nil, newError(line, col, "include classpath(...) is not supported in v1.0")

	case isUnquoted:
		// Bare unquoted argument (S14a.10): user meant an include statement but forgot the quotes.
		return nil, newError(cur.Line, cur.Col,
			fmt.Sprintf("include argument must be a quoted string, got unquoted: %q (HOCON.md L958)", cur.Value))

	default:
		// S12.5: `include` followed by separator/EOF/brace etc. — user intended it as a key name.
		return nil, newError(line, col,
			"'include' is reserved as a key name; use \"include\" (quoted) to use it as a field (HOCON.md L570)")
	}

	return &IncludeNode{pos: pos{line, col}, Path: path, Required: required, IsFile: isFile}, nil
}

func (p *parser) parseField() (*FieldNode, error) {
	line, col := p.current.Line, p.current.Col
	// S12.5: capture first-token provenance BEFORE parseKey advances past it.
	// We need to know whether the very first token was quoted and what its type
	// was, so the reservation check below can distinguish:
	//   include.foo = 1  → TokenString, IsQuoted=false → REJECT
	//   "include".foo = 1 → TokenString, IsQuoted=true  → allow
	firstTokenIsQuoted := p.current.IsQuoted
	firstTokenType := p.current.Type
	// parse key (dot-separated path, possibly multi-segment with quoted parts)
	key, err := p.parseKey()
	if err != nil {
		return nil, err
	}
	// S12.5 (HOCON.md L570): 'include' is reserved at the start of an unquoted
	// key path. TokenInclude inputs (`include = 1`, `include {...}`,
	// `include += [1]`) never reach parseField — they are dispatched to
	// parseInclude in parseObjectFields. This check handles the TokenString
	// case: `include.foo = 1` (lexer emits a single TokenString "include.foo",
	// parseKey splits on '.' to produce ["include", "foo"]).
	if len(key) > 0 && key[0] == "include" && !firstTokenIsQuoted && firstTokenType == lexer.TokenString {
		return nil, newError(line, col,
			"'include' is reserved at the start of a key path; use \"include\" (quoted) or rename the key (HOCON.md L570)")
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
		return nil, newError(p.current.Line, p.current.Col, "expected ':', '=' or '{' after key")
	}
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &FieldNode{pos: pos{line, col}, Key: key, Value: val, Append: append_}, nil
}

// validateKeySegment enforces HOCON.md L270-276 (S8.6) on a single unquoted /
// numeric key segment: a segment that begins with '-' must be followed by a
// digit. Quoted segments bypass this and are not passed here.
// Precondition: s must be non-empty (callers guard with `if s == "" { continue }`).
func validateKeySegment(line, col int, s string) error {
	if s[0] != '-' {
		return nil
	}
	if len(s) >= 2 && s[1] >= '0' && s[1] <= '9' {
		return nil
	}
	after := "EOF"
	if len(s) >= 2 {
		after = fmt.Sprintf("%q", rune(s[1]))
	}
	return newError(line, col, "unquoted key segment cannot begin with '-' unless followed by a digit (got '-' then %s in %q, HOCON.md L270-276)", after, s)
}

func (p *parser) parseKey() ([]string, error) {
	line, col := p.current.Line, p.current.Col
	if p.current.Type == lexer.TokenError {
		return nil, newError(line, col, "%s", p.current.Value)
	}
	if p.current.Type != lexer.TokenString && p.current.Type != lexer.TokenInt && p.current.Type != lexer.TokenFloat {
		return nil, newError(line, col, "expected key, got %v", p.current.Type)
	}

	var parts []string
	// prevKeyTokenIsNumeric tracks whether the segment most recently pushed to
	// `parts` came from a TokenInt or TokenFloat. This gates the adjacent-token
	// concat branch: concat may re-split the merged value on '.', so it must
	// not run after a quoted segment (whose literal '.' must not be
	// reinterpreted as a path separator) or after a plain unquoted
	// TokenString (which the lexer would have merged into one token if it
	// were genuinely adjacent).
	prevKeyTokenIsNumeric := false

	for {
		raw := p.current.Value
		isQuoted := p.current.IsQuoted
		prevTokenType := p.current.Type
		p.advance()

		if isQuoted {
			// Quoted key segment — no dot splitting
			parts = append(parts, raw)
			prevKeyTokenIsNumeric = false
		} else {
			// Unquoted / numeric key — split on dots for path notation. A trailing
			// dot (e.g., "arrays.") means the next token continues the path. For
			// TokenFloat (e.g., "3.14") this produces nested segments ["3","14"]
			// per HOCON.md key-as-path convention.
			segments := strings.Split(raw, ".")
			for _, s := range segments {
				if s == "" {
					continue // skip empty segments from leading/trailing dots
				}
				if err := validateKeySegment(line, col, s); err != nil {
					return nil, err
				}
				parts = append(parts, s)
			}
			prevKeyTokenIsNumeric = prevTokenType == lexer.TokenInt || prevTokenType == lexer.TokenFloat
			// If the raw value ends with '.', the next token is a continuation
			if strings.HasSuffix(raw, ".") {
				continue // read the next segment
			}
		}

		// Adjacent-token key concat (numeric only): a TokenInt or TokenFloat
		// followed by another stringifiable unquoted token with no
		// intervening whitespace merges into a single key segment. This is
		// the key-position analogue of value-position concat for `123abc`
		// (which the lexer splits as TokenInt("123") + TokenString("abc")
		// because S8.6 forbids a bare digit-leading unquoted token), and it
		// extends to keyword tails like `123true` (TokenBool) / `123null`
		// (TokenNull). The dotted form `123true.foo` already worked because
		// the lexer reads `true.foo` as a single TokenString (it only
		// keyword-promotes on the exact token value), so this branch fires
		// with the TokenString tail in that case as well — adding keyword
		// types here closes the bare-keyword asymmetry.
		// We deliberately do NOT run this branch after a quoted segment — a
		// literal '.' inside `"a.b"` must not be re-interpreted as a path
		// separator when concatenated with a following unquoted token. The
		// lexer also never emits two adjacent unquoted TokenStrings, so this
		// branch only matters when the previous token was numeric. The
		// leading-dot continuation check below still applies independently.
		isConcatTail := false
		switch p.current.Type {
		case lexer.TokenString:
			isConcatTail = !p.current.IsQuoted
		case lexer.TokenBool, lexer.TokenNull, lexer.TokenInclude:
			isConcatTail = true
		}
		if prevKeyTokenIsNumeric && isConcatTail && !p.current.PrecedingSpace && !strings.HasPrefix(p.current.Value, ".") && len(parts) > 0 {
			tail := p.current.Value
			p.advance()
			merged := parts[len(parts)-1] + tail
			parts = parts[:len(parts)-1]
			segments := strings.Split(merged, ".")
			for _, s := range segments {
				if s == "" {
					continue
				}
				if err := validateKeySegment(line, col, s); err != nil {
					return nil, err
				}
				parts = append(parts, s)
			}
			// After numeric+unquoted concat the merged segment is no longer a
			// pure number, so further concat is not allowed: a trailing dot
			// re-enters the loop (which resets prevKeyTokenIsNumeric from the
			// next token), otherwise we fall through to the leading-dot /
			// break checks below.
			if strings.HasSuffix(tail, ".") {
				continue // next token continues the path
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
		return nil, newError(line, col, "empty key")
	}
	return parts, nil
}

func (p *parser) parseValue() (Node, error) {
	p.skipNewlines()
	// Capture position before consuming the first token so ConcatNode can carry it.
	firstLine, firstCol := p.current.Line, p.current.Col
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
			nodes = append(nodes, &ScalarNode{Raw: " ", ValueType: "string"})
		}
		nodes = append(nodes, next)
	}
	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return &ConcatNode{pos: pos{firstLine, firstCol}, Nodes: nodes}, nil
}

func (p *parser) parseSingleValue() (Node, error) {
	if p.current.Type == lexer.TokenError {
		return nil, newError(p.current.Line, p.current.Col, "%s", p.current.Value)
	}
	line, col := p.current.Line, p.current.Col
	switch p.current.Type {
	case lexer.TokenLBrace:
		return p.parseObject()
	case lexer.TokenLBracket:
		return p.parseArray()
	case lexer.TokenSubstitution:
		val := p.current.Value
		optional := p.current.Subst != nil && p.current.Subst.Optional
		subst := p.current.Subst
		p.advance()
		return &SubstNode{pos: pos{line, col}, Path: val, Optional: optional, Segments: subst}, nil
	case lexer.TokenString:
		val := p.current.Value
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Raw: val, ValueType: "string"}, nil
	case lexer.TokenInclude:
		// `include` as a bare unquoted word in value position is an unquoted
		// string literal — e.g. `a = include` produces { a: "include" }.
		// The reservation rule (HOCON.md L570) applies only to key paths, not
		// value positions. The lexer always promotes the bare keyword to
		// TokenInclude; we demote it back to a string scalar here.
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Raw: "include", ValueType: "string"}, nil
	case lexer.TokenInt:
		raw := p.current.Value
		p.advance()
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, newError(line, col, "invalid int %q", raw)
		}
		// E8 (xx.hocon#31): canonicalize leading zeros and negative-zero sign
		// at value-position only. `parseKey` reads the same TokenInt.Value
		// upstream, so normalizing in the lexer would silently rewrite keys
		// (`01 = x` → `1`); keep it confined to the value path.
		return &ScalarNode{pos: pos{line, col}, Raw: strconv.FormatInt(parsed, 10), ValueType: "number"}, nil
	case lexer.TokenFloat:
		raw := p.current.Value
		p.advance()
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return nil, newError(line, col, "invalid float %q", raw)
		}
		return &ScalarNode{pos: pos{line, col}, Raw: raw, ValueType: "number"}, nil
	case lexer.TokenBool:
		raw := p.current.Value
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Raw: raw, ValueType: "boolean"}, nil
	case lexer.TokenNull:
		p.advance()
		return &ScalarNode{pos: pos{line, col}, Raw: "null", ValueType: "null"}, nil
	default:
		return nil, newError(line, col, "unexpected token %v", p.current.Type)
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
			return nil, newError(p.current.Line, p.current.Col, "unexpected EOF in array")
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
