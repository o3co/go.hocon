// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package keypath renders a path — a list of key segments — as the HOCON path
// expression that addresses it.
//
// Adapters report paths in errors, and a path is a list of segments, not a
// string: joining on "." alone makes the single key "foo.bar" (one segment
// holding a dot) indistinguishable from foo -> bar (two segments), which are
// different places in the config. Quoting the segments that need it removes
// the ambiguity, and for ordinary keys the result is also the expression a
// reader can paste into a getter.
//
// A quoted segment is spelled as a JSON string literal, which is also HOCON's
// own quoted-string syntax, with two deliberate departures from Go's %q:
//
//   - NUL is \u0000, not \x00. %q's hex escapes are Go syntax; JSON has no
//     \x form, and py.hocon and rs.hocon render the same segment the same way.
//   - U+2028 and U+2029 are escaped, though JSON permits them raw. They are
//     line separators to many log viewers and editors, and the point of
//     escaping at all is that a key cannot break the message it appears in.
//
// Printable non-ASCII is left as itself (é, İ), rather than escaped: F1.3
// leaves non-ASCII segments unfolded, so they reach here in normal use and
// escaping them would make the common case unreadable.
//
// A rendered path is NOT guaranteed to paste into a getter. For ordinary keys
// it does; for a segment needing escapes it does not, because HOCON's path
// parser does not decode escapes inside a quoted segment — measured, and true
// of all three implementations. What an error message needs is that two
// different paths never render alike, and that holds.
package keypath

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Render joins segments with "." and quotes the ones that could not be written
// bare in a HOCON path expression.
//
//	[]string{"db", "host"}  ->  db.host
//	[]string{"foo.bar"}     ->  "foo.bar"
//	[]string{"a b"}         ->  "a b"
func Render(path []string) string {
	var b strings.Builder
	for i, seg := range path {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(Segment(seg))
	}
	return b.String()
}

// Segment renders one path segment, quoting it unless it is bare-safe.
func Segment(seg string) string {
	if bare(seg) {
		return seg
	}
	var b strings.Builder
	b.Grow(len(seg) + 2)
	b.WriteByte('"')
	for _, r := range seg {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\u2028', '\u2029':
			// Valid raw in JSON, but a line separator to enough readers that
			// letting it through would let a key break its own error message.
			fmt.Fprintf(&b, `\u%04x`, r)
		case utf8.RuneError:
			// What a byte that is not valid UTF-8 decodes to. Spelling it out
			// beats emitting a replacement character the reader cannot tell
			// from one the key really contained.
			b.WriteString(`\ufffd`)
		default:
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

// bare reports whether a segment can be written without quotes: a non-empty
// run of ASCII letters, digits, "_" or "-".
//
// The rule is deliberately narrower than HOCON's unquoted-key grammar. Erring
// toward quoting keeps the output unambiguous — and it is the rule py.hocon
// and rs.hocon render with, which matters because these messages are compared
// in the cross-language fixtures. (Their form is [a-z0-9_-]; env segments are
// lowercased before they get here, so admitting A-Z as well changes nothing
// for env and lets other adapters share the function.)
func bare(seg string) bool {
	if seg == "" {
		return false
	}
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}
