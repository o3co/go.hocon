// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// includeProperties writes props to a .properties file, includes it from a
// HOCON document, and returns the parsed result.
func includeProperties(t *testing.T, props string) *hocon.Config {
	t.Helper()
	dir := t.TempDir()
	propsFile := filepath.Join(dir, "app.properties")
	if err := os.WriteFile(propsFile, []byte(props), 0o600); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "main.conf")
	slashed := strings.ReplaceAll(propsFile, `\`, "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include %q\n", slashed)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return cfg
}

// Lightbend hands an included .properties file to java.util.Properties, so
// include has to accept the whole of that syntax. Until 2026-07 this parser
// implemented roughly the "key=value with # comments" subset, and every case
// below produced something wrong — a dropped continuation line, a key split at
// an escaped separator, an escape left as literal backslash text.
func TestIncludePropertiesFullSyntax(t *testing.T) {
	cfg := includeProperties(t,
		"a = one\\\n"+ // backslash continuation
			"two\n"+
			"b\\:c = 2\n"+ // escaped separator belongs to the key
			"d = x\\ty\n"+ // escape sequence
			"e = \\u00e9\n"+ // unicode escape
			"f value = 3\n"+ // whitespace alone separates
			"g\\ h = 4\n") // escaped space belongs to the key

	for path, want := range map[string]string{
		"a":   "onetwo",
		"b:c": "2",
		"d":   "x\ty",
		"e":   "é",
		"f":   "value = 3",
		"g h": "4",
	} {
		got, ok := cfg.GetStringOption(path).Get()
		if !ok {
			t.Errorf("path %q missing", path)
			continue
		}
		if got != want {
			t.Errorf("path %q = %q, want %q", path, got, want)
		}
	}
}

// Java skips whitespace before a value but not after it.
func TestIncludePropertiesKeepsTrailingWhitespace(t *testing.T) {
	cfg := includeProperties(t, "key  =  value  \n")
	if got := cfg.GetString("key"); got != "value  " {
		t.Errorf("key = %q, want %q", got, "value  ")
	}
}

// A continuation line starting with '#' is value text, not a comment: comment
// status is decided before continuations are joined.
func TestIncludePropertiesContinuationIntoHash(t *testing.T) {
	cfg := includeProperties(t, "a = one\\\n#two\n")
	if got := cfg.GetString("a"); got != "one#two" {
		t.Errorf("a = %q, want %q", got, "one#two")
	}
}

// Dotted keys still nest, and a key that is also a parent loses its scalar.
func TestIncludePropertiesNestingAndObjectsWin(t *testing.T) {
	cfg := includeProperties(t, "a = 1\na.b = 2\nc.d.e = 3\n")
	if got := cfg.GetString("a.b"); got != "2" {
		t.Errorf("a.b = %q, want 2", got)
	}
	if got := cfg.GetString("c.d.e"); got != "3" {
		t.Errorf("c.d.e = %q, want 3", got)
	}
	if cfg.GetStringOption("a").IsSome() {
		t.Error("scalar at `a` survived, want the object to win")
	}
}

// A malformed escape is reported rather than silently mangled.
func TestIncludePropertiesMalformedEscapeIsError(t *testing.T) {
	dir := t.TempDir()
	propsFile := filepath.Join(dir, "app.properties")
	if err := os.WriteFile(propsFile, []byte(`a = \ud83d`), 0o600); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "main.conf")
	slashed := strings.ReplaceAll(propsFile, `\`, "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include %q\n", slashed)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hocon.ParseFile(mainFile); err == nil {
		t.Fatal("unpaired surrogate accepted, want error")
	}
}
