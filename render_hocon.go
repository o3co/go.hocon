// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/o3co/go.hocon/internal/resolver"
)

// RenderHOCON renders a resolved Config as HOCON text.
//
// The output round-trips: parsing it back yields the same value tree. That is
// the correctness contract, not byte-for-byte formatting — a scalar is quoted
// whenever leaving it bare would re-parse as a different type (a string "8080"
// becomes "8080", not 8080), and left bare only when it provably cannot.
//
// The Config must be resolved and hold only data (objects, arrays, string /
// number / boolean / null scalars) — exactly what FromMap and the format
// adapters produce. An unresolved placeholder is an error; substitutions have
// no textual round trip through a value tree.
//
// The root object's fields are emitted without enclosing braces, nested
// objects as `key { … }`, arrays as newline-separated `[ … ]`, indented two
// spaces. Source comments are not represented — a value tree does not carry
// them.
func (c *Config) RenderHOCON() (string, error) {
	var b strings.Builder
	if err := renderObjectBody(&b, c.root, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

func renderObjectBody(b *strings.Builder, o *resolver.ObjectVal, depth int) error {
	indent := strings.Repeat("  ", depth)
	for _, k := range o.Keys() {
		// Keys() lists only present keys, so Get always succeeds; a nil here
		// would fall through to renderValue's error rather than be skipped.
		v, _ := o.Get(k)
		b.WriteString(indent)
		b.WriteString(renderKey(k))
		switch child := v.(type) {
		case *resolver.ObjectVal:
			b.WriteString(" {")
			if len(child.Keys()) == 0 {
				b.WriteString("}\n")
				continue
			}
			b.WriteString("\n")
			if err := renderObjectBody(b, child, depth+1); err != nil {
				return err
			}
			b.WriteString(indent)
			b.WriteString("}\n")
		default:
			b.WriteString(" = ")
			if err := renderValue(b, v, depth); err != nil {
				return err
			}
			b.WriteString("\n")
		}
	}
	return nil
}

func renderValue(b *strings.Builder, v resolver.Val, depth int) error {
	switch x := v.(type) {
	case *resolver.ObjectVal:
		if len(x.Keys()) == 0 {
			b.WriteString("{}")
			return nil
		}
		b.WriteString("{\n")
		if err := renderObjectBody(b, x, depth+1); err != nil {
			return err
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteString("}")
		return nil
	case *resolver.ArrayVal:
		return renderArray(b, x, depth)
	case *resolver.ScalarVal:
		b.WriteString(renderScalar(x))
		return nil
	}
	return fmt.Errorf("RenderHOCON: unrenderable value %T (config must be resolved data)", v)
}

func renderArray(b *strings.Builder, a *resolver.ArrayVal, depth int) error {
	if len(a.Elements) == 0 {
		b.WriteString("[]")
		return nil
	}
	inner := strings.Repeat("  ", depth+1)
	b.WriteString("[\n")
	for _, e := range a.Elements {
		b.WriteString(inner)
		if err := renderValue(b, e, depth+1); err != nil {
			return err
		}
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("]")
	return nil
}

func renderScalar(s *resolver.ScalarVal) string {
	switch s.Type {
	case resolver.ScalarNull:
		return "null"
	case resolver.ScalarBoolean, resolver.ScalarNumber:
		// Raw already holds the canonical textual form; both re-parse to their
		// own type, so they are emitted bare.
		return s.Raw
	default:
		return renderString(s.Raw)
	}
}

// safeUnquotedKey matches a key that is unambiguous unquoted: no dot (which
// would nest), no whitespace, no forbidden character.
var safeUnquotedKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func renderKey(k string) string {
	// `include` is reserved unquoted at the start of a key (S12.5/S14a) — the
	// parser (and Lightbend: "include keyword is not followed by a quoted
	// string") rejects `include = 1`, so the key must be quoted to round-trip.
	//
	// A quoted key always takes the escaped form, never triple quotes (E18
	// decision 3): the rs, ts and py parsers reject a triple-quoted key, so a
	// key containing LF would not round-trip there.
	if k == "include" {
		return escapeQuoted(k)
	}
	if safeUnquotedKey.MatchString(k) {
		return k
	}
	return escapeQuoted(k)
}

// safeBareString matches a string value that cannot be misread as another
// type: an identifier that is not a boolean/null keyword and not numeric. Any
// other string is quoted, which always round-trips.
var safeBareString = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

var stringKeywords = map[string]bool{
	"true": true, "false": true, "null": true,
	"yes": true, "no": true, "on": true, "off": true,
}

func renderString(s string) string {
	if safeBareString.MatchString(s) && !stringKeywords[strings.ToLower(s)] {
		return s
	}
	return quoteString(s)
}

func quoteString(s string) string {
	// A string containing newlines is triple-quoted when that is unambiguous
	// and lossless: no embedded `"""`, no trailing `"`, and no C0 control
	// other than LF and TAB (E18 decision 5). Raw controls are valid inside
	// triple quotes (S9.1, and the lexer preserves CR verbatim per S9.2), but
	// an invisible one is ambiguous to readers, so such strings take the
	// escaped double-quoted form below.
	if strings.Contains(s, "\n") && !hasControlOtherThanLFTab(s) &&
		!strings.Contains(s, `"""`) && !strings.HasSuffix(s, `"`) {
		return `"""` + s + `"""`
	}
	return escapeQuoted(s)
}

// escapeQuoted writes s as a double-quoted string with JSON escapes.
func escapeQuoted(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			// S1.2.4: a raw C0 control inside quotes is a parse error, so
			// every other one is written as \u00xx (lowercase, as Lightbend's
			// renderer does). DEL and C1 are legal and stay verbatim.
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// hasControlOtherThanLFTab reports whether s holds a C0 control character
// (U+0000–U+001F) other than LF and TAB.
func hasControlOtherThanLFTab(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			return true
		}
	}
	return false
}
