// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package adapters_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/o3co/go.hocon"
)

const jsonDoc = `{
  "name": "svc",
  "port": 8080,
  "ratio": 0.5,
  "debug": true,
  "absent": null,
  "tags": ["a", "b"],
  "db": { "host": "localhost", "replicas": [{ "id": 1 }, { "id": 2 }] }
}`

// F3.1 claims plain JSON needs no adapter because HOCON is a JSON superset.
// This keeps the claim honest rather than asserted.
func TestPlainJSONParsesAsHOCON(t *testing.T) {
	cfg, err := hocon.ParseString(jsonDoc)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}

	if got := cfg.GetString("name"); got != "svc" {
		t.Errorf(`name = %q, want "svc"`, got)
	}
	if got := cfg.GetInt64("port"); got != 8080 {
		t.Errorf("port = %d, want 8080", got)
	}
	if got := cfg.GetString("db.host"); got != "localhost" {
		t.Errorf(`db.host = %q, want "localhost"`, got)
	}
	if !cfg.GetBool("debug") {
		t.Error("debug = false, want true")
	}
	if got := cfg.GetStringSlice("tags"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("tags = %#v, want [a b]", got)
	}
	if got := cfg.GetConfigSlice("db.replicas"); len(got) != 2 || got[1].GetInt64("id") != 2 {
		t.Errorf("db.replicas did not round-trip as a list of objects: %#v", got)
	}
}

// Pinned, because an adapter must not change what a null means.
//
// Has reports true for a null value. That matches all four o3co
// implementations but not Lightbend, whose hasPath returns false for null and
// which offers hasPathOrNull for the other behaviour. Tracked as an open
// cross-impl question in docs/TODO.md.
func TestJSONNullBehaviour(t *testing.T) {
	cfg, err := hocon.ParseString(jsonDoc)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if !cfg.Has("absent") {
		t.Error("Has(absent) = false; the o3co implementations report true for a null value")
	}
	if cfg.GetStringOption("absent").IsSome() {
		t.Error("GetStringOption(absent) returned a value, want none")
	}
}

func TestJSONFileParsesAsHOCON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(jsonDoc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := hocon.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := cfg.GetString("name"); got != "svc" {
		t.Errorf(`name = %q, want "svc"`, got)
	}
}
