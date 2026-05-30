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
	"strconv"
	"time"

	"github.com/o3co/go.hocon/internal/resolver"
)

// Error-returning (T, error) accessors — the third member of go.hocon's
// accessor family triad (go.hocon#142):
//
//   GetString(path) string                  — panics on missing / type mismatch
//   GetStringE(path) (string, error)        — typed error (this file)
//   GetStringOption(path) Option[string]    — soft None on missing / type mismatch
//
// Mirrors rs.hocon's Result-primary API (`get_string -> Result<String, ConfigError>`)
// and is the idiomatic Go shape for surfacing expected config-shape failures.
// Returns *ConfigError on all failures (missing path / unresolved / non-scalar /
// null / type-conversion failure). Unresolved-placeholder errors carry
// ErrNotResolved in their Unwrap chain so callers can `errors.Is(err,
// hocon.ErrNotResolved)`.

// lookupE is the (T, error)-sibling of lookup() / lookupSoft(): returns the
// resolved value or a typed ConfigError, never panicking. Unresolved-placeholder
// errors wrap ErrNotResolved for errors.Is matching.
func (c *Config) lookupE(path string) (resolver.Val, error) {
	if path == "" {
		return nil, &ConfigError{Path: path, Message: "empty path"}
	}
	segments := splitPath(path)
	v, ok := lookupSegments(c.root, segments)
	if !ok {
		return nil, &ConfigError{Path: path, Message: "key not found"}
	}
	if !c.resolved && isUnresolvedPlaceholder(v) {
		return nil, &ConfigError{
			Path:    path,
			Message: "value is not resolved (call Resolve() first)",
			cause:   ErrNotResolved,
		}
	}
	return v, nil
}

// getScalarE is the (T, error)-sibling of getScalar(): returns the scalar value
// or a typed ConfigError on missing / non-scalar / null.
func (c *Config) getScalarE(path string) (*resolver.ScalarVal, error) {
	v, err := c.lookupE(path)
	if err != nil {
		return nil, err
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		return nil, &ConfigError{Path: path, Message: fmt.Sprintf("expected scalar, got %T", v)}
	}
	if sv.Type == resolver.ScalarNull {
		return nil, &ConfigError{Path: path, Message: "value is null"}
	}
	return sv, nil
}

// getArrayE is the (T, error)-sibling of getArray(): returns the array (with
// S15 numeric-object→array conversion) or a typed ConfigError. Mirrors the
// lookup-then-convert order of getArray()/lookupArray().
func (c *Config) getArrayE(path string) (*resolver.ArrayVal, error) {
	v, err := c.lookupE(path)
	if err != nil {
		return nil, err
	}
	if obj, isObj := v.(*resolver.ObjectVal); isObj {
		if converted, convOK := resolver.NumericObjectToArray(obj); convOK {
			return converted, nil
		}
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		return nil, &ConfigError{Path: path, Message: fmt.Sprintf("expected array, got %T", v)}
	}
	return arr, nil
}

// ── scalar accessors ─────────────────────────────────────────────────

// GetStringE returns the value at path as a string, or *ConfigError on missing /
// non-scalar / null / unresolved.
func (c *Config) GetStringE(path string) (string, error) {
	sv, err := c.getScalarE(path)
	if err != nil {
		return "", err
	}
	return sv.Raw, nil
}

// GetInt64E returns the value at path parsed as int64, or *ConfigError on
// missing / non-scalar / null / unresolved / non-integer text.
func (c *Config) GetInt64E(path string) (int64, error) {
	sv, err := c.getScalarE(path)
	if err != nil {
		return 0, err
	}
	parsed, perr := strconv.ParseInt(sv.Raw, 10, 64)
	if perr != nil {
		return 0, &ConfigError{Path: path, Message: fmt.Sprintf("expected int64, got %q", sv.Raw)}
	}
	return parsed, nil
}

// GetIntE returns the value at path parsed as int (narrowed from int64). On
// 32-bit platforms the int64 value is truncated to int — matches the existing
// GetInt panic-getter's width contract.
func (c *Config) GetIntE(path string) (int, error) {
	v, err := c.GetInt64E(path)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// GetFloat64E returns the value at path parsed as float64, or *ConfigError on
// missing / non-scalar / null / unresolved / non-numeric text.
func (c *Config) GetFloat64E(path string) (float64, error) {
	sv, err := c.getScalarE(path)
	if err != nil {
		return 0, err
	}
	parsed, perr := strconv.ParseFloat(sv.Raw, 64)
	if perr != nil {
		return 0, &ConfigError{Path: path, Message: fmt.Sprintf("expected float64, got %q", sv.Raw)}
	}
	return parsed, nil
}

// GetFloat32E returns the value at path narrowed to float32. Precision loss
// follows Go's float64→float32 conversion semantics — matches the existing
// GetFloat32 panic-getter.
func (c *Config) GetFloat32E(path string) (float32, error) {
	v, err := c.GetFloat64E(path)
	if err != nil {
		return 0, err
	}
	return float32(v), nil
}

// GetBoolE returns the value at path parsed as bool (HOCON's permissive
// true/false/yes/no/on/off — see parseBool), or *ConfigError otherwise.
func (c *Config) GetBoolE(path string) (bool, error) {
	sv, err := c.getScalarE(path)
	if err != nil {
		return false, err
	}
	parsed, ok := parseBool(sv.Raw)
	if !ok {
		return false, &ConfigError{Path: path, Message: fmt.Sprintf("expected bool, got %q", sv.Raw)}
	}
	return parsed, nil
}

// GetDurationE returns the value at path parsed as a HOCON duration string,
// or *ConfigError on missing / non-scalar / null / unresolved / invalid format.
func (c *Config) GetDurationE(path string) (time.Duration, error) {
	s, err := c.GetStringE(path)
	if err != nil {
		return 0, err
	}
	d, perr := parseDuration(s)
	if perr != nil {
		return 0, &ConfigError{Path: path, Message: "invalid duration: " + s}
	}
	return d, nil
}

// GetBytesE returns the value at path parsed as a HOCON byte-size string. Per
// S18.4 accessor invariant: negative byte sizes are rejected (the value parses
// but the accessor returns an error — matching the existing GetBytes panic
// behavior). Returns *ConfigError on missing / non-scalar / null / unresolved /
// invalid format / negative.
func (c *Config) GetBytesE(path string) (int64, error) {
	s, err := c.GetStringE(path)
	if err != nil {
		return 0, err
	}
	n, perr := parseBytes(s)
	if perr != nil {
		return 0, &ConfigError{Path: path, Message: "invalid byte size: " + s}
	}
	if n < 0 {
		return 0, &ConfigError{Path: path, Message: "byte size must not be negative"}
	}
	return n, nil
}

// ── slice accessors ──────────────────────────────────────────────────

// GetStringSliceE returns the array at path as a []string, or *ConfigError on
// missing / unresolved / non-array / a non-scalar / null element.
func (c *Config) GetStringSliceE(path string) ([]string, error) {
	arr, err := c.getArrayE(path)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return nil, &ConfigError{Path: path, Message: fmt.Sprintf("element %d is not a non-null scalar", i)}
		}
		result[i] = sv.Raw
	}
	return result, nil
}

// GetInt64SliceE returns the array at path as a []int64, or *ConfigError on
// missing / unresolved / non-array / non-integer element.
func (c *Config) GetInt64SliceE(path string) ([]int64, error) {
	arr, err := c.getArrayE(path)
	if err != nil {
		return nil, err
	}
	result := make([]int64, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return nil, &ConfigError{Path: path, Message: fmt.Sprintf("element %d is not an int", i)}
		}
		parsed, perr := strconv.ParseInt(sv.Raw, 10, 64)
		if perr != nil {
			return nil, &ConfigError{Path: path, Message: fmt.Sprintf("element %d: expected int64, got %q", i, sv.Raw)}
		}
		result[i] = parsed
	}
	return result, nil
}

// GetIntSliceE returns the array at path narrowed to []int. Narrows the
// int64 result of GetInt64SliceE; on a 32-bit platform values outside int range
// truncate (matches the existing GetIntSlice panic-getter).
func (c *Config) GetIntSliceE(path string) ([]int, error) {
	s64, err := c.GetInt64SliceE(path)
	if err != nil {
		return nil, err
	}
	r := make([]int, len(s64))
	for i, v := range s64 {
		r[i] = int(v)
	}
	return r, nil
}

// ── object accessors ─────────────────────────────────────────────────

// GetConfigE returns the object at path as a *Config, or *ConfigError on
// missing / unresolved / non-object. A null value folds into the non-object
// error (parity with the GetConfig panic-getter); callers that need to
// distinguish null from other non-object cases should use GetConfigOption,
// which returns None on null.
func (c *Config) GetConfigE(path string) (*Config, error) {
	v, err := c.lookupE(path)
	if err != nil {
		return nil, err
	}
	obj, ok := v.(*resolver.ObjectVal)
	if !ok {
		return nil, &ConfigError{Path: path, Message: fmt.Sprintf("expected object, got %T", v)}
	}
	return newConfig(obj), nil
}

// GetConfigSliceE returns the array at path as a []*Config, or *ConfigError on
// missing / unresolved / non-array / non-object element.
func (c *Config) GetConfigSliceE(path string) ([]*Config, error) {
	arr, err := c.getArrayE(path)
	if err != nil {
		return nil, err
	}
	result := make([]*Config, len(arr.Elements))
	for i, elem := range arr.Elements {
		obj, ok := elem.(*resolver.ObjectVal)
		if !ok {
			return nil, &ConfigError{Path: path, Message: fmt.Sprintf("element %d is not an object", i)}
		}
		result[i] = newConfig(obj)
	}
	return result, nil
}
