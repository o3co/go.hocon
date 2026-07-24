// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package tree rebuilds a decoded foreign document into the shape
// hocon.FromMap accepts.
//
// Formats that already nest — JSON, TOML — decode into map[string]any and
// []any, so the only work left is enforcing the object-root rule and mapping
// each leaf onto a type the value factory understands. The per-format leaf
// rules differ (a TOML datetime, a JSON number), so callers supply those.
//
// Flat formats do not come through here; they build their nesting in
// internal/pathmap instead.
package tree

import (
	"fmt"
	"strconv"
	"strings"
)

// ScalarFunc converts one decoded leaf into a value hocon.FromMap accepts:
// nil, bool, string, int64 or float64.
type ScalarFunc func(v any) (any, error)

// Object rebuilds doc, which must be an object (spec F0.3).
func Object(doc any, scalar ScalarFunc) (map[string]any, error) {
	m, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("document root is %s, but a config root must be an object (spec F0.3)", describe(doc))
	}
	out, err := convert(m, scalar, nil)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func convert(v any, scalar ScalarFunc, path []string) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			ev, err := convert(e, scalar, descend(path, k))
			if err != nil {
				return nil, err
			}
			out[k] = ev
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			ev, err := convert(e, scalar, descend(path, "["+strconv.Itoa(i)+"]"))
			if err != nil {
				return nil, err
			}
			out[i] = ev
		}
		return out, nil
	default:
		s, err := scalar(v)
		if err != nil {
			return nil, fmt.Errorf("at %s: %w", join(path), err)
		}
		return s, nil
	}
}

// descend copies rather than appending in place: siblings would otherwise
// share a backing array and overwrite each other's segments.
func descend(path []string, seg string) []string {
	child := make([]string, len(path), len(path)+1)
	copy(child, path)
	return append(child, seg)
}

func join(path []string) string {
	if len(path) == 0 {
		return "document root"
	}
	// Index segments already carry their brackets, so they attach without a dot.
	var b strings.Builder
	for i, seg := range path {
		if i > 0 && !strings.HasPrefix(seg, "[") {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}

func describe(v any) string {
	switch v.(type) {
	case []any:
		return "an array"
	case nil:
		return "null"
	}
	return fmt.Sprintf("a %T", v)
}
