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

// Multi-agent-review regression (cross-impl with ts.hocon / rs.hocon): after the
// #134 `+=` desugar, a fallback's `+=` value is a `${?items} [...]` self-ref
// concat. MergeUnresolved (WithFallback) must fold it self-ref-free before
// recording it as the receiver's prior — otherwise the receiver's `${?items}`
// follows a prior that still contains `${?items}`, recursing forever (stack
// overflow). Fallback fills, receiver appends → ["f", "r"].
func TestS13b2_DeferredWithFallbackPlusEqualsAccumulates(t *testing.T) {
	deferred := hocon.DefaultParseOptions().WithResolveSubstitutions(false)
	recv, err := hocon.ParseStringWithOptions(`items += "r"`, deferred)
	if err != nil {
		t.Fatalf("parse recv: %v", err)
	}
	fb, err := hocon.ParseStringWithOptions(`items += "f"`, deferred)
	if err != nil {
		t.Fatalf("parse fb: %v", err)
	}
	merged, err := recv.WithFallback(fb).Resolve(hocon.DefaultResolveOptions())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := merged.GetStringSlice("items")
	want := []string{"f", "r"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
