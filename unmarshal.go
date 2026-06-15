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
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/o3co/go.hocon/internal/resolver"
)

// Unmarshal maps the config into v using `hocon` struct tags.
// v must be a non-nil pointer to a struct or map[string]any.
func (c *Config) Unmarshal(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("hocon: Unmarshal requires a non-nil pointer")
	}
	return unmarshalVal(c.root, rv.Elem())
}

// UnmarshalPath maps the value at path into v using `hocon` struct tags.
// v must be a non-nil pointer. Unlike GetConfig(path).Unmarshal (which accepts
// only objects), path may reference any node — object, array, or scalar — so
// e.g. UnmarshalPath("servers", &[]Server{}) deserializes a list directly.
//
// Returns an error if the path is missing, if the value (or any nested value)
// is an unresolved substitution (the error wraps ErrNotResolved, detectable via
// errors.Is), or if the value cannot be unmarshalled into v.
func (c *Config) UnmarshalPath(path string, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("hocon: UnmarshalPath requires a non-nil pointer")
	}
	if path == "" {
		return fmt.Errorf("hocon: UnmarshalPath requires a non-empty path")
	}
	node, ok := lookupSegments(c.root, splitPath(path))
	if !ok {
		return fmt.Errorf("hocon: path %q: key not found", path)
	}
	if !c.resolved && isUnresolvedPlaceholder(node) {
		return fmt.Errorf("hocon: path %q is not resolved: %w", path, ErrNotResolved)
	}
	return unmarshalVal(node, rv.Elem())
}

func unmarshalVal(val resolver.Val, target reflect.Value) error {
	// dereference pointer
	if target.Kind() == reflect.Pointer {
		if val == nil {
			return nil
		}
		if sv, ok := val.(*resolver.ScalarVal); ok && sv.Type == resolver.ScalarNull {
			return nil
		}
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		return unmarshalVal(val, target.Elem())
	}

	switch target.Kind() {
	case reflect.Struct:
		return unmarshalStruct(val, target)
	case reflect.Map:
		return unmarshalMap(val, target)
	case reflect.Slice:
		return unmarshalSlice(val, target)
	case reflect.Interface:
		// Generic target (`any`): decode the node into the natural Go value
		// (map[string]any / []any / string / float64 / bool / nil). Only the
		// empty interface is supported; a non-empty interface (e.g. error,
		// fmt.Stringer) can't hold an arbitrary decoded value.
		if target.NumMethod() != 0 {
			return fmt.Errorf("hocon: cannot unmarshal into non-empty interface %s", target.Type())
		}
		a := valToAny(val)
		if a == nil {
			// null / nil node → reset the interface to its typed zero (nil),
			// so an explicit null overwrites any pre-populated value.
			target.Set(reflect.Zero(target.Type()))
			return nil
		}
		// target is the empty interface here (non-empty rejected above), so any
		// concrete value from valToAny is assignable.
		target.Set(reflect.ValueOf(a))
		return nil
	default:
		return unmarshalScalar(val, target)
	}
}

func unmarshalStruct(val resolver.Val, target reflect.Value) error {
	t := target.Type()

	// special case: time.Duration (underlying kind is int64, not struct,
	// so this branch is only reached if someone wraps Duration in a struct embedding;
	// kept for defensive completeness)
	if t == reflect.TypeOf(time.Duration(0)) {
		sv, ok2 := val.(*resolver.ScalarVal)
		if !ok2 {
			return fmt.Errorf("hocon: expected string for duration")
		}
		d, err := parseDuration(sv.Raw)
		if err != nil {
			return err
		}
		target.Set(reflect.ValueOf(d))
		return nil
	}

	obj, ok := val.(*resolver.ObjectVal)
	if !ok {
		return fmt.Errorf("hocon: expected object for struct, got %T", val)
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fval := target.Field(i)
		if !fval.CanSet() {
			continue
		}
		key, omitempty, skip := parseTag(field)
		if skip {
			continue
		}
		v, ok2 := obj.Get(key)
		if !ok2 {
			if omitempty {
				continue // preserve pre-populated value
			}
			// slices and maps default to nil when absent (not an error)
			if fval.Kind() == reflect.Slice || fval.Kind() == reflect.Map {
				continue
			}
			return fmt.Errorf("hocon: missing required field %q", key)
		}
		// null + omitempty: preserve
		if sv, isSc := v.(*resolver.ScalarVal); isSc && sv.Type == resolver.ScalarNull && omitempty {
			continue
		}
		if err := unmarshalVal(v, fval); err != nil {
			return fmt.Errorf("hocon: field %q: %w", key, err)
		}
	}
	return nil
}

func parseTag(f reflect.StructField) (key string, omitempty bool, skip bool) {
	tag := f.Tag.Get("hocon")
	if tag == "" {
		return strings.ToLower(f.Name), false, false
	}
	if tag == "-" {
		return "", false, true
	}
	parts := strings.SplitN(tag, ",", 2)
	key = parts[0]
	if key == "" {
		key = strings.ToLower(f.Name)
	}
	if len(parts) > 1 && parts[1] == "omitempty" {
		omitempty = true
	}
	return
}

func unmarshalMap(val resolver.Val, target reflect.Value) error {
	obj, ok := val.(*resolver.ObjectVal)
	if !ok {
		return fmt.Errorf("hocon: expected object for map, got %T", val)
	}
	if target.IsNil() {
		target.Set(reflect.MakeMap(target.Type()))
	}
	elemType := target.Type().Elem()
	// anyType is true for map[string]any (and any other map with an interface element type).
	// In that case we bypass reflection-based unmarshalling and use valToAny, which preserves
	// the native Go value (int64, float64, bool, string, slice, map) without needing a
	// concrete reflect.Value to set into.
	anyType := elemType.Kind() == reflect.Interface
	for _, k := range obj.Keys() {
		v, _ := obj.Get(k)
		if anyType {
			// valToAny returns an untyped nil for an explicit null; reflect.ValueOf(nil)
			// is an invalid Value, and SetMapIndex with an invalid value deletes the
			// entry rather than storing key→nil. Use a typed zero of the interface
			// element so explicit nulls remain visible keys (go.hocon#131).
			av := valToAny(v)
			rval := reflect.Zero(elemType)
			if av != nil {
				rval = reflect.ValueOf(av)
			}
			target.SetMapIndex(reflect.ValueOf(k), rval)
		} else {
			ev := reflect.New(elemType).Elem()
			if err := unmarshalVal(v, ev); err != nil {
				return fmt.Errorf("hocon: map key %q: %w", k, err)
			}
			target.SetMapIndex(reflect.ValueOf(k), ev)
		}
	}
	return nil
}

func valToAny(v resolver.Val) any {
	switch vv := v.(type) {
	case *resolver.ScalarVal:
		switch vv.Type {
		case resolver.ScalarNull:
			return nil
		case resolver.ScalarBoolean:
			return vv.Raw == "true"
		case resolver.ScalarNumber:
			// Try int first (no dot/exponent), then float
			if !strings.ContainsAny(vv.Raw, ".eE") {
				if n, err := strconv.ParseInt(vv.Raw, 10, 64); err == nil {
					return n
				}
			}
			if f, err := strconv.ParseFloat(vv.Raw, 64); err == nil {
				return f
			}
			return vv.Raw
		default:
			return vv.Raw
		}
	case *resolver.ArrayVal:
		r := make([]any, len(vv.Elements))
		for i, e := range vv.Elements {
			r[i] = valToAny(e)
		}
		return r
	case *resolver.ObjectVal:
		m := make(map[string]any)
		for _, k := range vv.Keys() {
			cv, _ := vv.Get(k)
			m[k] = valToAny(cv)
		}
		return m
	default:
		return nil
	}
}

func unmarshalSlice(val resolver.Val, target reflect.Value) error {
	arr, ok := val.(*resolver.ArrayVal)
	if !ok {
		// S15 parity: a numeric-keyed object converts to an array in slice
		// context (matching the typed slice getters and rs serde sequences).
		if obj, isObj := val.(*resolver.ObjectVal); isObj {
			if converted, convOK := resolver.NumericObjectToArray(obj); convOK {
				arr = converted
			} else {
				return fmt.Errorf("hocon: expected array for slice, got %T", val)
			}
		} else {
			return fmt.Errorf("hocon: expected array for slice, got %T", val)
		}
	}
	elemType := target.Type().Elem()
	slice := reflect.MakeSlice(target.Type(), len(arr.Elements), len(arr.Elements))
	for i, elem := range arr.Elements {
		ev := reflect.New(elemType).Elem()
		if err := unmarshalVal(elem, ev); err != nil {
			return fmt.Errorf("hocon: slice element %d: %w", i, err)
		}
		slice.Index(i).Set(ev)
	}
	target.Set(slice)
	return nil
}

func unmarshalScalar(val resolver.Val, target reflect.Value) error {
	sv, ok := val.(*resolver.ScalarVal)
	if !ok {
		return fmt.Errorf("hocon: expected scalar for %v, got %T", target.Type(), val)
	}
	if sv.Type == resolver.ScalarNull {
		return nil // null → zero value
	}

	// time.Duration special case (underlying kind is int64)
	if target.Type() == reflect.TypeOf(time.Duration(0)) {
		d, err := parseDuration(sv.Raw)
		if err != nil {
			return err
		}
		target.Set(reflect.ValueOf(d))
		return nil
	}

	switch target.Kind() {
	case reflect.String:
		target.SetString(sv.Raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(sv.Raw, 10, 64)
		if err != nil {
			// Whole-number float/exponent fallback, matching rs.hocon. Wholeness
			// and the value are derived from the raw decimal text, never via
			// float64 (xx.hocon#56): above 2^52 a float64 cannot represent
			// fractional parts, so a non-whole literal like "9007199254740992.5"
			// would round to a whole float64 and a would-be-whole one like
			// "9007199254740993.0" would round to the wrong integer.
			var ok bool
			n, ok = wholeFloatToInt64(sv.Raw)
			if !ok {
				return fmt.Errorf("hocon: expected int, got %q", sv.Raw)
			}
		}
		if target.OverflowInt(n) {
			return fmt.Errorf("hocon: int %d overflows %s", n, target.Type())
		}
		target.SetInt(n)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(sv.Raw, 64)
		if err != nil {
			return fmt.Errorf("hocon: expected float, got %q", sv.Raw)
		}
		target.SetFloat(parsed)
	case reflect.Bool:
		parsed, ok := parseBool(sv.Raw)
		if !ok {
			return fmt.Errorf("hocon: expected bool, got %q", sv.Raw)
		}
		target.SetBool(parsed)
	default:
		return fmt.Errorf("hocon: unsupported target type %v", target.Type())
	}
	return nil
}

// wholeFloatToInt64 coerces a whole-number float/exponent raw string to an exact
// int64, deriving both wholeness and the value from the decimal text rather than
// an intermediate float64 (xx.hocon#56, byte-identical to rs.hocon's
// whole_float_to_i64). Above 2^52 a float64 cannot represent fractional parts, so
// a float64-based check both false-accepts non-whole values (e.g.
// "9007199254740992.5") and off-by-one's would-be-whole ones (e.g.
// "9007199254740993.0"). Returns ok=false for non-float-like, non-whole, or
// out-of-int64-range input. Plain integers are handled by the caller's
// strconv.ParseInt fast path.
func wholeFloatToInt64(raw string) (int64, bool) {
	// Plain integers are handled by the caller's strconv.ParseInt.
	if !strings.ContainsAny(raw, ".eE") {
		return 0, false
	}
	s := strings.TrimSpace(raw)
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	// Mantissa and base-10 exponent (default 0).
	mantissa := s
	exp := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mantissa = s[:i]
		// Parse the exponent into an int32 range (matching rs.hocon's
		// `e.parse::<i32>()`): an exponent outside that range is rejected here, so
		// the int64 arithmetic on `r`/`zeros` below cannot overflow (a value like
		// "1e-9223372036854775808" would otherwise wrap and panic strings.Repeat).
		e, err := strconv.ParseInt(s[i+1:], 10, 32)
		if err != nil {
			return 0, false
		}
		exp = int(e)
	}
	// Integer and fractional digit runs.
	intPart, fracPart := mantissa, ""
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		intPart, fracPart = mantissa[:i], mantissa[i+1:]
	}
	if intPart == "" && fracPart == "" {
		return 0, false
	}
	if !allDigits(intPart) || !allDigits(fracPart) {
		return 0, false
	}
	// Significant digits; leading zeros never affect the value. An all-zero
	// mantissa is exactly 0 for any exponent — handle before the append-zeros
	// guard, which is keyed off the exponent.
	digits := strings.TrimLeft(intPart+fracPart, "0")
	if digits == "" {
		return 0, true
	}
	r := len(fracPart) - exp // digits to the right of the decimal point
	var magStr string
	if r <= 0 {
		zeros := -r
		// int64 has at most 19 digits; bound BEFORE building the string so a huge
		// exponent like "1e2147483647" can't allocate gigabytes. Check zeros
		// alone first to avoid int overflow when exp is enormous.
		if zeros > 19 || len(digits) > 19 || len(digits)+zeros > 19 {
			return 0, false
		}
		magStr = digits + strings.Repeat("0", zeros)
	} else {
		// digits is non-empty with a non-zero leading byte, so if every digit is
		// fractional the value is < 1 and not whole.
		if r >= len(digits) {
			return 0, false
		}
		head, tail := digits[:len(digits)-r], digits[len(digits)-r:]
		if !allZero(tail) {
			return 0, false
		}
		magStr = head
	}
	// Parse the unsigned magnitude, then apply the sign with an explicit range
	// check so that math.MinInt64 (magnitude 2^63, which does not fit int64) is
	// preserved for float-like spellings like "-9223372036854775808.0".
	mag, err := strconv.ParseUint(magStr, 10, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		switch {
		case mag < uint64(math.MaxInt64)+1:
			return -int64(mag), true
		case mag == uint64(math.MaxInt64)+1:
			return math.MinInt64, true
		default:
			return 0, false
		}
	}
	if mag > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(mag), true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func allZero(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}
