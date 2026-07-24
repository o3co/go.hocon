// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package toml_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/toml"
)

func parse(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := toml.Parse([]byte(src), "test.toml")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return cfg
}

func TestScalarsTablesAndArrays(t *testing.T) {
	cfg := parse(t, `
name = "svc"
port = 8080
ratio = 0.5
debug = true
tags = ["a", "b"]

[db]
host = "localhost"

[[db.replicas]]
id = 1

[[db.replicas]]
id = 2
`)

	if got := cfg.GetString("name"); got != "svc" {
		t.Errorf("name = %q, want svc", got)
	}
	if got := cfg.GetInt64("port"); got != 8080 {
		t.Errorf("port = %d, want 8080", got)
	}
	if !cfg.GetBool("debug") {
		t.Error("debug = false, want true")
	}
	if got := cfg.GetStringSlice("tags"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("tags = %#v, want [a b]", got)
	}
	if got := cfg.GetString("db.host"); got != "localhost" {
		t.Errorf("db.host = %q, want localhost", got)
	}
	replicas := cfg.GetConfigSlice("db.replicas")
	if len(replicas) != 2 || replicas[1].GetInt64("id") != 2 {
		t.Errorf("db.replicas did not become a list of objects: %#v", replicas)
	}
}

func TestDottedKeysNest(t *testing.T) {
	cfg := parse(t, "a.b.c = 1\n")
	if got := cfg.GetInt64("a.b.c"); got != 1 {
		t.Errorf("a.b.c = %d, want 1", got)
	}
}

// F4.2: HOCON has no datetime, so all four TOML date-time types become their
// RFC 3339 string forms rather than a lossy number.
func TestDateTimesBecomeStrings(t *testing.T) {
	cfg := parse(t, `
offset = 1979-05-27T07:32:00Z
local_dt = 1979-05-27T07:32:00
local_d = 1979-05-27
local_t = 07:32:00
`)
	for path, want := range map[string]string{
		"offset":   "1979-05-27T07:32:00Z",
		"local_dt": "1979-05-27T07:32:00",
		"local_d":  "1979-05-27",
		"local_t":  "07:32:00",
	} {
		if got := cfg.GetString(path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

// F0.6: TOML can write inf and nan; HOCON cannot represent them.
func TestInfinityAndNaNRejected(t *testing.T) {
	for _, src := range []string{"x = inf", "x = -inf", "x = nan"} {
		t.Run(src, func(t *testing.T) {
			_, err := toml.Parse([]byte(src), "test.toml")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
			if !strings.Contains(err.Error(), "F0.6") {
				t.Errorf("error %q does not cite the spec item F0.6", err)
			}
		})
	}
}

// The error should say where in the document the problem is.
func TestErrorNamesThePath(t *testing.T) {
	_, err := toml.Parse([]byte("[db]\nlimits = [1.0, inf]\n"), "test.toml")
	if err == nil {
		t.Fatal("nan/inf inside an array accepted, want error")
	}
	if !strings.Contains(err.Error(), "db.limits[1]") {
		t.Errorf("error %q does not name the offending path db.limits[1]", err)
	}
}

func TestSyntaxErrorRejected(t *testing.T) {
	if _, err := toml.Parse([]byte("this is not toml"), "test.toml"); err == nil {
		t.Fatal("malformed TOML accepted, want error")
	}
}

// F0.2: a ${...} in foreign data is text, not a reference.
func TestSubstitutionSyntaxStaysLiteral(t *testing.T) {
	cfg := parse(t, `a = "${foo.bar}"`)
	resolved, err := cfg.Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != "${foo.bar}" {
		t.Errorf("a = %q, want the literal ${foo.bar}", got)
	}
}

func TestUseAsSubstitutionSourceUnderHOCON(t *testing.T) {
	base, err := toml.Parse([]byte("[project]\nname = \"svc\"\nversion = \"1.4.0\"\n"), "pyproject.toml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg, err := hocon.ParseStringWithOptions(
		`image = "registry.example.com/"${project.name}":"${project.version}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := merged.GetString("image"); got != "registry.example.com/svc:1.4.0" {
		t.Errorf("image = %q, want registry.example.com/svc:1.4.0", got)
	}
}
