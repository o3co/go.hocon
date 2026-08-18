// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package json5 ingests JSON5 (https://json5.org, spec 1.0.0) as HOCON config.
//
//	base, _ := json5.ParseFile("settings.json5")
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// Unlike the jsonc package — which strips comments and hands the rest to
// encoding/json — JSON5 changes the token grammar itself (unquoted identifier
// keys, single-quoted strings with line continuations, hex integers, leading
// and trailing decimal points, an explicit plus sign), so this package is a
// hand-rolled scanner and recursive-descent parser. The Go JSON5 libraries
// are unmaintained, and text-normalising JSON5 into JSON would need the same
// tokenizer anyway. Zero dependencies, like every adapter in this module.
//
// The accepted grammar is JSON5 1.0.0 as defined by the reference
// implementation (the json5 npm package), the dialect owner this spec item
// tracks — the same ownership rule F3.2 applies to JSONC. Where the mapping
// spec is stricter than JSON5, the spec wins:
//
//   - Infinity and NaN (signed or bare) are errors, not values (spec F0.6).
//   - Integers — decimal or hex — must fit in int64 (spec F0.5); floats decode
//     as float64. A number written with '.', 'e' or 'E' is a float, all other
//     decimal forms and every hex form are integers.
//   - An unpaired \uXXXX surrogate is an error, and a valid pair combines into
//     the astral codepoint (spec F3.5).
//   - Duplicate keys follow HOCON semantics: objects merge, otherwise the
//     later value wins (spec F0.7).
//   - The document holds exactly one value; whitespace and comments may follow
//     it, anything else is an error (the F3.2 strictness rule).
//
// This package is part of the github.com/o3co/go.hocon/adapters module, which
// is versioned separately from the parser: see the module README for the
// go get line and the core-version requirement.
//
// See the F3.x items in the format-ingestion mapping spec:
// https://github.com/o3co/xx.hocon/blob/main/docs/format-ingestion-mapping.md
package json5

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/bom"
	"github.com/o3co/go.hocon/adapters/internal/tree"
)

// Parse reads JSON5 data. originDescription names the source in error
// messages; "" leaves it to hocon's default.
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	// F0.9: a leading BOM is not data. (JSON5 additionally treats U+FEFF as
	// whitespace anywhere, which the scanner handles; stripping here keeps
	// the origin column of the first token honest.)
	data = bom.Strip(data)
	p := &parser{src: string(data), origin: originDescription}
	doc, err := p.parseDocument()
	if err != nil {
		return nil, fmt.Errorf("json5: %s: %w", describe(originDescription), err)
	}
	nested, err := tree.Object(doc, scalar)
	if err != nil {
		return nil, fmt.Errorf("json5: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
}

// ParseFile reads path and parses it, using path as the origin description.
func ParseFile(path string) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("json5: %w", err)
	}
	return Parse(data, path)
}

func describe(origin string) string {
	if origin == "" {
		return "document"
	}
	return origin
}

// scalar maps a parsed leaf onto a value hocon.FromMap accepts. The parser
// only produces these types, so this is a type-preserving pass-through; it
// exists to satisfy tree.Object's contract.
func scalar(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string, int64, float64:
		return x, nil
	}
	return nil, fmt.Errorf("unsupported value of type %T", v)
}

// ---------------------------------------------------------------------------
// Scanner / parser
// ---------------------------------------------------------------------------

type parser struct {
	src    string
	pos    int // byte offset
	line   int // 0-based; reported 1-based
	lineAt int // byte offset where the current line starts
	origin string
}

func (p *parser) errf(format string, args ...any) error {
	col := utf8.RuneCountInString(p.src[p.lineAt:p.pos]) + 1
	return fmt.Errorf("line %d col %d: %s", p.line+1, col, fmt.Sprintf(format, args...))
}

// parseDocument parses exactly one JSON5 value, allowing only whitespace and
// comments after it (the F3.2 strictness rule).
func (p *parser) parseDocument() (any, error) {
	if err := p.skipSpace(); err != nil {
		return nil, err
	}
	v, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	if err := p.skipSpace(); err != nil {
		return nil, err
	}
	if p.pos < len(p.src) {
		return nil, p.errf("unexpected content after top-level value")
	}
	return v, nil
}

// isLineTerminator reports the JSON5 LineTerminator set: LF, CR, LS, PS.
// This deliberately differs from JSONC (F3.2), whose dialect owner ends //
// comments at LF/CR only — the JSON5 spec includes LS and PS.
func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029'
}

// isJSON5Space reports the JSON5 WhiteSpace set: TAB, VT, FF, SP, NBSP, BOM,
// and any Unicode Zs character.
func isJSON5Space(r rune) bool {
	switch r {
	case '\t', '\v', '\f', ' ', '\u00a0', '\ufeff':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// rune returns the rune at p.pos without advancing. Invalid UTF-8 is an
// error surfaced by the caller via ok=false with size 1 (RuneError).
func (p *parser) rune() (rune, int) {
	return utf8.DecodeRuneInString(p.src[p.pos:])
}

func (p *parser) advance(r rune, size int) {
	p.pos += size
	if isLineTerminator(r) {
		// Treat CRLF as one terminator for line counting.
		if r == '\r' && p.pos < len(p.src) && p.src[p.pos] == '\n' {
			p.pos++
		}
		p.line++
		p.lineAt = p.pos
	}
}

// skipSpace consumes whitespace, line terminators, and both comment forms.
func (p *parser) skipSpace() error {
	for p.pos < len(p.src) {
		r, size := p.rune()
		if r == utf8.RuneError && size == 1 {
			return p.errf("invalid UTF-8 byte 0x%02x", p.src[p.pos])
		}
		switch {
		case isJSON5Space(r) || isLineTerminator(r):
			p.advance(r, size)
		case r == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			p.pos += 2
			for p.pos < len(p.src) {
				r2, s2 := p.rune()
				if r2 == utf8.RuneError && s2 == 1 {
					return p.errf("invalid UTF-8 byte 0x%02x", p.src[p.pos])
				}
				if isLineTerminator(r2) {
					break
				}
				p.pos += s2
			}
		case r == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '*':
			p.pos += 2
			closed := false
			for p.pos < len(p.src) {
				r2, s2 := p.rune()
				if r2 == utf8.RuneError && s2 == 1 {
					return p.errf("invalid UTF-8 byte 0x%02x", p.src[p.pos])
				}
				if r2 == '*' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/' {
					p.pos += 2
					closed = true
					break
				}
				p.advance(r2, s2)
			}
			if !closed {
				return p.errf("unterminated /* comment")
			}
		default:
			return nil
		}
	}
	return nil
}

func (p *parser) parseValue() (any, error) {
	if p.pos >= len(p.src) {
		return nil, p.errf("unexpected end of input, expected a value")
	}
	r, _ := p.rune()
	switch {
	case r == '{':
		return p.parseObject()
	case r == '[':
		return p.parseArray()
	case r == '"' || r == '\'':
		return p.parseString(byte(r))
	case r == '+' || r == '-' || r == '.' || (r >= '0' && r <= '9'):
		return p.parseNumber()
	default:
		return p.parseKeyword()
	}
}

// parseKeyword handles true/false/null and rejects Infinity/NaN by name so
// the error explains itself (spec F0.6).
func (p *parser) parseKeyword() (any, error) {
	rest := p.src[p.pos:]
	for kw, v := range map[string]any{"true": true, "false": false, "null": nil} {
		if strings.HasPrefix(rest, kw) && !continuesIdentifier(rest, len(kw)) {
			p.pos += len(kw)
			return v, nil
		}
	}
	for _, kw := range []string{"Infinity", "NaN"} {
		if strings.HasPrefix(rest, kw) && !continuesIdentifier(rest, len(kw)) {
			return nil, p.errf("%s is not representable in the HOCON number model (spec F0.6)", kw)
		}
	}
	return nil, p.errf("unexpected character %q", firstRune(rest))
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

// continuesIdentifier reports whether s continues with an identifier
// character at byte offset i — used to reject tokens like `nullx`.
func continuesIdentifier(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return isIdentPart(r)
}

// ---------------------------------------------------------------------------
// Objects and arrays
// ---------------------------------------------------------------------------

func (p *parser) parseObject() (map[string]any, error) {
	p.pos++ // '{'
	obj := map[string]any{}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) {
			return nil, p.errf("unterminated object, expected '}'")
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return obj, nil
		}
		key, err := p.parseMemberName()
		if err != nil {
			return nil, err
		}
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return nil, p.errf("expected ':' after object key %q", key)
		}
		p.pos++
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		// F0.7: duplicate keys follow HOCON semantics — two objects merge,
		// any other combination is last-wins.
		if prev, dup := obj[key]; dup {
			if po, ok1 := prev.(map[string]any); ok1 {
				if vo, ok2 := val.(map[string]any); ok2 {
					val = mergeObjects(po, vo)
				}
			}
		}
		obj[key] = val
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) {
			return nil, p.errf("unterminated object, expected ',' or '}'")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++ // trailing comma before '}' is legal; loop handles it
		case '}':
			p.pos++
			return obj, nil
		default:
			return nil, p.errf("expected ',' or '}' in object")
		}
	}
}

// mergeObjects merges src over dst per HOCON duplicate-key semantics (F0.7),
// returning a new map.
func mergeObjects(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst)+len(src))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if prev, ok := out[k]; ok {
			if po, ok1 := prev.(map[string]any); ok1 {
				if vo, ok2 := v.(map[string]any); ok2 {
					out[k] = mergeObjects(po, vo)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

func (p *parser) parseArray() ([]any, error) {
	p.pos++ // '['
	arr := []any{}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) {
			return nil, p.errf("unterminated array, expected ']'")
		}
		if p.src[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) {
			return nil, p.errf("unterminated array, expected ',' or ']'")
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++ // trailing comma before ']' is legal; loop handles it
		case ']':
			p.pos++
			return arr, nil
		default:
			return nil, p.errf("expected ',' or ']' in array")
		}
	}
}

// ---------------------------------------------------------------------------
// Member names (quoted or ES5 IdentifierName)
// ---------------------------------------------------------------------------

func (p *parser) parseMemberName() (string, error) {
	r, _ := p.rune()
	if r == '"' || r == '\'' {
		return p.parseString(byte(r))
	}
	return p.parseIdentifier()
}

// isIdentStart / isIdentPart implement ES5 IdentifierName characters, the
// key grammar the JSON5 spec adopts: start = UnicodeLetter (Lu Ll Lt Lm Lo
// Nl) | '$' | '_'; part adds Mn Mc Nd Pc and ZWNJ/ZWJ.
func isIdentStart(r rune) bool {
	return r == '$' || r == '_' ||
		unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lm, unicode.Lo, unicode.Nl)
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || r == '\u200c' || r == '\u200d' ||
		unicode.In(r, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc)
}

// parseIdentifier scans an ES5 IdentifierName, honouring \uXXXX escapes in
// the name (the escaped codepoint must itself be a legal identifier
// character for its position, per ES5 — `1` cannot start a key).
func (p *parser) parseIdentifier() (string, error) {
	var sb strings.Builder
	first := true
	for p.pos < len(p.src) {
		r, size := p.rune()
		if r == utf8.RuneError && size == 1 {
			return "", p.errf("invalid UTF-8 byte 0x%02x", p.src[p.pos])
		}
		escaped := false
		if r == '\\' {
			if p.pos+1 >= len(p.src) || p.src[p.pos+1] != 'u' {
				return "", p.errf("only \\uXXXX escapes are allowed in identifiers")
			}
			p.pos += 2
			cp, err := p.readHex4()
			if err != nil {
				return "", err
			}
			r, escaped = cp, true
		}
		legal := isIdentPart(r)
		if first {
			legal = isIdentStart(r)
		}
		if !legal {
			if escaped {
				return "", p.errf("escape \\u%04X is not a valid identifier character here", r)
			}
			if first {
				return "", p.errf("expected an object key, got %q", r)
			}
			break
		}
		sb.WriteRune(r)
		if !escaped {
			p.pos += size
		}
		first = false
	}
	if sb.Len() == 0 {
		return "", p.errf("expected an object key")
	}
	return sb.String(), nil
}

// readHex4 reads exactly four hex digits at p.pos and returns the codepoint.
func (p *parser) readHex4() (rune, error) {
	if p.pos+4 > len(p.src) {
		return 0, p.errf("truncated \\u escape")
	}
	quad := p.src[p.pos : p.pos+4]
	n, err := strconv.ParseUint(quad, 16, 32)
	if err != nil {
		return 0, p.errf("invalid \\u escape %q", "\\u"+quad)
	}
	p.pos += 4
	return rune(n), nil
}

// ---------------------------------------------------------------------------
// Strings
// ---------------------------------------------------------------------------

// parseString scans a single- or double-quoted JSON5 string. quote is the
// opening quote byte. JSON5 differences from JSON: either quote character,
// \xHH escapes, \v, \0, line continuations (backslash before a line
// terminator, including CRLF as one), any other non-digit character escaping
// to itself, and unescaped LS/PS allowed inside the string.
func (p *parser) parseString(quote byte) (string, error) {
	p.pos++ // opening quote
	var sb strings.Builder
	for {
		if p.pos >= len(p.src) {
			return "", p.errf("unterminated string")
		}
		r, size := p.rune()
		if r == utf8.RuneError && size == 1 {
			return "", p.errf("invalid UTF-8 byte 0x%02x", p.src[p.pos])
		}
		switch {
		case byte(r) == quote && size == 1:
			p.pos++
			return sb.String(), nil
		case r == '\n' || r == '\r':
			return "", p.errf("unescaped line terminator in string")
		case r == '\\':
			p.pos++
			if err := p.readEscape(&sb); err != nil {
				return "", err
			}
		default:
			// LS/PS are legal unescaped inside JSON5 strings.
			sb.WriteRune(r)
			p.advance(r, size)
		}
	}
}

// readEscape consumes one escape sequence (the backslash is already
// consumed) and appends its value to sb.
func (p *parser) readEscape(sb *strings.Builder) error {
	if p.pos >= len(p.src) {
		return p.errf("unterminated escape sequence")
	}
	r, size := p.rune()
	if r == utf8.RuneError && size == 1 {
		return p.errf("invalid UTF-8 byte 0x%02x", p.src[p.pos])
	}
	// Line continuation: backslash before a line terminator joins the lines,
	// contributing nothing. CRLF counts as one terminator.
	if isLineTerminator(r) {
		p.advance(r, size)
		return nil
	}
	switch r {
	case 'n':
		sb.WriteByte('\n')
	case 't':
		sb.WriteByte('\t')
	case 'r':
		sb.WriteByte('\r')
	case 'b':
		sb.WriteByte('\b')
	case 'f':
		sb.WriteByte('\f')
	case 'v':
		sb.WriteByte('\v')
	case '0':
		// \0 is NUL unless followed by a decimal digit (octal escapes are
		// not part of JSON5).
		if p.pos+1 < len(p.src) && p.src[p.pos+1] >= '0' && p.src[p.pos+1] <= '9' {
			return p.errf("octal escape sequences are not allowed")
		}
		sb.WriteByte(0)
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return p.errf("escape \\%c is not allowed (digits cannot be escaped)", r)
	case 'x':
		p.pos++
		if p.pos+2 > len(p.src) {
			return p.errf("truncated \\x escape")
		}
		n, err := strconv.ParseUint(p.src[p.pos:p.pos+2], 16, 8)
		if err != nil {
			return p.errf("invalid \\x escape")
		}
		p.pos += 2
		sb.WriteRune(rune(n))
		return nil
	case 'u':
		p.pos++
		cp, err := p.readHex4()
		if err != nil {
			return err
		}
		// F3.5: a lone surrogate is an error; a valid pair combines.
		if utf16.IsSurrogate(cp) {
			if p.pos+6 <= len(p.src) && p.src[p.pos] == '\\' && p.src[p.pos+1] == 'u' {
				p.pos += 2
				lo, err := p.readHex4()
				if err != nil {
					return err
				}
				combined := utf16.DecodeRune(cp, lo)
				if combined == utf8.RuneError {
					return p.errf("unpaired \\u%04X surrogate (spec F3.5)", cp)
				}
				sb.WriteRune(combined)
				return nil
			}
			return p.errf("unpaired \\u%04X surrogate (spec F3.5)", cp)
		}
		sb.WriteRune(cp)
		return nil
	default:
		// Any other character escapes to itself (JSON5 SingleEscapeCharacter
		// and NonEscapeCharacter collapse to this rule).
		sb.WriteRune(r)
		p.advance(r, size)
		return nil
	}
	p.pos++ // the single-character escapes above
	return nil
}

// ---------------------------------------------------------------------------
// Numbers
// ---------------------------------------------------------------------------

// parseNumber scans a JSON5 numeric literal: optional sign, then a hex
// integer or a decimal with optional leading/trailing point and exponent.
// Signed Infinity/NaN are routed to the F0.6 error here.
func (p *parser) parseNumber() (any, error) {
	start := p.pos
	neg := false
	if c := p.src[p.pos]; c == '+' || c == '-' {
		neg = c == '-'
		p.pos++
	}
	rest := p.src[p.pos:]
	for _, kw := range []string{"Infinity", "NaN"} {
		if strings.HasPrefix(rest, kw) && !continuesIdentifier(rest, len(kw)) {
			return nil, p.errf("%s is not representable in the HOCON number model (spec F0.6)", kw)
		}
	}
	if strings.HasPrefix(rest, "0x") || strings.HasPrefix(rest, "0X") {
		p.pos += 2
		ds := p.pos
		for p.pos < len(p.src) && isHexDigit(p.src[p.pos]) {
			p.pos++
		}
		if p.pos == ds {
			return nil, p.errf("hex literal needs at least one digit")
		}
		mag, err := strconv.ParseUint(p.src[ds:p.pos], 16, 64)
		limit := uint64(1) << 63
		if !neg {
			limit--
		}
		if err != nil || mag > limit {
			return nil, p.errf("integer %s does not fit in int64 (spec F0.5)", p.src[start:p.pos])
		}
		if neg {
			if mag == 1<<63 {
				return int64(-1 << 63), nil
			}
			return -int64(mag), nil
		}
		return int64(mag), nil
	}

	sawDigit, sawDot, sawExp := false, false, false
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c == '.' && !sawDot && !sawExp:
			sawDot = true
		case (c == 'e' || c == 'E') && sawDigit && !sawExp:
			sawExp = true
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '+' || p.src[p.pos+1] == '-') {
				p.pos++
			}
		default:
			goto done
		}
		p.pos++
	}
done:
	text := p.src[start:p.pos]
	if !sawDigit {
		return nil, p.errf("malformed number %q", text)
	}
	// F0.5: '.', 'e', 'E' make a float; everything else is an int64 or an
	// error. strconv accepts leading '+' in both paths.
	if sawDot || sawExp {
		f, err := strconv.ParseFloat(strings.TrimPrefix(text, "+"), 64)
		if err != nil {
			return nil, p.errf("malformed number %q", text)
		}
		return f, nil
	}
	i, err := strconv.ParseInt(strings.TrimPrefix(text, "+"), 10, 64)
	if err != nil {
		return nil, p.errf("integer %s does not fit in int64 (spec F0.5)", text)
	}
	return i, nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
