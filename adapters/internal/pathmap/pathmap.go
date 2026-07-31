// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package pathmap builds the nested map that hocon.FromMap expects out of
// flat, path-keyed entries.
//
// hocon.FromMap treats its keys as plain keys rather than path expressions:
// passing {"a.b": 1} produces a single top-level key literally named "a.b".
// Adapters for flat formats (Properties, env) therefore have to build the
// nesting themselves, which is what this package does.  Leaves are strings
// because every flat format this serves carries only strings; nested formats
// such as TOML decode straight into map[string]any and never come through
// here.
package pathmap

import (
	"fmt"
	"strings"
)

// Entry is one flat input pair: the path already split into segments, the
// value, and the original textual key used in error messages.
type Entry struct {
	Path   []string
	Value  string
	Source string
}

// Build nests entries into the map[string]any accepted by hocon.FromMap.
//
// Two rules from the format-ingestion mapping spec apply
// (https://github.com/o3co/xx.hocon/blob/main/docs/format-ingestion-mapping.md):
//
//   - F2.5 (objects win): a path that is both a value and a parent of another
//     path loses its scalar, so `a=1` plus `a.b=2` yields {"a":{"b":"2"}}.
//     The whole entry set is examined before anything is inserted, so the
//     outcome does not depend on input order.
//   - F0.7 (duplicates): the last entry for a path wins.  Callers that need a
//     collision to be an error instead — env, whose iteration order is not
//     deterministic — detect it before calling Build.
func Build(entries []Entry) (map[string]any, error) {
	scopes := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if len(e.Path) == 0 {
			return nil, fmt.Errorf("%s: empty path", e.Source)
		}
		for i, seg := range e.Path {
			if seg == "" {
				return nil, fmt.Errorf("%s: empty path segment at position %d", e.Source, i+1)
			}
		}
		for i := 1; i < len(e.Path); i++ {
			scopes[joinKey(e.Path[:i])] = struct{}{}
		}
	}

	root := map[string]any{}
	for _, e := range entries {
		if _, isScope := scopes[joinKey(e.Path)]; isScope {
			continue
		}
		if err := insert(root, e); err != nil {
			return nil, err
		}
	}
	return root, nil
}

func insert(root map[string]any, e Entry) error {
	cur := root
	for _, seg := range e.Path[:len(e.Path)-1] {
		next, ok := cur[seg]
		if !ok {
			child := map[string]any{}
			cur[seg] = child
			cur = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			// Unreachable: any path that is a parent was filtered out above.
			return fmt.Errorf("%s: %q is both a value and an object", e.Source, seg)
		}
		cur = child
	}
	cur[e.Path[len(e.Path)-1]] = e.Value
	return nil
}

// joinKey uses NUL, which cannot appear in a segment, so distinct paths cannot
// collide into the same scope key.
func joinKey(path []string) string { return strings.Join(path, "\x00") }
