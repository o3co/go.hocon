package resolver

import (
	"testing"
)

func TestParseSubstPath(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a.b.c", []string{"a", "b", "c"}},
		{`"a.b".c`, []string{"a.b", "c"}},
		{`"".foo`, []string{"", "foo"}},
		{"x", []string{"x"}},
		{`"a.b"."c.d"`, []string{"a.b", "c.d"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSubstPath(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("parseSubstPath(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("parseSubstPath(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSegmentsToKey(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"a", "b", "c"}, "a.b.c"},
		{[]string{"a.b", "c"}, `"a.b".c`},
		{[]string{"", "foo"}, `"".foo`},
		{[]string{"x"}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := segmentsToKey(tt.input)
			if got != tt.expected {
				t.Fatalf("segmentsToKey(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRoundtrip(t *testing.T) {
	cases := [][]string{
		{"a", "b"},
		{"a.b", "c"},
		{"", "x", ""},
		{"a.b.c", "d.e"},
	}
	for _, segs := range cases {
		key := segmentsToKey(segs)
		got := parseSubstPath(key)
		if len(got) != len(segs) {
			t.Fatalf("roundtrip %v → %q → %v", segs, key, got)
		}
		for i := range got {
			if got[i] != segs[i] {
				t.Fatalf("roundtrip %v → %q → %v", segs, key, got)
			}
		}
	}
}
