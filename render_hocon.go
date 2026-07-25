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
	if safeUnquotedKey.MatchString(k) {
		return k
	}
	return quoteString(k)
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
	// and lossless: no embedded `"""`, no trailing `"`, and no carriage return
	// (the parser normalizes CRLF inside triple quotes, which would drop the
	// `\r`, so those fall through to escaped double quotes below).
	if strings.Contains(s, "\n") && !strings.Contains(s, "\r") &&
		!strings.Contains(s, `"""`) && !strings.HasSuffix(s, `"`) {
		return `"""` + s + `"""`
	}
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
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
