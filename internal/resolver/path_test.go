package resolver

import (
	"testing"
)

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

func TestSegmentsToKey_QuotesWhitespace(t *testing.T) {
	got := segmentsToKey([]string{" a ", "b"})
	expected := `" a ".b`
	if got != expected {
		t.Fatalf("segmentsToKey whitespace: got %q, want %q", got, expected)
	}
}

func TestSegmentsToKey_EscapedQuotes(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{`a"b`, "c"}, `"a\"b".c`},
		{[]string{`a\b`, "c"}, `"a\\b".c`},
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
