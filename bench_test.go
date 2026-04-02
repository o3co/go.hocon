// Copyright 2026 o3co Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// ---------------------------------------------------------------------------
// Fixture generators (mirrors ts.hocon/tests/bench/fixtures.ts)
// ---------------------------------------------------------------------------

// generateConfig builds a HOCON string with totalKeys spread across maxDepth
// groups, returning the HOCON text and a sample lookup path.
func generateConfig(totalKeys, maxDepth int) (hocon string, samplePath string) {
	var b strings.Builder
	keysPerGroup := totalKeys / maxDepth
	if keysPerGroup < 1 {
		keysPerGroup = 1
	}

	for d := 0; d < maxDepth; d++ {
		count := keysPerGroup
		if d == maxDepth-1 {
			count = totalKeys - keysPerGroup*(maxDepth-1)
		}
		fmt.Fprintf(&b, "group%d {\n", d)
		for i := 0; i < count; i++ {
			fmt.Fprintf(&b, "  key%d = \"value%d_%d\"\n", i, d, i)
		}
		b.WriteString("}\n")
	}

	return b.String(), "group0.key0"
}

// generateWithSubstitutions builds a HOCON string containing base keys and
// count substitution references, returning the text and a sample lookup path.
func generateWithSubstitutions(count int) (hocon string, samplePath string) {
	total := count * 2
	var b strings.Builder

	for i := 0; i < total; i++ {
		fmt.Fprintf(&b, "base%d = \"value%d\"\n", i, i)
	}
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "sub%d = ${base%d}\n", i, i%total)
	}

	return b.String(), "sub0"
}

// generateDeepNested builds a deeply nested HOCON object with 5 leaf keys at
// the innermost level, returning the text and a sample lookup path.
func generateDeepNested(depth int) (hocon string, samplePath string) {
	var b strings.Builder
	indent := ""

	// Open nesting levels
	for d := 0; d < depth; d++ {
		fmt.Fprintf(&b, "%slevel%d {\n", indent, d)
		indent += "  "
	}

	// Leaf keys
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&b, "%skey%d = \"deep_value%d\"\n", indent, i, i)
	}

	// Close nesting levels
	for d := depth - 1; d >= 0; d-- {
		indent = strings.Repeat("  ", d)
		fmt.Fprintf(&b, "%s}\n", indent)
	}

	// Build path: level0.level1. ... .levelN-1.key0
	parts := make([]string, depth+1)
	for i := 0; i < depth; i++ {
		parts[i] = fmt.Sprintf("level%d", i)
	}
	parts[depth] = "key0"

	return b.String(), strings.Join(parts, ".")
}

// ---------------------------------------------------------------------------
// Benchmarks — config size
// ---------------------------------------------------------------------------

func BenchmarkParseSmall(b *testing.B) {
	input, path := generateConfig(10, 2)
	benchParse(b, input, path)
}

func BenchmarkParseMedium(b *testing.B) {
	input, path := generateConfig(100, 4)
	benchParse(b, input, path)
}

func BenchmarkParseLarge(b *testing.B) {
	input, path := generateConfig(1000, 6)
	benchParse(b, input, path)
}

// ---------------------------------------------------------------------------
// Benchmarks — substitutions
// ---------------------------------------------------------------------------

func BenchmarkSubstitutions10(b *testing.B) {
	input, path := generateWithSubstitutions(10)
	benchParse(b, input, path)
}

func BenchmarkSubstitutions50(b *testing.B) {
	input, path := generateWithSubstitutions(50)
	benchParse(b, input, path)
}

func BenchmarkSubstitutions100(b *testing.B) {
	input, path := generateWithSubstitutions(100)
	benchParse(b, input, path)
}

// ---------------------------------------------------------------------------
// Benchmarks — deep nesting
// ---------------------------------------------------------------------------

func BenchmarkDeepNest5(b *testing.B) {
	input, path := generateDeepNested(5)
	benchParse(b, input, path)
}

func BenchmarkDeepNest10(b *testing.B) {
	input, path := generateDeepNested(10)
	benchParse(b, input, path)
}

func BenchmarkDeepNest20(b *testing.B) {
	input, path := generateDeepNested(20)
	benchParse(b, input, path)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func benchParse(b *testing.B, input, path string) {
	b.Helper()
	// Verify once before the loop so failures are visible.
	cfg, err := hocon.ParseString(input)
	if err != nil {
		b.Fatalf("ParseString setup error: %v", err)
	}
	_ = cfg.GetString(path)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg, _ = hocon.ParseString(input)
		_ = cfg.GetString(path)
	}
}
