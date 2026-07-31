// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keypath_test

import (
	"testing"

	"github.com/o3co/go.hocon/adapters/internal/keypath"
)

// The rendering is a cross-implementation contract: py.hocon and rs.hocon
// produce the same text, and the xx.hocon collision fixtures compare it.
func TestRender(t *testing.T) {
	for name, tc := range map[string]struct {
		path []string
		want string
	}{
		"simple":          {[]string{"db", "host"}, "db.host"},
		"digits and dash": {[]string{"a-1", "2"}, "a-1.2"},
		"underscore":      {[]string{"max_conn"}, "max_conn"},
		"literal dot":     {[]string{"foo.bar"}, `"foo.bar"`},
		"space":           {[]string{"a b"}, `"a b"`},
		"empty segment":   {[]string{""}, `""`},
		"non-ascii":       {[]string{"é"}, `"é"`},
		"quote inside":    {[]string{`a"b`}, `"a\"b"`},
		"nothing":         {nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := keypath.Render(tc.path); got != tc.want {
				t.Errorf("Render(%q) = %s, want %s", tc.path, got, tc.want)
			}
		})
	}
}

// A dotted segment must not render like two segments — the whole point.
func TestDottedSegmentIsDistinguishable(t *testing.T) {
	if a, b := keypath.Render([]string{"foo.bar"}), keypath.Render([]string{"foo", "bar"}); a == b {
		t.Errorf("one segment and two render alike: %s", a)
	}
}

// A quoted segment is a JSON string literal — which is also HOCON's own
// quoted-string syntax — so that the same key renders the same way in
// go.hocon, ts.hocon, py.hocon and rs.hocon (spec F0.10). %q would spell NUL
// as \x00, which is Go syntax and which no sibling produces.
func TestSegmentRendersAsJSONString(t *testing.T) {
	cases := []struct{ seg, want string }{
		{"ab", "ab"},
		{"a-b_1", "a-b_1"},
		{"", `""`},
		{"a b", `"a b"`},
		{"a.b", `"a.b"`},
		{"a\nb", `"a\nb"`},
		{"a\tb", `"a\tb"`},
		{"a\rb", `"a\rb"`},
		{"a\x00b", `"a\u0000b"`},
		{"a\x1fb", `"a\u001fb"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		// Printable non-ASCII stays itself: F1.3 leaves these unfolded, so they
		// arrive here in normal use and escaping them would bury the common case.
		{"\u0130a", "\"\u0130a\""},
		{"a\u00e9b", "\"a\u00e9b\""},
	}
	for _, c := range cases {
		if got := keypath.Segment(c.seg); got != c.want {
			t.Errorf("keypath.Segment(%q) = %s, want %s", c.seg, got, c.want)
		}
	}
}

// U+2028 and U+2029 are legal raw in JSON, and escaped anyway: they are line
// separators to enough log viewers and editors that letting one through would
// let a key break the message reporting it.
func TestSegmentEscapesLineSeparators(t *testing.T) {
	if got, want := keypath.Segment("a\u2028b"), `"a\u2028b"`; got != want {
		t.Errorf("U+2028: got %s, want %s", got, want)
	}
	if got, want := keypath.Segment("a\u2029b"), `"a\u2029b"`; got != want {
		t.Errorf("U+2029: got %s, want %s", got, want)
	}
}

// A byte that is not valid UTF-8 decodes to U+FFFD. Spelling it out beats
// emitting a replacement character the reader cannot tell from one the key
// really contained.
func TestSegmentSpellsOutInvalidUTF8(t *testing.T) {
	if got, want := keypath.Segment("\xff"), `"\ufffd"`; got != want {
		t.Errorf("invalid UTF-8: got %s, want %s", got, want)
	}
}

// Two different paths must never render alike, or a collision report cannot be
// acted on. This is the property an error message actually needs — pasting the
// result into a getter is NOT guaranteed, because HOCON's path parser does not
// decode escapes inside a quoted segment.
func TestDistinctPathsRenderDistinctly(t *testing.T) {
	seen := map[string][]string{}
	paths := [][]string{
		{"foo.bar"}, {"foo", "bar"},
		{"a\nb"}, {"a", "b"}, {`a\nb`},
		{"a\x00b"}, {`a\u0000b`},
		{""}, {"", ""},
	}
	for _, p := range paths {
		r := keypath.Render(p)
		if prev, dup := seen[r]; dup {
			t.Errorf("%q and %q both render as %s", prev, p, r)
		}
		seen[r] = p
	}
}
