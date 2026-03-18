package hocon_test

import (
	"errors"
	"testing"

	"github.com/o3co/go.hocon/hocon"
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
	var pe *hocon.ParseError = &hocon.ParseError{Message: "oops"}
	var err error = pe
	var target *hocon.ParseError
	if !errors.As(err, &target) {
		t.Fatal("errors.As failed for ParseError")
	}
}
