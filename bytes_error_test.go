// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Bytes accessor error paths. Asserts that:
//   - GetBytes panics with "invalid byte size" (wrapped in *ConfigError) when
//     the value cannot be parsed as a byte size (e.g. "not-a-size"). Mirrors
//     Lightbend's ConfigException for non-parseable size strings.
//   - GetBytesOption returns None for unknown unit suffix (e.g. "10KX"),
//     reflecting parseBytes's "unknown byte unit" error path.

package hocon_test

import (
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

func TestGetBytes_InvalidString_Panics(t *testing.T) {
	cfg, err := hocon.ParseString(`v = "not-a-size"`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("GetBytes(\"v\"): expected panic for non-parseable byte size, got none")
		}
		ce, ok := r.(*hocon.ConfigError)
		if !ok {
			t.Fatalf("panic value type = %T, want *hocon.ConfigError", r)
		}
		if !strings.Contains(ce.Message, "invalid byte size") {
			t.Errorf("panic message = %q; want substring %q", ce.Message, "invalid byte size")
		}
	}()
	_ = cfg.GetBytes("v")
}

func TestGetBytesOption_UnknownUnit_ReturnsNone(t *testing.T) {
	cfg, err := hocon.ParseString(`v = "10KX"`)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	opt := cfg.GetBytesOption("v")
	if opt.IsSome() {
		got, _ := opt.Get()
		t.Errorf("GetBytesOption(\"v\") on unknown unit %q: expected None, got %d", "10KX", got)
	}
}
