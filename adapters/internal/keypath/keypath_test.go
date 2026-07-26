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
