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
	"reflect"
	"strings"
	"time"

	"github.com/o3co/go.hocon/internal/resolver"
)

// Unmarshal maps the config into v using `hocon` struct tags.
// v must be a non-nil pointer to a struct or map[string]any.
func (c *Config) Unmarshal(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("hocon: Unmarshal requires a non-nil pointer")
	}
	return unmarshalVal(c.root, rv.Elem())
}

func unmarshalVal(val resolver.Val, target reflect.Value) error {
	// dereference pointer
	if target.Kind() == reflect.Ptr {
		if val == nil {
			return nil
		}
		if sv, ok := val.(*resolver.ScalarVal); ok && sv.V == nil {
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
		s, ok3 := sv.V.(string)
		if !ok3 {
			return fmt.Errorf("hocon: expected string for duration, got %T", sv.V)
		}
		d, err := parseDuration(s)
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
		key, omitempty := parseTag(field)
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
		if sv, isSc := v.(*resolver.ScalarVal); isSc && sv.V == nil && omitempty {
			continue
		}
		if err := unmarshalVal(v, fval); err != nil {
			return fmt.Errorf("hocon: field %q: %w", key, err)
		}
	}
	return nil
}

func parseTag(f reflect.StructField) (key string, omitempty bool) {
	tag := f.Tag.Get("hocon")
	if tag == "" {
		return strings.ToLower(f.Name), false
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
	for _, k := range obj.Keys() {
		v, _ := obj.Get(k)
		mval := valToAny(v)
		target.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(mval))
	}
	return nil
}

func valToAny(v resolver.Val) any {
	switch vv := v.(type) {
	case *resolver.ScalarVal:
		return vv.V
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
		return fmt.Errorf("hocon: expected array for slice, got %T", val)
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
	if sv.V == nil {
		return nil // null → zero value
	}

	// time.Duration special case (underlying kind is int64)
	if target.Type() == reflect.TypeOf(time.Duration(0)) {
		s, ok2 := sv.V.(string)
		if !ok2 {
			return fmt.Errorf("hocon: expected string for duration")
		}
		d, err := parseDuration(s)
		if err != nil {
			return err
		}
		target.Set(reflect.ValueOf(d))
		return nil
	}

	switch target.Kind() {
	case reflect.String:
		s, ok2 := sv.V.(string)
		if !ok2 {
			return fmt.Errorf("hocon: expected string, got %T", sv.V)
		}
		target.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch n := sv.V.(type) {
		case int64:
			target.SetInt(n)
		case float64:
			target.SetInt(int64(n))
		default:
			return fmt.Errorf("hocon: expected int, got %T", sv.V)
		}
	case reflect.Float32, reflect.Float64:
		switch f := sv.V.(type) {
		case float64:
			target.SetFloat(f)
		case int64:
			target.SetFloat(float64(f))
		default:
			return fmt.Errorf("hocon: expected float, got %T", sv.V)
		}
	case reflect.Bool:
		b, ok2 := sv.V.(bool)
		if !ok2 {
			return fmt.Errorf("hocon: expected bool, got %T", sv.V)
		}
		target.SetBool(b)
	default:
		return fmt.Errorf("hocon: unsupported target type %v", target.Type())
	}
	return nil
}
