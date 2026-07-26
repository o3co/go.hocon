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
// The exception is a key holding control characters: Segment quotes with %q,
// which spells them as Go escapes (\n, \x00), and HOCON's path parser does not
// interpret those. Such a path still reads unambiguously, which is what an
// error message needs, but it will not round-trip through a getter verbatim.
package keypath

import (
	"fmt"
	"strings"
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
	return fmt.Sprintf("%q", seg)
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
