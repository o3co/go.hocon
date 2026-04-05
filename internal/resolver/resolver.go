// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/properties"
)

// Val is a resolved value.
type Val interface{ val() }

// ObjectVal is a resolved object with ordered keys.
type ObjectVal struct {
	keys        []string
	values      map[string]Val
	priorValues map[string]Val // prior non-object values for optional-substitution fallback
}

func (o *ObjectVal) val() {}

func newObjectVal() *ObjectVal {
	return &ObjectVal{values: make(map[string]Val), priorValues: make(map[string]Val)}
}

func (o *ObjectVal) set(key string, v Val) {
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = v
}

func (o *ObjectVal) Get(key string) (Val, bool) {
	v, ok := o.values[key]
	return v, ok
}

func (o *ObjectVal) Keys() []string {
	r := make([]string, len(o.keys))
	copy(r, o.keys)
	return r
}

// NewObjectVal creates an empty ObjectVal (exported for use by hocon package).
func NewObjectVal() *ObjectVal { return newObjectVal() }

// Set adds or replaces a key (exported).
func (o *ObjectVal) Set(key string, v Val) { o.set(key, v) }

// GetVal returns the value for key (exported).
func (o *ObjectVal) GetVal(key string) (Val, bool) { return o.Get(key) }

// deepMerge merges src into dst (dst values take precedence for non-objects).
func deepMerge(dst, src *ObjectVal) *ObjectVal {
	result := newObjectVal()
	// add all dst keys first
	for _, k := range dst.keys {
		result.set(k, dst.values[k])
	}
	// carry over priorValues from dst
	for k, v := range dst.priorValues {
		result.priorValues[k] = v
	}
	// merge src keys
	for _, k := range src.keys {
		sv := src.values[k]
		if dv, ok := result.values[k]; ok {
			// both object → merge
			if do, dok := dv.(*ObjectVal); dok {
				if so, sok := sv.(*ObjectVal); sok {
					result.values[k] = deepMerge(do, so)
					continue
				}
			}
			// dst wins — skip
			continue
		}
		result.set(k, sv)
	}
	// carry over priorValues from src (dst's take precedence)
	for k, v := range src.priorValues {
		if _, exists := result.priorValues[k]; !exists {
			result.priorValues[k] = v
		}
	}
	return result
}

// ArrayVal is a resolved array.
type ArrayVal struct{ Elements []Val }

func (a *ArrayVal) val() {}

// ScalarVal holds a resolved primitive.
type ScalarVal struct{ V any }

func (s *ScalarVal) val() {}

// Result is the output of Resolve.
type Result struct {
	Root *ObjectVal
}

// Options controls resolution behavior.
type Options struct {
	// BaseDir is the directory for resolving relative include paths.
	// Defaults to os.Getwd().
	BaseDir string
	// Fallback is a previously resolved tree used for self-referential substitutions.
	Fallback *ObjectVal
}

// ResolveError wraps resolution failures.
type ResolveError struct {
	Message  string
	Path     string
	Line     int
	Col      int
	FilePath string
}

func (e *ResolveError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("resolve error at path %q: %s", e.Path, e.Message)
	}
	return "resolve error: " + e.Message
}

// Resolve transforms an AST into a fully resolved Result.
func Resolve(root *parser.ObjectNode, opts Options) (*Result, error) {
	if opts.BaseDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, &ResolveError{Message: "cannot determine working directory: " + err.Error()}
		}
		opts.BaseDir = wd
	}
	stack := make([]string, 0, 8)
	r := &resolver{opts: opts, resolving: make(map[string]bool), resolvedCache: make(map[string]Val), priorValues: make(map[string]Val), includeStack: &stack}
	obj, err := r.resolveObject(root, opts.Fallback, nil)
	if err != nil {
		return nil, err
	}
	// second pass: resolve substitutions with full object context
	obj2, err := r.resolveSubstitutions(obj, obj)
	if err != nil {
		return nil, err
	}
	return &Result{Root: obj2}, nil
}

type resolver struct {
	opts          Options
	resolving     map[string]bool // cycle detection
	resolvedCache map[string]Val  // previously resolved values for self-reference
	priorValues   map[string]Val  // previous value before self-referential overwrite (first pass)
	includeStack  *[]string       // shared stack for circular include detection (normalized paths)
	lenient       bool            // when true, unresolved substitutions are left as placeholders instead of erroring
}

func (r *resolver) resolveObject(node *parser.ObjectNode, fallback *ObjectVal, pathPrefix []string) (*ObjectVal, error) {
	obj := newObjectVal()
	if fallback != nil {
		// seed with fallback
		for _, k := range fallback.Keys() {
			v, _ := fallback.Get(k)
			obj.set(k, v)
		}
	}

	for _, field := range node.Fields {
		// include directive
		if inc, ok := field.Value.(*parser.IncludeNode); ok {
			included, err := r.resolveInclude(inc, pathPrefix)
			if err != nil {
				return nil, err
			}
			merged := deepMerge(obj, included)
			// preserve obj's original keys order, then add new ones
			for _, k := range merged.keys {
				obj.set(k, merged.values[k])
			}
			continue
		}

		if len(field.Key) == 0 {
			continue
		}

		// Extend pathPrefix with the field key for child resolution.
		// For multi-segment keys like a.b, all segments form the path
		// prefix for includes nested within the value.
		childPrefix := append(append([]string{}, pathPrefix...), field.Key...)
		val, err := r.resolveNode(field.Value, obj, childPrefix)
		if err != nil {
			return nil, err
		}

		if field.Append {
			// += : look up existing array and append
			existing, _ := r.lookupPath(obj, field.Key)
			if existing == nil {
				// treat as empty array
				existing = &ArrayVal{}
			}
			existArr, ok := existing.(*ArrayVal)
			if !ok {
				return nil, &ResolveError{Message: "'+=' on non-array value", Path: segmentsToKey(field.Key)}
			}
			newArr, ok2 := val.(*ArrayVal)
			if !ok2 {
				newArr = &ArrayVal{Elements: []Val{val}}
			}
			combined := &ArrayVal{Elements: append(existArr.Elements, newArr.Elements...)}
			r.setPath(obj, field.Key, combined)
			continue
		}

		// normal assignment — handle duplicate key merging
		if len(field.Key) == 1 {
			key := field.Key[0]
			if existing, ok := obj.Get(key); ok {
				if eo, eok := existing.(*ObjectVal); eok {
					if nv, nok := val.(*ObjectVal); nok {
						val = deepMerge(nv, eo) // new over existing: nv=dst wins
					}
				} else {
					// non-object overwrite: save prior value for self-referential substitution support
					r.priorValues[segmentsToKey([]string{key})] = existing
					obj.priorValues[key] = existing // per-object scope for optional-substitution fallback
				}
				// non-object: last value wins (fall through to obj.set below)
			}
			obj.set(key, val)
		} else {
			r.setPath(obj, field.Key, val)
		}
	}
	return obj, nil
}

func (r *resolver) resolveNode(node parser.Node, ctx *ObjectVal, pathPrefix []string) (Val, error) {
	switch n := node.(type) {
	case *parser.ScalarNode:
		return &ScalarVal{V: n.Value}, nil
	case *parser.ObjectNode:
		return r.resolveObject(n, nil, pathPrefix)
	case *parser.ArrayNode:
		arr := &ArrayVal{}
		for _, elem := range n.Elements {
			v, err := r.resolveNode(elem, ctx, pathPrefix)
			if err != nil {
				return nil, err
			}
			arr.Elements = append(arr.Elements, v)
		}
		return arr, nil
	case *parser.SubstNode:
		// leave substitution nodes for second pass
		return &substPlaceholder{node: n, segments: parseSubstPath(n.Path)}, nil
	case *parser.ConcatNode:
		return r.resolveConcatPartial(n, ctx, pathPrefix)
	case *parser.IncludeNode:
		return r.resolveInclude(n, pathPrefix)
	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}
}

// substPlaceholder is a temporary stand-in for unresolved substitutions.
type substPlaceholder struct {
	node      *parser.SubstNode
	segments  []string // path segments — source of truth
	prefixLen int      // 0 for normal, >0 for relativized (number of prefix segments)
}

func (s *substPlaceholder) val() {}

func (r *resolver) resolveConcatPartial(n *parser.ConcatNode, ctx *ObjectVal, pathPrefix []string) (Val, error) {
	var vals []Val
	for _, child := range n.Nodes {
		v, err := r.resolveNode(child, ctx, pathPrefix)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return &concatPlaceholder{vals: vals}, nil
}

type concatPlaceholder struct{ vals []Val }

func (c *concatPlaceholder) val() {}

// resolveSubstitutions performs the second pass, replacing placeholders.
func (r *resolver) resolveSubstitutions(obj *ObjectVal, root *ObjectVal) (*ObjectVal, error) {
	result := newObjectVal()
	for _, k := range obj.Keys() {
		v, _ := obj.Get(k)
		resolved, err := r.resolveVal(v, root, k)
		if err != nil {
			return nil, err
		}
		if resolved != nil {
			// Delayed merge (site 1): if both current and prior resolve to objects,
			// deep-merge them (prior as base, current on top).
			if resolvedObj, rOk := resolved.(*ObjectVal); rOk {
				if prior, hasPrior := obj.priorValues[k]; hasPrior {
					priorResolved, perr := r.resolveVal(prior, root, k)
					if perr != nil {
						return nil, perr
					}
					if priorResolved != nil {
						if priorObj, pOk := priorResolved.(*ObjectVal); pOk {
							resolved = deepMerge(resolvedObj, priorObj)
						}
					}
				}
			}
			result.set(k, resolved)
			// cache top-level resolved values to support self-referential substitutions
			if obj == root {
				r.resolvedCache[segmentsToKey([]string{k})] = resolved
			}
		} else if prior, ok := obj.priorValues[k]; ok {
			// optional substitution resolved to nothing — fall back to prior value (per-object scope)
			fallback, ferr := r.resolveVal(prior, root, k)
			if ferr != nil {
				return nil, ferr
			}
			if fallback != nil {
				result.set(k, fallback)
				if obj == root {
					r.resolvedCache[segmentsToKey([]string{k})] = fallback
				}
			}
		}
		// nil means the field was dropped (optional substitution resolved to nothing)
	}
	return result, nil
}

func (r *resolver) resolveVal(v Val, root *ObjectVal, path string) (Val, error) {
	switch vv := v.(type) {
	case *substPlaceholder:
		return r.resolveSubst(vv, root)
	case *concatPlaceholder:
		return r.resolveConcat(vv.vals, root, path)
	case *ObjectVal:
		return r.resolveSubstitutions(vv, root)
	case *ArrayVal:
		arr := &ArrayVal{}
		for _, elem := range vv.Elements {
			rv, err := r.resolveVal(elem, root, path)
			if err != nil {
				return nil, err
			}
			if rv != nil {
				arr.Elements = append(arr.Elements, rv)
			}
		}
		return arr, nil
	default:
		return v, nil
	}
}

func (r *resolver) resolveSubst(s *substPlaceholder, root *ObjectVal) (Val, error) {
	n := s.node
	key := segmentsToKey(s.segments)
	segments := s.segments

	if r.resolving[key] {
		// Check if a previously resolved value exists (self-referential substitution).
		// e.g. path=${path}["/extra"] — the ${path} should resolve to the prior value.
		if cached, ok := r.resolvedCache[key]; ok {
			return cached, nil
		}
		// Check for prior (pre-overwrite) value saved during first pass.
		if prior, ok := r.priorValues[key]; ok {
			// Resolve the prior value (it may itself contain placeholders).
			return r.resolveVal(prior, root, key)
		}
		if !n.Optional {
			return nil, &ResolveError{Message: "circular reference detected", Path: key, Line: n.Line(), Col: n.Col()}
		}
		return nil, nil
	}
	r.resolving[key] = true
	defer delete(r.resolving, key)

	val, ok := r.lookupPath(root, segments)
	if ok {
		// If the found value is still a placeholder that references the SAME path
		// (actual self-reference like b=${b}), use the prior value instead.
		// A placeholder referencing a DIFFERENT path is not self-referential and
		// should be resolved normally.
		isSelfRef := false
		switch v := val.(type) {
		case *substPlaceholder:
			if slices.Equal(v.segments, s.segments) {
				isSelfRef = true
			}
		case *concatPlaceholder:
			for _, cv := range v.vals {
				if sp, spOk := cv.(*substPlaceholder); spOk && slices.Equal(sp.segments, s.segments) {
					isSelfRef = true
					break
				}
			}
		}
		if isSelfRef {
			if prior, ok2 := r.priorValues[key]; ok2 {
				return r.resolveVal(prior, root, key)
			}
			// For nested paths, check the parent object's per-object priorValues.
			if len(segments) > 1 {
				if parent, pok := r.lookupPathObj(root, segments[:len(segments)-1]); pok {
					lastKey := segments[len(segments)-1]
					if prior2, ok3 := parent.priorValues[lastKey]; ok3 {
						return r.resolveVal(prior2, root, key)
					}
				}
			}
		}
		resolved, err := r.resolveVal(val, root, key)
		if err != nil {
			return nil, err
		}
		if resolved == nil {
			// The looked-up value resolved to nil (e.g. optional substitution with
			// missing env var). Check the target path's per-object priorValues for a
			// fallback — this mirrors the fallback logic in resolveSubstitutions but
			// applies when accessing the value via a substitution lookup.
			if len(segments) > 0 {
				var parentObj *ObjectVal
				if len(segments) == 1 {
					parentObj = root
				} else {
					parentObj, _ = r.lookupPathObj(root, segments[:len(segments)-1])
				}
				if parentObj != nil {
					lastKey := segments[len(segments)-1]
					if prior, pOk := parentObj.priorValues[lastKey]; pOk {
						return r.resolveVal(prior, root, key)
					}
				}
			}
		}
		// Delayed merge (site 2): after resolving a substitution lookup, if the
		// result is an object and there is a prior value that also resolves to an
		// object, deep-merge them (prior as base, current on top).
		// Restricted to single-segment paths to avoid incorrect merges on nested paths.
		if len(segments) == 1 {
			if resolvedObj, rOk := resolved.(*ObjectVal); rOk {
				prior := r.findPrior(root, segments, key)
				if prior != nil {
					priorResolved, perr := r.resolveVal(prior, root, key)
					if perr != nil {
						return nil, perr
					}
					if priorResolved != nil {
						if priorObj, poOk := priorResolved.(*ObjectVal); poOk {
							resolved = deepMerge(resolvedObj, priorObj)
						}
					}
				}
			}
		}
		return resolved, nil
	}

	// Relativized path not found — fall back to original (non-relativized) path.
	if s.prefixLen > 0 && len(segments) > s.prefixLen {
		originalSegments := segments[s.prefixLen:]
		originalKey := segmentsToKey(originalSegments)
		origVal, origOk := r.lookupPath(root, originalSegments)
		if origOk {
			resolved, err := r.resolveVal(origVal, root, originalKey)
			if err != nil {
				return nil, err
			}
			return resolved, nil
		}
		// Also try env var with original path (raw dot-join, no quoting)
		if ev, ok := os.LookupEnv(strings.Join(originalSegments, ".")); ok {
			return &ScalarVal{V: ev}, nil
		}
	}

	// env var fallback — use raw dot-join (no quoting) to match Lightbend behavior
	if ev, ok := os.LookupEnv(strings.Join(s.segments, ".")); ok {
		return &ScalarVal{V: ev}, nil
	}
	if n.Optional {
		return nil, nil // field will be dropped
	}
	if r.lenient {
		// In lenient mode (child resolver for includes), leave unresolved
		// substitutions as placeholders for the parent resolver to handle.
		return s, nil
	}
	return nil, &ResolveError{Message: "unresolved substitution", Path: key, Line: n.Line(), Col: n.Col()}
}

// findPrior looks up the per-object priorValues for a given path in the tree.
func (r *resolver) findPrior(root *ObjectVal, segments []string, pathStr string) Val {
	// Check resolver-level priorValues first (top-level keys).
	if prior, ok := r.priorValues[pathStr]; ok {
		return prior
	}
	// Check the parent object's per-object priorValues for nested paths.
	if len(segments) > 0 {
		var parentObj *ObjectVal
		if len(segments) == 1 {
			parentObj = root
		} else {
			parentObj, _ = r.lookupPathObj(root, segments[:len(segments)-1])
		}
		if parentObj != nil {
			lastKey := segments[len(segments)-1]
			if prior, ok := parentObj.priorValues[lastKey]; ok {
				return prior
			}
		}
	}
	return nil
}

func isSeparator(v Val) bool {
	if s, ok := v.(*ScalarVal); ok {
		if str, ok := s.V.(string); ok && str == " " {
			return true
		}
	}
	return false
}

func (r *resolver) resolveConcat(vals []Val, root *ObjectVal, path string) (Val, error) {
	// resolve each val
	var resolved []Val
	for _, v := range vals {
		rv, err := r.resolveVal(v, root, path)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, rv)
	}

	// Classify non-nil, non-separator elements to determine concatenation mode.
	hasArray := false
	hasObject := false
	hasScalar := false
	for _, rv := range resolved {
		if rv == nil || isSeparator(rv) {
			continue
		}
		switch rv.(type) {
		case *ArrayVal:
			hasArray = true
		case *ObjectVal:
			hasObject = true
		default:
			hasScalar = true
		}
	}

	switch {
	case hasObject && !hasArray && !hasScalar:
		// All meaningful elements are objects → deep merge (left to right, later wins)
		return concatObjects(resolved), nil
	case hasArray:
		// Array concatenation (permissive: non-array elements become single items)
		return concatArraysPermissive(resolved), nil
	default:
		// String concatenation (fallback)
		return r.concatStrings(resolved), nil
	}
}

// mergeObjectConcat merges two objects for object concatenation.
// Key order follows left (earlier), new keys from right are appended.
// For duplicate keys, right (later) values win.
func mergeObjectConcat(left, right *ObjectVal) *ObjectVal {
	result := newObjectVal()
	// seed with left keys in order
	for _, k := range left.keys {
		result.set(k, left.values[k])
	}
	// merge right keys: override existing values, append new keys
	for _, k := range right.keys {
		rv := right.values[k]
		if lv, ok := result.values[k]; ok {
			// both object → recursive merge preserving order
			if lo, lok := lv.(*ObjectVal); lok {
				if ro, rok := rv.(*ObjectVal); rok {
					result.values[k] = mergeObjectConcat(lo, ro)
					continue
				}
			}
			// right wins for value
			result.values[k] = rv
		} else {
			result.set(k, rv)
		}
	}
	return result
}

func concatObjects(vals []Val) Val {
	var result *ObjectVal
	for _, v := range vals {
		if v == nil || isSeparator(v) {
			continue
		}
		obj, ok := v.(*ObjectVal)
		if !ok {
			continue
		}
		if result == nil {
			result = obj
		} else {
			result = mergeObjectConcat(result, obj)
		}
	}
	if result == nil {
		return newObjectVal()
	}
	return result
}

func concatArraysPermissive(vals []Val) Val {
	result := &ArrayVal{}
	for _, v := range vals {
		if v == nil || isSeparator(v) {
			continue
		}
		if arr, ok := v.(*ArrayVal); ok {
			result.Elements = append(result.Elements, arr.Elements...)
		} else {
			result.Elements = append(result.Elements, v)
		}
	}
	return result
}

func (r *resolver) concatStrings(vals []Val) Val {
	var sb strings.Builder
	for _, v := range vals {
		if v == nil {
			continue
		}
		sb.WriteString(valToString(v))
	}
	return &ScalarVal{V: sb.String()}
}

func valToString(v Val) string {
	switch vv := v.(type) {
	case *ScalarVal:
		if vv.V == nil {
			return "null"
		}
		return fmt.Sprintf("%v", vv.V)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (r *resolver) lookupPath(obj *ObjectVal, segments []string) (Val, bool) {
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
	child, ok2 := v.(*ObjectVal)
	if !ok2 {
		return nil, false
	}
	return r.lookupPath(child, segments[1:])
}

func (r *resolver) lookupPathObj(obj *ObjectVal, segments []string) (*ObjectVal, bool) {
	v, ok := r.lookupPath(obj, segments)
	if !ok {
		return nil, false
	}
	ov, ok := v.(*ObjectVal)
	return ov, ok
}

func (r *resolver) setPath(obj *ObjectVal, segments []string, val Val) {
	if len(segments) == 1 {
		key := segments[0]
		// Deep merge if both existing and new values are objects
		if existing, ok := obj.Get(key); ok {
			if eo, eok := existing.(*ObjectVal); eok {
				if nv, nok := val.(*ObjectVal); nok {
					val = deepMerge(nv, eo) // new over existing: nv=dst wins
				}
			} else {
				// non-object overwrite: save prior value for self-referential substitution support
				r.priorValues[segmentsToKey(segments)] = existing
			}
		}
		obj.set(key, val)
		return
	}
	child, ok := obj.Get(segments[0])
	var childObj *ObjectVal
	if ok {
		childObj, _ = child.(*ObjectVal)
	}
	if childObj == nil {
		childObj = newObjectVal()
	}
	r.setPath(childObj, segments[1:], val)
	obj.set(segments[0], childObj)
}

// includeExtensions is the list of extensions to probe when an include path
// has no extension, per the HOCON spec (properties first, then JSON, then HOCON).
var includeExtensions = []string{".properties", ".json", ".conf"}

func (r *resolver) resolveInclude(inc *parser.IncludeNode, pathPrefix []string) (*ObjectVal, error) {
	path := inc.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.opts.BaseDir, path)
	}

	var included *ObjectVal

	// Single file with explicit extension.
	if filepath.Ext(path) != "" {
		obj, err := r.loadIncludeFile(path, inc.Required)
		if err != nil {
			return nil, err
		}
		included = obj
	} else {
		// No extension: probe all known extensions and merge all found files.
		// Per HOCON spec, .properties first, then .json, then .conf (HOCON last).
		merged := newObjectVal()
		found := false
		for _, ext := range includeExtensions {
			p := path + ext
			if _, err := os.Stat(p); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, &ResolveError{
					Message:  "cannot stat include file: " + err.Error(),
					FilePath: p,
				}
			}
			obj, err := r.loadIncludeFile(p, true)
			if err != nil {
				return nil, err
			}
			found = true
			merged = deepMerge(obj, merged)
		}
		if !found {
			if inc.Required {
				return nil, &ResolveError{
					Message:  fmt.Sprintf("cannot read include file: no file found for %q (tried %v)", inc.Path, includeExtensions),
					FilePath: path,
				}
			}
			return newObjectVal(), nil
		}
		included = merged
	}

	// Relativize substitution placeholders in the included object so that
	// paths like ${x} become ${prefix.x} when included under a nested scope.
	if len(pathPrefix) > 0 {
		relativizeVals(included, pathPrefix)
	}

	return included, nil
}

// relativizeVals recursively walks all values in obj and prepends prefix segments to
// any substPlaceholder paths, recording prefixLen so the resolver can fall
// back to the original (non-relativized) path if the relativized one is not found.
func relativizeVals(obj *ObjectVal, prefixSegments []string) {
	for _, k := range obj.keys {
		obj.values[k] = relativizeVal(obj.values[k], prefixSegments)
	}
	for k, v := range obj.priorValues {
		obj.priorValues[k] = relativizeVal(v, prefixSegments)
	}
}

func relativizeVal(v Val, prefixSegments []string) Val {
	switch vv := v.(type) {
	case *substPlaceholder:
		// Create a new SubstNode with the relativized path.
		// Accumulate prefixLen so multi-layer includes compose correctly.
		newNode := &parser.SubstNode{}
		*newNode = *vv.node
		newSegments := make([]string, 0, len(prefixSegments)+len(vv.segments))
		newSegments = append(newSegments, prefixSegments...)
		newSegments = append(newSegments, vv.segments...)
		newNode.Path = segmentsToKey(newSegments)
		return &substPlaceholder{node: newNode, segments: newSegments, prefixLen: vv.prefixLen + len(prefixSegments)}
	case *concatPlaceholder:
		newVals := make([]Val, len(vv.vals))
		for i, cv := range vv.vals {
			newVals[i] = relativizeVal(cv, prefixSegments)
		}
		return &concatPlaceholder{vals: newVals}
	case *ObjectVal:
		relativizeVals(vv, prefixSegments)
		return vv
	case *ArrayVal:
		for i, elem := range vv.Elements {
			vv.Elements[i] = relativizeVal(elem, prefixSegments)
		}
		return vv
	default:
		return v
	}
}

// loadIncludeFile reads, parses, and resolves a single include file.
// When required=false a missing file is silently ignored (returns empty object).
func (r *resolver) loadIncludeFile(path string, required bool) (*ObjectVal, error) {
	// Normalize path for reliable circular detection.
	// Clean removes ".." / "." segments; Abs makes relative paths absolute.
	canonicalPath := filepath.Clean(path)
	if abs, err := filepath.Abs(canonicalPath); err == nil {
		canonicalPath = abs
	}

	// Circular include detection using shared stack.
	for _, p := range *r.includeStack {
		if p == canonicalPath {
			return nil, &ResolveError{
				Message:  fmt.Sprintf("circular include: %s", path),
				FilePath: path,
			}
		}
	}
	*r.includeStack = append(*r.includeStack, canonicalPath)
	defer func() {
		*r.includeStack = (*r.includeStack)[:len(*r.includeStack)-1]
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		if !required && os.IsNotExist(err) {
			// Non-required include: silently ignore missing file per HOCON spec.
			return newObjectVal(), nil
		}
		// For required includes, or non-ENOENT errors on optional includes,
		// always propagate (permission errors etc. should not be silently swallowed).
		return nil, &ResolveError{
			Message:  "cannot read include file: " + err.Error(),
			FilePath: path,
		}
	}

	// .properties files: use dedicated parser instead of HOCON lexer.
	// Standard .properties syntax (e.g. ! comments, URL values) is not valid HOCON.
	if filepath.Ext(path) == ".properties" {
		return propsToObjectVal(properties.Parse(string(data))), nil
	}

	obj, err := r.parseAndResolve(data, path)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// parseAndResolve parses raw HOCON/JSON data and resolves it into an ObjectVal.
// Substitutions that can be resolved within the included file are resolved here.
// Substitutions that reference external paths are left as placeholders for the
// parent resolver to relativize and resolve against the full tree.
func (r *resolver) parseAndResolve(data []byte, filePath string) (*ObjectVal, error) {
	ast, err := parser.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	childResolver := &resolver{
		opts:          Options{BaseDir: filepath.Dir(filePath)},
		resolving:     make(map[string]bool),
		resolvedCache: make(map[string]Val),
		priorValues:   make(map[string]Val),
		includeStack:  r.includeStack,
		lenient:       true, // don't error on unresolved substitutions; leave as placeholders
	}
	obj, err := childResolver.resolveObject(ast, nil, nil)
	if err != nil {
		return nil, err
	}
	return childResolver.resolveSubstitutions(obj, obj)
}

// propsToObjectVal converts a flat map[string]string (from a .properties file)
// into a nested ObjectVal. Dotted keys are expanded into nested objects:
// "server.host" = "x" becomes {server: {host: "x"}}.
// All leaf values are ScalarVal with string type, per .properties spec.
func propsToObjectVal(props map[string]string) *ObjectVal {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	root := newObjectVal()
	for _, key := range keys {
		value := props[key]
		parts := strings.Split(key, ".")
		cur := root
		for i, part := range parts {
			if i == len(parts)-1 {
				cur.set(part, &ScalarVal{V: value})
			} else {
				existing, ok := cur.values[part]
				if !ok {
					child := newObjectVal()
					cur.set(part, child)
					cur = child
				} else if child, ok := existing.(*ObjectVal); ok {
					cur = child
				} else {
					// Conflict: scalar already set for this segment — leaf wins, skip.
					break
				}
			}
		}
	}
	return root
}
