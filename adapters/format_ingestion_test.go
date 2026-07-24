// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Conformance against the shared format-ingestion fixtures from xx.hocon.
//
// These expectations are not oracle-generated — Lightbend has no equivalent of
// these adapters — so they encode the project's own F-item decisions. Their
// value is cross-implementation: all four must agree with them and with each
// other. See testdata-format-ingestion/manifest.json.

package adapters_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/env"
	"github.com/o3co/go.hocon/adapters/jsonc"
	"github.com/o3co/go.hocon/adapters/toml"
	"github.com/o3co/go.hocon/adapters/yaml"
)

const fixtureRoot = "testdata-format-ingestion"

type manifest struct {
	Cases []struct {
		ID       string `json:"id"`
		Format   string `json:"format"`
		Input    string `json:"input"`
		Kind     string `json:"kind"`
		Expect   string `json:"expect"`
		Expected string `json:"expected"`
		Cites    string `json:"cites"`
		Note     string `json:"note"`
	} `json:"cases"`
}

// envFixture is the shape of an `env-vars` input: the prefix to mount and the
// variables to mount it from, so the case needs no real environment.
type envFixture struct {
	Prefix string            `json:"prefix"`
	Vars   map[string]string `json:"vars"`
}

func TestFormatIngestionFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixtureRoot, "manifest.json"))
	if err != nil {
		t.Skipf("format-ingestion fixtures missing: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(m.Cases) == 0 {
		t.Fatal("manifest lists no cases")
	}

	for _, c := range m.Cases {
		t.Run(c.ID, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(fixtureRoot, c.Input))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			cfg, err := ingest(c.Format, c.Kind, data, c.ID)

			if c.Expect == "error" {
				if err == nil {
					t.Fatalf("%s: succeeded, want error (%s)", c.ID, c.Note)
				}
				if c.Cites != "" && !strings.Contains(err.Error(), c.Cites) {
					t.Errorf("error %q does not mention %q", err, c.Cites)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: %v", c.ID, err)
			}

			wantRaw, err := os.ReadFile(filepath.Join(fixtureRoot, c.Expected))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			var want any
			if err := json.Unmarshal(wantRaw, &want); err != nil {
				t.Fatalf("expected JSON: %v", err)
			}

			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !jsonEqual(got, want) {
				g, _ := json.MarshalIndent(got, "", "  ")
				w, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("%s mismatch (%s)\ngot:\n%s\nwant:\n%s", c.ID, c.Note, g, w)
			}
		})
	}
}

func ingest(format, kind string, data []byte, origin string) (*hocon.Config, error) {
	switch format {
	case "jsonc":
		return jsonc.Parse(data, origin)
	case "toml":
		return toml.Parse(data, origin)
	case "yaml":
		return yaml.Parse(data, origin)
	case "env":
		if kind == "dotenv" {
			return env.Parse(data, env.Options{Origin: origin})
		}
		var f envFixture
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		environ := make([]string, 0, len(f.Vars))
		for k, v := range f.Vars {
			environ = append(environ, k+"="+v)
		}
		return env.Load(env.Options{Prefix: f.Prefix, Environ: environ, Origin: origin})
	}
	panic("unknown format " + format)
}

// jsonEqual compares two decoded JSON trees, treating numbers by value so an
// int64 and a float64 holding the same number agree.
func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(normalizeJSON(a))
	jb, _ := json.Marshal(normalizeJSON(b))
	return string(ja) == string(jb)
}

func normalizeJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalizeJSON(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeJSON(e)
		}
		return out
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return v
}
