// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon"
)

// go.hocon#131 (Lightbend parity): explicit null values contributed by a
// fallback config must remain visible keys in the resolved object. The resolver,
// the merge (MergeUnresolved), and renderJSON all preserved the top-level
// `value = null` already; the defect was Unmarshal-only — for a map[string]any
// target, valToAny returns an untyped Go nil, reflect.ValueOf(nil) is an invalid
// Value, and SetMapIndex with an invalid value DELETES the entry rather than
// storing key→nil. Nested nulls survived because valToAny builds nested maps via
// plain `m[k] = nil` assignment (no reflect.SetMapIndex). The fix stores a typed
// zero of the element type for nils.
func TestIssue131_NullFallbackPreservedInUnmarshalMap(t *testing.T) {
	source := `
variables {
  value = null
}

environment {
  value = ${value}
}
`
	cfg, err := hocon.ParseStringWithOptions(source, hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.WithFallback(cfg.GetConfig("variables")).
		Resolve(hocon.DefaultResolveOptions().WithUseSystemEnvironment(false))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var out map[string]any
	if err := resolved.Unmarshal(&out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The top-level fallback null must be present as a visible key with a nil value.
	v, ok := out["value"]
	if !ok {
		t.Fatalf("top-level fallback null key 'value' missing from unmarshal map; got keys %v", keysOf(out))
	}
	if v != nil {
		t.Fatalf("top-level 'value' = %#v, want nil", v)
	}

	// Regression guard: non-null sibling keys must still be present.
	for _, k := range []string{"environment", "variables"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("non-null key %q dropped from unmarshal map; got keys %v", k, keysOf(out))
		}
	}
}

// A standalone explicit null (no fallback, no substitution) must also unmarshal
// into map[string]any as a present key→nil, not a dropped key.
func TestIssue131_PlainNullKeyPreservedInUnmarshalMap(t *testing.T) {
	cfg, err := hocon.ParseString(`a = null
b = 1`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out map[string]any
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := out["a"]; !ok || v != nil {
		t.Fatalf("key 'a' = (%#v, present=%v), want (nil, true)", out["a"], ok)
	}
	if out["b"] != int64(1) {
		t.Fatalf("key 'b' = %#v, want int64(1)", out["b"])
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
