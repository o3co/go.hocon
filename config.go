// Copyright 2026 o3co Inc.
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
	return strings.Split(path, ".")
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

func (c *Config) getScalar(path string) any {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected scalar, got %T", v))
	}
	if sv.V == nil {
		panicConfig(path, "value is null")
	}
	return sv.V
}

func (c *Config) GetString(path string) string {
	v := c.getScalar(path)
	s, ok := v.(string)
	if !ok {
		panicConfig(path, fmt.Sprintf("expected string, got %T", v))
	}
	return s
}

func (c *Config) GetStringOption(path string) Option[string] {
	v, ok := c.lookup(path)
	if !ok {
		return None[string]()
	}
	sv, ok := v.(*resolver.ScalarVal)
	if !ok || sv.V == nil {
		return None[string]()
	}
	s, ok := sv.V.(string)
	if !ok {
		return None[string]()
	}
	return Some(s)
}

func (c *Config) GetInt64(path string) int64 {
	v := c.getScalar(path)
	switch n := v.(type) {
	case int64:
		return n
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			panicConfig(path, fmt.Sprintf("expected int64, got string %q", n))
		}
		return parsed
	default:
		panicConfig(path, fmt.Sprintf("expected int64, got %T", v))
	}
	panic("unreachable")
}

func (c *Config) GetInt64Option(path string) Option[int64] {
	v, ok := c.lookup(path)
	if !ok {
		return None[int64]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.V == nil {
		return None[int64]()
	}
	switch n := sv.V.(type) {
	case int64:
		return Some(n)
	case string:
		if parsed, err := strconv.ParseInt(n, 10, 64); err == nil {
			return Some(parsed)
		}
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
	v := c.getScalar(path)
	switch f := v.(type) {
	case float64:
		return f
	case int64:
		return float64(f)
	case string:
		parsed, err := strconv.ParseFloat(f, 64)
		if err != nil {
			panicConfig(path, fmt.Sprintf("expected float64, got string %q", f))
		}
		return parsed
	}
	panicConfig(path, fmt.Sprintf("expected float64, got %T", v))
	return 0
}

func (c *Config) GetFloat64Option(path string) Option[float64] {
	v, ok := c.lookup(path)
	if !ok {
		return None[float64]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.V == nil {
		return None[float64]()
	}
	switch f := sv.V.(type) {
	case float64:
		return Some(f)
	case int64:
		return Some(float64(f))
	case string:
		if parsed, err := strconv.ParseFloat(f, 64); err == nil {
			return Some(parsed)
		}
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
	v := c.getScalar(path)
	switch b := v.(type) {
	case bool:
		return b
	case string:
		parsed, err := strconv.ParseBool(b)
		if err != nil {
			panicConfig(path, fmt.Sprintf("expected bool, got string %q", b))
		}
		return parsed
	}
	panicConfig(path, fmt.Sprintf("expected bool, got %T", v))
	return false
}

func (c *Config) GetBoolOption(path string) Option[bool] {
	v, ok := c.lookup(path)
	if !ok {
		return None[bool]()
	}
	sv, ok2 := v.(*resolver.ScalarVal)
	if !ok2 || sv.V == nil {
		return None[bool]()
	}
	switch b := sv.V.(type) {
	case bool:
		return Some(b)
	case string:
		if parsed, err := strconv.ParseBool(b); err == nil {
			return Some(parsed)
		}
	}
	return None[bool]()
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

func (c *Config) getArray(path string) *resolver.ArrayVal {
	v, ok := c.lookup(path)
	if !ok {
		panicConfig(path, "key not found")
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
		if !ok || sv.V == nil {
			panicConfig(path, fmt.Sprintf("element %d is not a string", i))
		}
		s, ok := sv.V.(string)
		if !ok {
			panicConfig(path, fmt.Sprintf("element %d: expected string, got %T", i, sv.V))
		}
		result[i] = s
	}
	return result
}

func (c *Config) GetStringSliceOption(path string) Option[[]string] {
	if !c.Has(path) {
		return None[[]string]()
	}
	return Some(c.GetStringSlice(path))
}

func (c *Config) GetInt64Slice(path string) []int64 {
	arr := c.getArray(path)
	result := make([]int64, len(arr.Elements))
	for i, elem := range arr.Elements {
		sv, ok := elem.(*resolver.ScalarVal)
		if !ok || sv.V == nil {
			panicConfig(path, fmt.Sprintf("element %d is not an int", i))
		}
		switch n := sv.V.(type) {
		case int64:
			result[i] = n
		case string:
			parsed, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				panicConfig(path, fmt.Sprintf("element %d: expected int64, got string %q", i, n))
			}
			result[i] = parsed
		default:
			panicConfig(path, fmt.Sprintf("element %d: expected int64, got %T", i, sv.V))
		}
	}
	return result
}

func (c *Config) GetInt64SliceOption(path string) Option[[]int64] {
	if !c.Has(path) {
		return None[[]int64]()
	}
	return Some(c.GetInt64Slice(path))
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
	if !c.Has(path) {
		return None[[]int]()
	}
	return Some(c.GetIntSlice(path))
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
	if isSc && sv.V == nil {
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
	if !c.Has(path) {
		return None[[]*Config]()
	}
	return Some(c.GetConfigSlice(path))
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

var _ = math.IsInf // ensure math is used (for float32 overflow docs)
