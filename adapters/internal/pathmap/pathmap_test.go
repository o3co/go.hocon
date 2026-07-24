// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package pathmap

import (
	"reflect"
	"strings"
	"testing"
)

func entry(path string, value string) Entry {
	return Entry{Path: strings.Split(path, "."), Value: value, Source: path}
}

func TestBuildNests(t *testing.T) {
	got, err := Build([]Entry{entry("a.b.c", "1"), entry("a.b.d", "2"), entry("e", "3")})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := map[string]any{
		"a": map[string]any{"b": map[string]any{"c": "1", "d": "2"}},
		"e": "3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// F2.5: a path that is both a value and a parent loses its scalar, and the
// outcome must not depend on which entry came first.
func TestBuildObjectsWinRegardlessOfOrder(t *testing.T) {
	want := map[string]any{"a": map[string]any{"b": "2"}}
	for _, entries := range [][]Entry{
		{entry("a", "1"), entry("a.b", "2")},
		{entry("a.b", "2"), entry("a", "1")},
	} {
		got, err := Build(entries)
		if err != nil {
			t.Fatalf("Build(%v): %v", entries, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Build(%v) = %#v, want %#v", entries, got, want)
		}
	}
}

func TestBuildObjectsWinAtDepth(t *testing.T) {
	got, err := Build([]Entry{entry("a.b", "1"), entry("a.b.c", "2")})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := map[string]any{"a": map[string]any{"b": map[string]any{"c": "2"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// F0.7: leaves are scalars, so a repeated path is simply last-wins.
func TestBuildDuplicateLastWins(t *testing.T) {
	got, err := Build([]Entry{entry("a.b", "1"), entry("a.b", "2")})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := map[string]any{"a": map[string]any{"b": "2"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildRejectsEmptySegments(t *testing.T) {
	for _, tc := range []struct{ name, path string }{
		{"leading", ".a"},
		{"trailing", "a."},
		{"middle", "a..b"},
		{"only", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build([]Entry{entry(tc.path, "1")}); err == nil {
				t.Fatalf("Build(%q) succeeded, want error", tc.path)
			}
		})
	}
}

func TestBuildRejectsEmptyPath(t *testing.T) {
	if _, err := Build([]Entry{{Path: nil, Value: "1", Source: "x"}}); err == nil {
		t.Fatal("Build with nil path succeeded, want error")
	}
}

func TestBuildEmptyInput(t *testing.T) {
	got, err := Build(nil)
	if err != nil {
		t.Fatalf("Build(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %#v, want empty map", got)
	}
}
