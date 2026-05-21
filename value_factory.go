// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/o3co/go.hocon/internal/resolver"
)

// FromMap constructs a Config from an in-memory map.  Keys are treated as
// plain keys (NOT path expressions — `"a.b": 1` produces a top-level key
// literally named "a.b", not nested `a.b`).  Values are coerced per the
// spec table (E12 § "Value factories").
//
// originDescription provides the source name for error messages when no
// file path is associated; "" means "use default".
//
// Returns ErrNotResolved is never; substitutions are not parsed from FromMap
// input.  Type-coercion errors return a fmt.Errorf wrapping the bad path.
func FromMap(values map[string]any, originDescription string) (*Config, error) {
	obj, err := coerceMap(values)
	if err != nil {
		return nil, err
	}
	return &Config{
		root:              obj,
		resolved:          true,
		originDescription: originDescription,
	}, nil
}

// Empty returns a Config with no keys.  Equivalent to FromMap(nil, origin).
func Empty(originDescription string) *Config {
	return &Config{
		root:              resolver.NewObjectVal(),
		resolved:          true,
		originDescription: originDescription,
	}
}

func coerceMap(in map[string]any) (*resolver.ObjectVal, error) {
	obj := resolver.NewObjectVal()
	// Sorted key iteration so cross-impl JSON output is stable.
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, err := coerceValue(in[k])
		if err != nil {
			return nil, fmt.Errorf("FromMap: key %q: %w", k, err)
		}
		obj.Set(k, v)
	}
	return obj, nil
}

func coerceValue(v any) (resolver.Val, error) {
	if v == nil {
		return &resolver.ScalarVal{Raw: "null", Type: resolver.ScalarNull}, nil
	}
	switch x := v.(type) {
	case bool:
		raw := "false"
		if x {
			raw = "true"
		}
		return &resolver.ScalarVal{Raw: raw, Type: resolver.ScalarBoolean}, nil
	case string:
		return &resolver.ScalarVal{Raw: x, Type: resolver.ScalarString}, nil
	case int:
		return &resolver.ScalarVal{Raw: strconv.FormatInt(int64(x), 10), Type: resolver.ScalarNumber}, nil
	case int8:
		return &resolver.ScalarVal{Raw: strconv.FormatInt(int64(x), 10), Type: resolver.ScalarNumber}, nil
	case int16:
		return &resolver.ScalarVal{Raw: strconv.FormatInt(int64(x), 10), Type: resolver.ScalarNumber}, nil
	case int32:
		return &resolver.ScalarVal{Raw: strconv.FormatInt(int64(x), 10), Type: resolver.ScalarNumber}, nil
	case int64:
		return &resolver.ScalarVal{Raw: strconv.FormatInt(x, 10), Type: resolver.ScalarNumber}, nil
	case uint:
		return uintToVal(uint64(x))
	case uint8:
		return &resolver.ScalarVal{Raw: strconv.FormatUint(uint64(x), 10), Type: resolver.ScalarNumber}, nil
	case uint16:
		return &resolver.ScalarVal{Raw: strconv.FormatUint(uint64(x), 10), Type: resolver.ScalarNumber}, nil
	case uint32:
		return &resolver.ScalarVal{Raw: strconv.FormatUint(uint64(x), 10), Type: resolver.ScalarNumber}, nil
	case uint64:
		return uintToVal(x)
	case float32:
		return floatToVal(float64(x))
	case float64:
		return floatToVal(x)
	case []any:
		arr := &resolver.ArrayVal{}
		for i, e := range x {
			ev, err := coerceValue(e)
			if err != nil {
				return nil, fmt.Errorf("element[%d]: %w", i, err)
			}
			arr.Elements = append(arr.Elements, ev)
		}
		return arr, nil
	case map[string]any:
		return coerceMap(x)
	}
	return nil, fmt.Errorf("unsupported value type %T", v)
}

func uintToVal(u uint64) (resolver.Val, error) {
	if u > math.MaxInt64 {
		return nil, fmt.Errorf("uint64 value %d exceeds int64.Max (HOCON numbers fit int64)", u)
	}
	return &resolver.ScalarVal{Raw: strconv.FormatUint(u, 10), Type: resolver.ScalarNumber}, nil
}

func floatToVal(f float64) (resolver.Val, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("float value %v is not finite (NaN/Inf not representable in HOCON)", f)
	}
	// Use FormatFloat with -1 precision for round-tripable representation.
	return &resolver.ScalarVal{Raw: strconv.FormatFloat(f, 'g', -1, 64), Type: resolver.ScalarNumber}, nil
}
