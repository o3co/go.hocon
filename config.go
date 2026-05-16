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
	"strings"
	"time"
	"unicode"

	"github.com/o3co/go.hocon/internal/resolver"
)

// Config wraps a resolved HOCON value tree.
// All *Config values are safe for concurrent read access.
type Config struct {
	root *resolver.ObjectVal
}

// newConfig wraps an ObjectVal.
func newConfig(obj *resolver.ObjectVal) *Config {
	return &Config{root: obj}
}

// ── path resolution ──────────────────────────────────────────────

func (c *Config) lookup(path string) (resolver.Val, bool) {
	if path == "" {
		panicConfig(path, "empty path")
	}
	segments := splitPath(path)
	return lookupSegments(c.root, segments)
}

func splitPath(path string) []string {
	var segments []string
	i := 0
	for i < len(path) {
		if path[i] == '"' {
			i++ // skip opening quote
			var seg []byte
			closed := false
			for i < len(path) {
				if path[i] == '\\' && i+1 < len(path) {
					seg = append(seg, path[i+1])
					i += 2
					continue
				}
				if path[i] == '"' {
					closed = true
					i++
					break
				}
				seg = append(seg, path[i])
				i++
			}
			segments = append(segments, string(seg))
			if !closed {
				break
			}
			if i < len(path) && path[i] == '.' {
				i++
			}
		} else {
			dot := strings.IndexByte(path[i:], '.')
			if dot == -1 {
				segments = append(segments, path[i:])
				break
			}
			segments = append(segments, path[i:i+dot])
			i = i + dot + 1
		}
	}
	return segments
}

func lookupSegments(obj *resolver.ObjectVal, segments []string) (resolver.Val, bool) {
	if obj == nil || len(segments) == 0 {
		return nil, false
	}
	v, ok := obj.Get(segments[0])
	if !ok {
		return nil, false
	}
	if len(segments) == 1 {
		return v, true
	}
	child, ok2 := v.(*resolver.ObjectVal)
	if !ok2 {
		return nil, false
	}
	return lookupSegments(child, segments[1:])
}

// ── Has / Keys ────────────────────────────────────────────────────

// Has returns true if the path exists, including when the value is null.
func (c *Config) Has(path string) bool {
	_, ok := c.lookup(path)
	return ok
}

// Keys returns the direct child key names of the current object.
func (c *Config) Keys() []string {
	if c.root == nil {
		return nil
	}
	return c.root.Keys()
}

// ── scalar getters ────────────────────────────────────────────────

func (c *Config) getScalar(path string) *resolver.ScalarVal {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected scalar, got %T", v))
	}
	if sv.Type == resolver.ScalarNull {
		panicConfig(path, "value is null")
	}
	return sv
}

func (c *Config) GetString(path string) string {
	sv := c.getScalar(path)
	return sv.Raw
}

func (c *Config) GetStringOption(path string) Option[string] {
	v, ok := c.lookup(path)
	if !ok {
		return None[string]()
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.Type == resolver.ScalarNull {
		return None[string]()
	}
	return Some(sv.Raw)
}

func (c *Config) GetInt64(path string) int64 {
	sv := c.getScalar(path)
	parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
	if err != nil {
		panicConfig(path, fmt.Sprintf("expected int64, got %q", sv.Raw))
	}
	return parsed
}

func (c *Config) GetInt64Option(path string) Option[int64] {
	v, ok := c.lookup(path)
	if !ok {
		return None[int64]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.Type == resolver.ScalarNull {
		return None[int64]()
	}
	if parsed, err := strconv.ParseInt(sv.Raw, 10, 64); err == nil {
		return Some(parsed)
	}
	return None[int64]()
}

func (c *Config) GetInt(path string) int { return int(c.GetInt64(path)) }
func (c *Config) GetIntOption(path string) Option[int] {
	o := c.GetInt64Option(path)
	if o.IsNone() {
		return None[int]()
	}
	v, _ := o.Get()
	return Some(int(v))
}

func (c *Config) GetFloat64(path string) float64 {
	sv := c.getScalar(path)
	parsed, err := strconv.ParseFloat(sv.Raw, 64)
	if err != nil {
		panicConfig(path, fmt.Sprintf("expected float64, got %q", sv.Raw))
	}
	return parsed
}

func (c *Config) GetFloat64Option(path string) Option[float64] {
	v, ok := c.lookup(path)
	if !ok {
		return None[float64]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.Type == resolver.ScalarNull {
		return None[float64]()
	}
	if parsed, err := strconv.ParseFloat(sv.Raw, 64); err == nil {
		return Some(parsed)
	}
	return None[float64]()
}

func (c *Config) GetFloat32(path string) float32 { return float32(c.GetFloat64(path)) }
func (c *Config) GetFloat32Option(path string) Option[float32] {
	o := c.GetFloat64Option(path)
	if o.IsNone() {
		return None[float32]()
	}
	v, _ := o.Get()
	return Some(float32(v))
}

func (c *Config) GetBool(path string) bool {
	sv := c.getScalar(path)
	parsed, ok := parseBool(sv.Raw)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected bool, got %q", sv.Raw))
	}
	return parsed
}

func (c *Config) GetBoolOption(path string) Option[bool] {
	v, ok := c.lookup(path)
	if !ok {
		return None[bool]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.Type == resolver.ScalarNull {
		return None[bool]()
	}
	if parsed, ok3 := parseBool(sv.Raw); ok3 {
		return Some(parsed)
	}
	return None[bool]()
}

// parseBool parses a boolean string per the HOCON spec.
// Accepted values (case-insensitive): true, yes, on → true; false, no, off → false.
// Falls back to strconv.ParseBool for 1/0/t/f/T/F compatibility.
func parseBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "yes", "on":
		return true, true
	case "false", "no", "off":
		return false, true
	}
	if v, err := strconv.ParseBool(s); err == nil {
		return v, true
	}
	return false, false
}

// ── duration ──────────────────────────────────────────────────────

func (c *Config) GetDuration(path string) time.Duration {
	s := c.GetString(path)
	d, err := parseDuration(s)
	if err != nil {
		panicConfig(path, "invalid duration: "+s)
	}
	return d
}

func (c *Config) GetDurationOption(path string) Option[time.Duration] {
	opt := c.GetStringOption(path)
	if opt.IsNone() {
		return None[time.Duration]()
	}
	s, _ := opt.Get()
	d, err := parseDuration(s)
	if err != nil {
		return None[time.Duration]()
	}
	return Some(d)
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	// find where digits end
	i := 0
	for i < len(s) && (s[i] == '-' || (s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("no number in duration %q", s)
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, err
	}
	unit := strings.TrimSpace(s[i:])
	var mult time.Duration
	switch unit {
	case "ns", "nanoseconds", "nanosecond":
		mult = time.Nanosecond
	case "ms", "milliseconds", "millisecond":
		mult = time.Millisecond
	case "s", "seconds", "second":
		mult = time.Second
	case "m", "minutes", "minute":
		mult = time.Minute
	case "h", "hours", "hour":
		mult = time.Hour
	case "d", "days", "day":
		mult = 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown duration unit %q", unit)
	}
	return time.Duration(num * float64(mult)), nil
}

// ── bytes ─────────────────────────────────────────────────────────

func (c *Config) GetBytes(path string) int64 {
	s := c.GetString(path)
	n, err := parseBytes(s)
	if err != nil {
		panicConfig(path, "invalid byte size: "+s)
	}
	return n
}

func (c *Config) GetBytesOption(path string) Option[int64] {
	opt := c.GetStringOption(path)
	if opt.IsNone() {
		return None[int64]()
	}
	s, _ := opt.Get()
	n, err := parseBytes(s)
	if err != nil {
		return None[int64]()
	}
	return Some(n)
}

func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("no number in byte size %q", s)
	}
	num, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, err
	}
	unit := strings.TrimFunc(s[i:], unicode.IsSpace)
	multipliers := map[string]int64{
		"B": 1, "byte": 1, "bytes": 1,
		"KB": 1000, "kilobyte": 1000, "kilobytes": 1000,
		"KiB": 1024, "kibibyte": 1024, "kibibytes": 1024,
		"MB": 1_000_000, "megabyte": 1_000_000, "megabytes": 1_000_000,
		"MiB": 1024 * 1024, "mebibyte": 1024 * 1024, "mebibytes": 1024 * 1024,
		"GB": 1_000_000_000, "gigabyte": 1_000_000_000, "gigabytes": 1_000_000_000,
		"GiB": 1024 * 1024 * 1024, "gibibyte": 1024 * 1024 * 1024, "gibibytes": 1024 * 1024 * 1024,
		"TB": 1_000_000_000_000, "terabyte": 1_000_000_000_000, "terabytes": 1_000_000_000_000,
		"TiB": 1024 * 1024 * 1024 * 1024, "tebibyte": 1024 * 1024 * 1024 * 1024, "tebibytes": 1024 * 1024 * 1024 * 1024,
	}
	mult, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unknown byte unit %q", unit)
	}
	return num * mult, nil
}

// ── slices ────────────────────────────────────────────────────────

// lookupArray resolves path and returns an *ArrayVal, applying S15 numeric-object
// conversion when the value is an ObjectVal.  Returns (arr, true) on success,
// (nil, false) when the path is missing or the value cannot be converted.
// The caller must NOT call this when the path is expected to panic (use getArray).
func (c *Config) lookupArray(path string) (*resolver.ArrayVal, bool) {
	v, ok := c.lookup(path)
	if !ok {
		return nil, false
	}
	// S15: try numeric-object → array conversion before the type check.
	if obj, isObj := v.(*resolver.ObjectVal); isObj {
		if converted, convOK := resolver.NumericObjectToArray(obj); convOK {
			return converted, true
		}
		// Non-eligible object (empty or no int keys) → not an array.
		return nil, false
	}
	arr, isArr := v.(*resolver.ArrayVal)
	if !isArr {
		return nil, false
	}
	return arr, true
}

func (c *Config) getArray(path string) *resolver.ArrayVal {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	// S15: if the value is a numerically-indexed object, attempt conversion to
	// an array before the type check.  Non-eligible objects (empty, no int keys)
	// fall through to the panic below — preserving existing error semantics.
	if obj, isObj := v.(*resolver.ObjectVal); isObj {
		if converted, convOK := resolver.NumericObjectToArray(obj); convOK {
			return converted
		}
	}
	arr, ok := v.(*resolver.ArrayVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected array, got %T", v))
	}
	return arr
}

func (c *Config) GetStringSlice(path string) []string {
	arr := c.getArray(path)
	result := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			panicConfig(path, fmt.Sprintf("element %d is not a non-null scalar", i))
		}
		result[i] = sv.Raw
	}
	return result
}

func (c *Config) GetStringSliceOption(path string) Option[[]string] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]string]()
	}
	result := make([]string, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return None[[]string]()
		}
		result[i] = sv.Raw
	}
	return Some(result)
}

func (c *Config) GetInt64Slice(path string) []int64 {
	arr := c.getArray(path)
	result := make([]int64, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			panicConfig(path, fmt.Sprintf("element %d is not an int", i))
		}
		parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			panicConfig(path, fmt.Sprintf("element %d: expected int64, got %q", i, sv.Raw))
		}
		result[i] = parsed
	}
	return result
}

func (c *Config) GetInt64SliceOption(path string) Option[[]int64] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]int64]()
	}
	result := make([]int64, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return None[[]int64]()
		}
		parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			return None[[]int64]()
		}
		result[i] = parsed
	}
	return Some(result)
}

func (c *Config) GetIntSlice(path string) []int {
	s := c.GetInt64Slice(path)
	r := make([]int, len(s))
	for i, v := range s {
		r[i] = int(v)
	}
	return r
}

func (c *Config) GetIntSliceOption(path string) Option[[]int] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]int]()
	}
	result := make([]int, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.Type == resolver.ScalarNull {
			return None[[]int]()
		}
		parsed, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			return None[[]int]()
		}
		result[i] = int(parsed)
	}
	return Some(result)
}

// ── object getters ────────────────────────────────────────────────

func (c *Config) GetConfig(path string) *Config {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	obj, ok := v.(*resolver.ObjectVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected object, got %T", v))
	}
	return newConfig(obj)
}

func (c *Config) GetConfigOption(path string) Option[*Config] {
	v, ok := c.lookup(path)
	if !ok {
		return None[*Config]()
	}
	sv, isSc := v.(*resolver.ScalarVal)
	if isSc && sv.Type == resolver.ScalarNull {
		return None[*Config]()
	}
	obj, ok2 := v.(*resolver.ObjectVal)
	if !ok2 {
		return None[*Config]()
	}
	return Some(newConfig(obj))
}

func (c *Config) GetConfigSlice(path string) []*Config {
	arr := c.getArray(path)
	result := make([]*Config, len(arr.Elements))
	for i, elem := range arr.Elements {
		obj, ok := elem.(*resolver.ObjectVal)
		if !ok {
			panicConfig(path, fmt.Sprintf("element %d is not an object", i))
		}
		result[i] = newConfig(obj)
	}
	return result
}

func (c *Config) GetConfigSliceOption(path string) Option[[]*Config] {
	arr, ok := c.lookupArray(path)
	if !ok {
		return None[[]*Config]()
	}
	result := make([]*Config, len(arr.Elements))
	for i, elem := range arr.Elements {
		obj, ok := elem.(*resolver.ObjectVal)
		if !ok {
			return None[[]*Config]()
		}
		result[i] = newConfig(obj)
	}
	return Some(result)
}

// ── merge ─────────────────────────────────────────────────────────

// WithFallback returns a new Config that deep-merges receiver over fallback.
// Neither receiver nor fallback is mutated. If fallback is nil, returns receiver.
func (c *Config) WithFallback(fallback *Config) *Config {
	if fallback == nil {
		return c
	}
	merged := mergeObjectVals(c.root, fallback.root)
	return newConfig(merged)
}

// mergeObjectVals merges base into over (over's values win).
func mergeObjectVals(over, base *resolver.ObjectVal) *resolver.ObjectVal {
	result := resolver.NewObjectVal()
	// seed with base
	for _, k := range base.Keys() {
		v, _ := base.Get(k)
		result.Set(k, v)
	}
	// apply over
	for _, k := range over.Keys() {
		ov, _ := over.Get(k)
		if bv, ok := result.GetVal(k); ok {
			if bo, bok := bv.(*resolver.ObjectVal); bok {
				if oo, ook := ov.(*resolver.ObjectVal); ook {
					result.Set(k, mergeObjectVals(oo, bo))
					continue
				}
			}
		}
		result.Set(k, ov)
	}
	return result
}
