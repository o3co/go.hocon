// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// E18 — the shared emitter round-trip corpus (xx.hocon
// testdata/emitter-roundtrip/, synced by `make testdata`). Each fixture is a
// JSON value tree; the contract is parse(render(tree)) == tree, compared as
// trees, never as text. See xx.hocon docs/extra-spec-conventions.md §E18.
func TestE18EmitterRoundTripCorpus(t *testing.T) {
	dir := filepath.Join("testdata", "emitter-roundtrip")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("emitter-roundtrip corpus not synced — run `make testdata`")
	}
	ran := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ran++
		t.Run(strings.TrimSuffix(e.Name(), ".json"), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // fixture path
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			dec := json.NewDecoder(strings.NewReader(string(raw)))
			dec.UseNumber()
			var tree map[string]any
			if err := dec.Decode(&tree); err != nil {
				t.Fatalf("fixture is not a JSON object: %v", err)
			}
			if tree == nil {
				// Decode accepts a top-level `null` into a nil map without
				// error; a fixture must be a real object.
				t.Fatal("fixture is JSON null, not an object")
			}
			cfg, err := hocon.FromMap(normalizeNumbers(tree).(map[string]any), e.Name())
			if err != nil {
				t.Fatalf("FromMap: %v", err)
			}
			before, err := cfg.RenderJSONForTest()
			if err != nil {
				t.Fatalf("RenderJSONForTest(before): %v", err)
			}
			text, err := cfg.RenderHOCON()
			if err != nil {
				t.Fatalf("RenderHOCON: %v", err)
			}
			reparsed, err := hocon.ParseString(text)
			if err != nil {
				t.Fatalf("re-parse of emitted HOCON failed: %v\n--- emitted ---\n%s", err, text)
			}
			after, err := reparsed.RenderJSONForTest()
			if err != nil {
				t.Fatalf("RenderJSONForTest(after): %v", err)
			}
			if before != after {
				t.Errorf("round trip changed the tree\n  before: %s\n  after:  %s\n--- emitted ---\n%s", before, after, text)
			}
		})
	}
	if ran == 0 {
		t.Fatal("corpus directory exists but holds no fixtures")
	}
}

// normalizeNumbers converts json.Number leaves into int64 when the source
// wrote an integer and float64 otherwise, the value split FromMap expects
// (and the same rule the adapters use, spec F0.5).
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalizeNumbers(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeNumbers(e)
		}
		return out
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			if i, err := x.Int64(); err == nil {
				return i
			}
			// An integer lexeme past int64: keep the json.Number so FromMap
			// rejects it loudly instead of a silent precision-losing float.
			// (The shared corpus stays within 2^53 by convention — E18.)
			return x
		}
		f, _ := x.Float64()
		return f
	}
	return v
}
