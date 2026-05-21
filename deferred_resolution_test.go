// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package hocon_test exercises E12 deferred substitution resolution.
// All tests in this file are Layer-1 (programmatic per-impl) tests; the
// Layer-2 YAML scenario runner lives in deferred_resolution_fixture_test.go.
package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon"
)

func TestConfig_IsResolved_FusedParseAndResolveIsResolved(t *testing.T) {
	c, err := hocon.ParseString(`a = 1`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if !c.IsResolved() {
		t.Fatal("fused ParseString must produce a resolved Config")
	}
}
