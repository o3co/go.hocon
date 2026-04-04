package hocon_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

func TestParseError_Error(t *testing.T) {
	err := &hocon.ParseError{Message: "unexpected token", Line: 3, Col: 5}
	got := err.Error()
	if got == "" {
		t.Fatal("Error() returned empty string")
	}
	// must contain line info
	if got != "parse error at line 3, col 5: unexpected token" {
		t.Errorf("unexpected format: %q", got)
	}
}

func TestResolveError_Error(t *testing.T) {
	err := &hocon.ResolveError{Message: "circular reference", Path: "a.b"}
	got := err.Error()
	if got == "" {
		t.Fatal("Error() returned empty string")
	}
}

func TestConfigError_Error(t *testing.T) {
	err := &hocon.ConfigError{Message: "missing key", Path: "server.host"}
	got := err.Error()
	if got == "" {
		t.Fatal("Error() returned empty string")
	}
}

func TestParseError_IsError(t *testing.T) {
	pe := &hocon.ParseError{Message: "oops"}
	var err error = pe
	var target *hocon.ParseError
	if !errors.As(err, &target) {
		t.Fatal("errors.As failed for ParseError")
	}
}

func TestParseString_ErrorHasLineCol(t *testing.T) {
	// Trigger a parse error and verify the public ParseError has Line/Col populated.
	_, err := hocon.ParseString("{ a = 1")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *hocon.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *hocon.ParseError, got %T", err)
	}
	if pe.Line == 0 {
		t.Error("expected Line > 0 in ParseError")
	}
	if pe.Col == 0 {
		t.Error("expected Col > 0 in ParseError")
	}
	// Message should contain only the description, not the "parse error at line..." prefix.
	if strings.Contains(pe.Message, "parse error at") {
		t.Errorf("Message should not contain 'parse error at' prefix, got: %s", pe.Message)
	}
}
