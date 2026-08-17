// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/o3co/go.hocon/internal/lexer"
)

// ValidatePackageFile checks E11 decision 6 constraints on the <file> argument
// of a package(...) include. Returns a non-nil error if any constraint is violated.
// Called at parse time (returns *Error via newError wrapper) and at registration time.
//
// Constraints (OS-independent):
//   - must be non-empty
//   - must not start with "/" (Unix absolute)
//   - must not contain "\" (Windows separator)
//   - must not contain "//" (consecutive slashes)
//   - must not contain "." or ".." path segments (directory traversal)
//   - must not start with a Windows drive prefix (e.g. "C:" or "c:")
//   - must not have an empty trailing segment (i.e. must not end with "/")
func ValidatePackageFile(file string) error {
	if file == "" {
		return fmt.Errorf("include package(...) file argument must be non-empty")
	}
	if strings.HasPrefix(file, "/") {
		return fmt.Errorf("include package(...) file argument must not be an absolute path: %q", file)
	}
	if strings.Contains(file, "\\") {
		return fmt.Errorf("include package(...) file argument must use forward-slash separators (backslash not allowed): %q", file)
	}
	// Windows drive-prefix check (e.g. "C:/..." or "C:...") — reject regardless of OS.
	// A drive prefix is a single ASCII letter followed by ':'.
	if len(file) >= 2 && file[1] == ':' {
		ch := file[0]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			return fmt.Errorf("include package(...) file argument must not be a Windows absolute path: %q", file)
		}
	}
	if strings.Contains(file, "//") {
		return fmt.Errorf("include package(...) file argument must not contain consecutive slashes: %q", file)
	}
	for _, seg := range strings.Split(file, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf(`include package(...) file argument must not contain "." or ".." segments: %q`, file)
		}
		if seg == "" {
			// Empty segment: either leading "/" (caught above) or trailing "/" or "//"
			// (caught above). Any remaining empty segment means a trailing slash.
			return fmt.Errorf("include package(...) file argument must not end with a slash (empty trailing segment): %q", file)
		}
	}
	return nil
}

// ErrArrayAtRoot marks a syntactically valid array-root document rejected by
// the object-rooted Config API (S3.5, HOCON.md L989-991). The parser parses
// the array successfully and returns an error wrapping this sentinel; the
// public boundary (hocon.parseWithOptions) converts it to the type-mismatch
// error class, and the resolver converts it for include paths (S14b.1,
// HOCON.md L993-994). Detect with errors.Is.
var ErrArrayAtRoot = errors.New("document has type array rather than object at file root")

// Parse parses a HOCON string and returns the root ObjectNode.
// The input may omit outer braces (root object shorthand).
func Parse(src string) (*ObjectNode, error) {
	// S1.1 (HOCON.md L117): files must be valid UTF-8. A Go string is not
	// language-guaranteed UTF-8, so arbitrary bytes reach this boundary via
	// ParseString and file reads; reject them here rather than letting rune
	// decoding silently substitute U+FFFD. Every HOCON document — top-level
	// or include — funnels through Parse, so this is the single choke point.
	if !utf8.ValidString(src) {
		line, col := invalidUTF8Position(src)
		return nil, newError(line, col, "invalid UTF-8 byte sequence: HOCON input must be valid UTF-8 (S1.1)")
	}
	p := &parser{lex: lexer.New(src)}
	p.advance()
	return p.parseRoot()
}

// invalidUTF8Position returns the 1-based line/col of the first invalid UTF-8
// byte sequence in src. Call only when utf8.ValidString(src) is false; a
// properly encoded U+FFFD decodes with size 3 and is never reported.
func invalidUTF8Position(src string) (line, col int) {
	line, col = 1, 1
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == utf8.RuneError && size == 1 {
			return line, col
		}
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		i += size
	}
	return line, col
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
	// S3.1 (corrected, xx.hocon E10): an empty document — empty string,
	// whitespace-only, newlines-only, comment-only, BOM-only, or mixed — is
	// valid HOCON and parses to the empty object, per the HOCON.md L134-136
	// brace-omission relaxation (L130-132 is the JSON baseline). An EOF-only
	// stream falls through to parseObjectFields(false), which returns an
	// empty ObjectNode.
	// S3.5 (HOCON.md L989-991): "both JSON and HOCON allow arrays as root
	// values in a document" — an array-root document is valid syntax. Parse
	// the array fully (malformed arrays and trailing content stay syntax
	// errors), then return ErrArrayAtRoot so the Config boundary can reject
	// it as a TYPE error, matching Lightbend's Parseable.forceParsedToObject
	// (WrongType "has type LIST rather than object at file root").
	if p.current.Type == lexer.TokenLBracket {
		line, col := p.current.Line, p.current.Col
		if _, err := p.parseArray(); err != nil {
			return nil, err
		}
		p.skipNewlines()
		if p.current.Type != lexer.TokenEOF {
			return nil, newError(p.current.Line, p.current.Col, "unexpected token after root array")
		}
		return nil, fmt.Errorf("%d:%d: %w (HOCON.md L989-991); the Config API requires an object at file root", line, col, ErrArrayAtRoot)
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

// skipSeparator consumes an optional comma surrounded by optional newlines.
// HOCON accepts a comma after newline(s) following a completed field/element
// (e.g. `a: 1\n,\nb: 2`), so newlines must be consumed before and after the
// optional comma — not only after (closes #104).
func (p *parser) skipSeparator() {
	p.skipNewlines()
	if p.current.Type == lexer.TokenComma {
		p.advance()
		p.skipNewlines()
	}
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
		// same-token form. Use exact `file(` / `url(` / `classpath(` / `package(` (or bare word) match
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
		} else if strings.HasPrefix(innerPrefix, "package(") || innerPrefix == "package" {
			// E11: include required(package("id", "file")) — two-arg form mandatory.
			if innerPrefix == "package" {
				if p.current.Type != lexer.TokenString || p.current.IsQuoted || !strings.HasPrefix(p.current.Value, "(") {
					return nil, newError(line, col, "expected '(' after 'package' in include required(package(...))")
				}
			}
			id, file, err := p.parsePackageArgs(line, col)
			if err != nil {
				return nil, err
			}
			// Consume trailing `)` close-paren noise (one for package, one for required)
			for p.current.Type == lexer.TokenString && !p.current.IsQuoted && onlyClosingParens(p.current.Value) {
				p.advance()
			}
			return &IncludeNode{
				pos:       pos{line, col},
				Required:  true,
				IsPackage: true,
				PkgID:     id,
				PkgFile:   file,
			}, nil
		} else if innerPrefix != "" {
			return nil, newError(line, col,
				"include required(...) inner resource %q is not recognised — must be a quoted string or `file(...)` / `url(...)` / `classpath(...)` / `package(...)`",
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

	case isUnquoted && (cur.Value == "package" || strings.HasPrefix(cur.Value, "package(")):
		// E11: include package("identifier", "file") — two-arg form mandatory.
		// Post-#34 token model: `package(` may be a single unquoted token (no whitespace
		// before `(`), or `package` may be its own token followed by a `(`-prefixed token
		// (whitespace before `(`). Handle both.
		barePackage := cur.Value == "package"
		p.advance() // consume `package` or `package(...)` token
		if barePackage {
			if p.current.Type != lexer.TokenString || p.current.IsQuoted || !strings.HasPrefix(p.current.Value, "(") {
				return nil, newError(line, col, "expected '(' after 'package' in include directive")
			}
		}
		id, file, err := p.parsePackageArgs(line, col)
		if err != nil {
			return nil, err
		}
		// Consume trailing `)` close-paren noise (one for package).
		for p.current.Type == lexer.TokenString && !p.current.IsQuoted && onlyClosingParens(p.current.Value) {
			p.advance()
		}
		return &IncludeNode{
			pos:       pos{line, col},
			Required:  false,
			IsPackage: true,
			PkgID:     id,
			PkgFile:   file,
		}, nil

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

// parsePackageArgs parses the ("identifier", "file") portion of a package(...)
// qualifier. On entry the `package(` (or bare `package` + `(`-prefixed token)
// prefix has already been consumed. On exit the cursor is positioned at the
// first token after the closing `)` of the package qualifier — callers are
// responsible for consuming additional trailing `)` (e.g. the outer `required(`
// close-paren) via the onlyClosingParens loop.
//
// Returns the identifier and file (both already HOCON-unescaped by the lexer),
// or a *parserError positioned at `directiveLine`/`directiveCol`.
func (p *parser) parsePackageArgs(directiveLine, directiveCol int) (id, file string, err error) {
	// Skip any `(` / `(file(` etc. noise tokens until we reach the first quoted
	// string (the identifier). With the post-#34 lexer model, a `package(` token
	// has been consumed by the caller; if there was whitespace before the `(`,
	// the next token here is a bare `(` (a TokenString unquoted starting with `(`).
	if _, err = p.skipToIncludePath(directiveLine, directiveCol, "package(...)"); err != nil {
		return "", "", err
	}
	if p.current.Type != lexer.TokenString || !p.current.IsQuoted {
		return "", "", newError(directiveLine, directiveCol,
			"expected quoted identifier string as first argument in include package(...)")
	}
	id = p.current.Value
	p.advance()
	if id == "" {
		return "", "", newError(directiveLine, directiveCol,
			"include package(...) identifier must be non-empty")
	}

	// Two-arg form mandatory (E11 decision 2). One-arg form `package("id/file")`
	// would have the cursor at a `)`-only token here.
	if p.current.Type != lexer.TokenComma {
		if p.current.Type == lexer.TokenString && !p.current.IsQuoted && onlyClosingParens(p.current.Value) {
			return "", "", newError(directiveLine, directiveCol,
				"include package(...) requires two arguments (identifier, file); one-arg form is not supported (E11 decision 2)")
		}
		return "", "", newError(directiveLine, directiveCol,
			"expected ',' after identifier in include package(identifier, file)")
	}
	p.advance() // consume ','
	p.skipNewlines()

	if p.current.Type != lexer.TokenString || !p.current.IsQuoted {
		return "", "", newError(directiveLine, directiveCol,
			"expected quoted file string as second argument in include package(...)")
	}
	file = p.current.Value
	p.advance()

	// Validate file argument per E11 decision 6 (after HOCON unescape — lexer already unescaped).
	if verr := ValidatePackageFile(file); verr != nil {
		return "", "", newError(directiveLine, directiveCol, "%v", verr)
	}
	return id, file, nil
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

func (p *parser) parseKey() ([]string, error) {
	line, col := p.current.Line, p.current.Col
	if p.current.Type == lexer.TokenError {
		return nil, newError(line, col, "%s", p.current.Value)
	}
	// S11.8 (HOCON.md L504): path expressions always stringify, so TokenBool
	// ("true" / "false") and TokenNull ("null") are valid key starts — the
	// existing unquoted-branch below pushes them as single-segment string keys.
	// TokenInclude is NOT in this list: `include` at the start of a key is
	// reserved per S12.5 and dispatched as a directive before parseKey runs.
	if p.current.Type != lexer.TokenString &&
		p.current.Type != lexer.TokenInt &&
		p.current.Type != lexer.TokenFloat &&
		p.current.Type != lexer.TokenBool &&
		p.current.Type != lexer.TokenNull {
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
	// S10.8 (HOCON.md L317 + L553-560): "path expressions work like value
	// concatenations" — when the next key token has whitespace before it (and
	// is not a leading-dot continuation), it is a space-concat continuation
	// that merges into the LAST existing segment, using the LITERAL whitespace
	// from the source (PrecedingWhitespace, not a hardcoded ' '):
	//   `a b = 1`         → ['a b']
	//   `a b c : 42`      → ['a b c']        (spec L556 example)
	//   `a.b c = 1`       → ['a', 'b c']     (concat into last segment)
	//   `"a" b = 1`       → ['a b']          (quoted + unquoted)
	// E13 (xx.hocon#42) — path-expression whitespace is preserved verbatim
	// around dots, including the tab variant pw07:
	//   `a b. c = 1`      → ['a b', ' c']    (leading ' ' on " c" preserved)
	//   `a b.\tc = 1`     → ['a b', '\tc']   (HOCON_WS tab uniformly preserved)
	//   `a .b = 1`        → ['a ', 'b']      (trailing ' ' on 'a', leading
	//                                          dot still separator)
	//   `a . b = 1`       → ['a ', ' b']     (both sides preserved)
	//   `a. .b = 1`       → ['a', ' ', 'b']  (dot-WS-dot: WS becomes its
	//                                          own segment between two dots)
	//
	// S8.6 (HOCON.md L270-276) is NOT enforced on key path segments per E13
	// (xx.hocon#42): the rule is value-position lexer-disambiguation, not a
	// key-parser rule. Lightbend accepts `foo -bar = 1`, `foo.-bar = 1`, etc.
	spaceConcat := false
	// trailingDot tracks whether the path is currently "after" a dot separator,
	// expecting a continuation segment. Used for the post-loop guard that
	// rejects empty-trailing-segment keys (e.g. `a b. = 1`, pw06).
	trailingDot := false
	// postDotPrefix carries WS captured from a trailing-dot continuation, to be
	// applied as the leading prefix on the next segment (E13 path-WS rule).
	postDotPrefix := ""

	for {
		raw := p.current.Value
		isQuoted := p.current.IsQuoted
		prevTokenType := p.current.Type
		ws := p.current.PrecedingWhitespace
		tokLine, tokCol := p.current.Line, p.current.Col
		p.advance()

		if isQuoted {
			// Quoted key segment — no dot splitting
			if spaceConcat && len(parts) > 0 {
				// E13: preceding WS verbatim, then quoted content merged into last segment.
				parts[len(parts)-1] = parts[len(parts)-1] + ws + raw
			} else if postDotPrefix != "" {
				// post-dot WS becomes leading prefix on the new quoted segment
				parts = append(parts, postDotPrefix+raw)
				postDotPrefix = ""
			} else {
				parts = append(parts, raw)
			}
			prevKeyTokenIsNumeric = false
			trailingDot = false
		} else {
			// Unquoted / numeric key — split on dots for path notation. A trailing
			// dot (e.g., "arrays.") means the next token continues the path. For
			// TokenFloat (e.g., "3.14") this produces nested segments ["3","14"]
			// per HOCON.md key-as-path convention.
			newParts, err := splitKeySegments(raw, len(parts) > 0)
			if err != nil {
				return nil, newError(tokLine, tokCol, "%s", err.Error())
			}
			if spaceConcat && len(parts) > 0 {
				// E13 path-WS preservation: the literal preceding WS becomes
				// trailing on the PREVIOUS segment, uniformly. Then:
				//  - if raw starts with '.', the dot is a separator (S11.1)
				//    and filtered pieces (if any) become new segments;
				//  - otherwise the first piece merges into the just-extended
				//    segment, with remaining pieces as new segments.
				parts[len(parts)-1] = parts[len(parts)-1] + ws
				if strings.HasPrefix(raw, ".") {
					parts = append(parts, newParts...)
				} else if len(newParts) > 0 {
					parts[len(parts)-1] = parts[len(parts)-1] + newParts[0]
					parts = append(parts, newParts[1:]...)
				}
			} else if postDotPrefix != "" && strings.HasPrefix(raw, ".") {
				// E13 dot-WS-dot case (e.g. `a. .b = 1`): after a trailing dot
				// from the previous token, the WS-then-dot sequence means the
				// WS becomes its OWN path segment (between the two dot
				// separators), and the leading dot starts a new segment chain.
				// Lightbend: `a. .b = 1` → {"a":{" ":{"b":1}}} = ['a', ' ', 'b'].
				// Empirically verified via typesafe-config 1.4.3 probe.
				parts = append(parts, postDotPrefix)
				postDotPrefix = ""
				parts = append(parts, newParts...)
			} else if postDotPrefix != "" && len(newParts) > 0 {
				// post-dot WS becomes leading prefix on the new segment (E13)
				parts = append(parts, postDotPrefix+newParts[0])
				parts = append(parts, newParts[1:]...)
				postDotPrefix = ""
			} else {
				parts = append(parts, newParts...)
			}
			prevKeyTokenIsNumeric = prevTokenType == lexer.TokenInt || prevTokenType == lexer.TokenFloat
			// If the raw value ends with '.', the next token is a continuation
			if strings.HasSuffix(raw, ".") {
				spaceConcat = false
				trailingDot = true
				// E13: capture next token's PrecedingWhitespace for post-dot prefix.
				if p.current.PrecedingWhitespace != "" {
					switch p.current.Type {
					case lexer.TokenString, lexer.TokenInt, lexer.TokenFloat, lexer.TokenBool, lexer.TokenNull, lexer.TokenInclude:
						postDotPrefix = p.current.PrecedingWhitespace
					}
				}
				// Copilot review on PR #125: continue would re-enter the loop
				// even when the next token cannot start a key segment (=, :,
				// {, newline, EOF), allowing those tokens to be absorbed into
				// the key and bypassing the post-loop trailingDot guard. Only
				// continue when the next token is a real key-eligible
				// continuation; otherwise break so the BadPath error fires
				// with the correct "trailing period" message.
				switch p.current.Type {
				case lexer.TokenString, lexer.TokenInt, lexer.TokenFloat, lexer.TokenBool, lexer.TokenNull, lexer.TokenInclude:
					continue // read the next segment
				}
				break
			}
			trailingDot = false
		}
		// The continuation we just took has been consumed.
		spaceConcat = false

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
		// Adjacent-token key concat chain: once we have a numeric leading
		// segment, consume a chain of adjacent stringifiable tails as long
		// as each is unquoted (or a stringifiable keyword/literal), has no
		// preceding whitespace, and no leading dot. Each tail extends the
		// merged key text. The chain absorbs:
		//   - TokenString (unquoted): closes the `123abc` asymmetry (#81)
		//   - TokenBool / TokenNull / TokenInclude: keyword tails (#66 area)
		//   - TokenInt / TokenFloat: signed-numeric tails like `123-456`
		//     (#83) — the lexer's `readNumber` produces TokenInt("-456")
		//     when `-` is immediately followed by digits, so chained
		//     numeric segments arrive as separate adjacent number tokens.
		//
		// A trailing dot in a tail breaks the chain — the next token is a
		// new path segment, handled by the outer loop via `continue`.
		concatChain := prevKeyTokenIsNumeric && len(parts) > 0
		concatTrailingDotContinue := false
		for concatChain {
			isConcatTail := false
			switch p.current.Type {
			case lexer.TokenString:
				isConcatTail = !p.current.IsQuoted
			case lexer.TokenBool, lexer.TokenNull, lexer.TokenInclude:
				isConcatTail = true
			case lexer.TokenInt, lexer.TokenFloat:
				isConcatTail = true
			}
			if !isConcatTail || p.current.PrecedingSpace || strings.HasPrefix(p.current.Value, ".") {
				break
			}
			tail := p.current.Value
			tailLine, tailCol := p.current.Line, p.current.Col
			p.advance()
			merged := parts[len(parts)-1] + tail
			parts = parts[:len(parts)-1]
			// E13: S8.6 not enforced on key path segments (was: validateKeySegment).
			// S11.7 empty-element rejection does apply here — `123..abc` must
			// fail just like `a..b`.
			mergedParts, err := splitKeySegments(merged, len(parts) > 0)
			if err != nil {
				return nil, newError(tailLine, tailCol, "%s", err.Error())
			}
			parts = append(parts, mergedParts...)
			if strings.HasSuffix(tail, ".") {
				// Trailing dot: the next token is a new path segment.
				// Signal the outer loop to `continue` so the next iteration
				// reads it as a fresh segment instead of treating it as a
				// tail of the chain.
				concatTrailingDotContinue = true
				trailingDot = true
				break
			}
		}
		if concatTrailingDotContinue {
			continue
		}

		// Check if the next token is an unquoted string starting with '.'
		// or a quoted string preceded by a dot (like in "a"."b" patterns).
		// After a quoted segment, look if the next unquoted starts with '.'.
		// E13: if the dot-leading token also has preceding whitespace, set
		// spaceConcat=true so the WS-before-dot is preserved as trailing on
		// the previous segment in the next iteration (`a .b = 1` → ['a ', 'b']).
		if p.current.Type == lexer.TokenString && !p.current.IsQuoted && strings.HasPrefix(p.current.Value, ".") {
			if p.current.PrecedingSpace {
				spaceConcat = true
			}
			continue
		}

		// S10.8 space-concat continuation: an unquoted/quoted key-position
		// token separated from the previous key token by whitespace is part
		// of the same key (HOCON.md L317 + L553-560). The leading-dot branch
		// above is checked FIRST so that `.b`-style continuations preserve
		// path-separator semantics (S11.1) even when preceded by whitespace.
		if p.current.PrecedingSpace {
			switch p.current.Type {
			case lexer.TokenString, lexer.TokenInt, lexer.TokenFloat, lexer.TokenBool, lexer.TokenNull, lexer.TokenInclude:
				spaceConcat = true
				continue
			}
		}
		break
	}

	// E13 pw06: a key path ending with `.` (e.g. `a b. = 1`) creates an empty
	// trailing segment. Lightbend throws BadPath; we match — loosening S8.6-in-key
	// and preserving path-WS does NOT cascade into accepting empty path segments.
	if trailingDot {
		return nil, newError(p.current.Line, p.current.Col,
			"path has a trailing period '.' — empty key segment not allowed (HOCON.md path rules)")
	}

	// Defensive backstop. Since S11.7 is enforced in splitKeySegments, every
	// all-dots key token (`.`, `..`) is rejected before reaching here, and the
	// lexer never emits an empty unquoted TokenString — so no known input lands
	// on this branch. Kept so a future token-shape change fails loudly rather
	// than producing a zero-segment key.
	if len(parts) == 0 {
		return nil, newError(line, col, "empty key")
	}
	return parts, nil
}

// errEmptyKeySegment is the S11.7 (HOCON.md L515-519) BadPath condition for
// key paths. The wording deliberately echoes the lexer's "empty segment in
// path" error for ${...} paths — parseSubstBody's state machine already
// enforced this rule, so the two positions now reject the same shapes.
var errEmptyKeySegment = errors.New(
	"path has an empty element — `a..b` and paths starting with '.' are invalid; " +
		"an empty path element must be quoted as \"\" (HOCON.md L515-519)")

// splitKeySegments splits an unquoted key token on '.' (the S11.1 path
// separator) and rejects empty path elements per S11.7. `havePrev` says
// whether the key path already has at least one segment, which decides
// whether a leading '.' is a separator or an illegal empty first element.
//
// Two empty pieces are structural rather than genuine empty elements and are
// dropped:
//   - a leading empty piece when havePrev is true — the token's leading '.'
//     separates this token from the segments already collected (`"a".b`,
//     `a. .b`, `a .b`);
//   - a trailing empty piece — the token's trailing '.' marks a continuation
//     to be read as the next token (`a. b`). parseKey's post-loop trailingDot
//     guard rejects it when no continuation actually follows (pw06).
//
// Every other empty piece is an S11.7 error: `a..b`, `.a` (havePrev false),
// `a...c`, `"a"..b`.
func splitKeySegments(raw string, havePrev bool) ([]string, error) {
	segments := strings.Split(raw, ".")
	var out []string
	for i, s := range segments {
		if s == "" {
			if (i == 0 && havePrev) || i == len(segments)-1 {
				continue
			}
			return nil, errEmptyKeySegment
		}
		out = append(out, s)
	}
	return out, nil
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
		// insert a separator node for proper concatenation. S10.5 (go.hocon#132):
		// preserve the literal whitespace run verbatim rather than collapsing to
		// a single space. Capture PrecedingWhitespace BEFORE parseSingleValue
		// advances the token. Fall back to a single space if the lexer reported
		// PrecedingSpace but captured no chars (comment-only shape, which cannot
		// occur mid-value-concat — the loop breaks on newline).
		hadSpace := p.current.PrecedingSpace
		precedingWS := p.current.PrecedingWhitespace
		next, err2 := p.parseSingleValue()
		if err2 != nil {
			break
		}
		if hadSpace {
			sep := precedingWS
			if sep == "" {
				sep = " "
			}
			nodes = append(nodes, &ScalarNode{Raw: sep, ValueType: "string", Separator: true})
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
		if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
			return nil, newError(line, col, "invalid int %q", raw)
		}
		// S10.11 (go.hocon#133): preserve the source lexeme so a numeric value
		// stringifies "as written" when concatenated (`minor = 05` →
		// `${major}.${minor}` = "26.05", and `00_example` keeps its "00"
		// prefix). The numeric accessors (GetInt64 etc. at config.go re-parse
		// Raw via strconv.ParseInt) still drop leading zeros / negative-zero
		// sign for the standalone value, so this refines — not reverses — the
		// earlier E8/F3 canonicalization, which over-canonicalized the stored
		// lexeme. `parseKey` reads the same TokenInt.Value upstream and is
		// unaffected (keys were never canonicalized here).
		return &ScalarNode{pos: pos{line, col}, Raw: raw, ValueType: "number"}, nil
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
