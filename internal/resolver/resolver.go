// Copyright 2026 o3co Inc.
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
	"strings"

	"github.com/o3co/go.hocon/internal/parser"
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
	r := &resolver{opts: opts, resolving: make(map[string]bool), resolvedCache: make(map[string]Val), priorValues: make(map[string]Val)}
	obj, err := r.resolveObject(root, opts.Fallback)
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
}

func (r *resolver) resolveObject(node *parser.ObjectNode, fallback *ObjectVal) (*ObjectVal, error) {
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
			included, err := r.resolveInclude(inc)
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

		val, err := r.resolveNode(field.Value, obj)
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
				return nil, &ResolveError{Message: "'+=' on non-array value", Path: strings.Join(field.Key, ".")}
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
					r.priorValues[key] = existing
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

func (r *resolver) resolveNode(node parser.Node, ctx *ObjectVal) (Val, error) {
	switch n := node.(type) {
	case *parser.ScalarNode:
		return &ScalarVal{V: n.Value}, nil
	case *parser.ObjectNode:
		return r.resolveObject(n, nil)
	case *parser.ArrayNode:
		arr := &ArrayVal{}
		for _, elem := range n.Elements {
			v, err := r.resolveNode(elem, ctx)
			if err != nil {
				return nil, err
			}
			arr.Elements = append(arr.Elements, v)
		}
		return arr, nil
	case *parser.SubstNode:
		// leave substitution nodes for second pass
		return &substPlaceholder{node: n}, nil
	case *parser.ConcatNode:
		return r.resolveConcatPartial(n, ctx)
	case *parser.IncludeNode:
		return r.resolveInclude(n)
	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}
}

// substPlaceholder is a temporary stand-in for unresolved substitutions.
type substPlaceholder struct{ node *parser.SubstNode }

func (s *substPlaceholder) val() {}

func (r *resolver) resolveConcatPartial(n *parser.ConcatNode, ctx *ObjectVal) (Val, error) {
	var vals []Val
	for _, child := range n.Nodes {
		v, err := r.resolveNode(child, ctx)
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
			result.set(k, resolved)
			// cache top-level resolved values to support self-referential substitutions
			if obj == root {
				r.resolvedCache[k] = resolved
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
					r.resolvedCache[k] = fallback
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
		return r.resolveSubst(vv.node, root)
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

func (r *resolver) resolveSubst(n *parser.SubstNode, root *ObjectVal) (Val, error) {
	pathStr := n.Path
	if r.resolving[pathStr] {
		// Check if a previously resolved value exists (self-referential substitution).
		// e.g. path=${path}["/extra"] — the ${path} should resolve to the prior value.
		if cached, ok := r.resolvedCache[pathStr]; ok {
			return cached, nil
		}
		// Check for prior (pre-overwrite) value saved during first pass.
		if prior, ok := r.priorValues[pathStr]; ok {
			// Resolve the prior value (it may itself contain placeholders).
			return r.resolveVal(prior, root, pathStr)
		}
		if !n.Optional {
			return nil, &ResolveError{Message: "circular reference detected", Path: pathStr, Line: n.Line(), Col: n.Col()}
		}
		return nil, nil
	}
	r.resolving[pathStr] = true
	defer delete(r.resolving, pathStr)

	segments := strings.Split(pathStr, ".")
	val, ok := r.lookupPath(root, segments)
	if ok {
		// If the found value is still a placeholder (self-referential definition),
		// use the prior value instead.
		switch val.(type) {
		case *substPlaceholder, *concatPlaceholder:
			if prior, ok2 := r.priorValues[pathStr]; ok2 {
				return r.resolveVal(prior, root, pathStr)
			}
			// For nested paths, check the parent object's per-object priorValues.
			if len(segments) > 1 {
				if parent, pok := r.lookupPathObj(root, segments[:len(segments)-1]); pok {
					lastKey := segments[len(segments)-1]
					if prior2, ok3 := parent.priorValues[lastKey]; ok3 {
						return r.resolveVal(prior2, root, pathStr)
					}
				}
			}
		}
		return r.resolveVal(val, root, pathStr)
	}
	// env var fallback
	if ev := os.Getenv(pathStr); ev != "" {
		return &ScalarVal{V: ev}, nil
	}
	if n.Optional {
		return nil, nil // field will be dropped
	}
	return nil, &ResolveError{Message: "unresolved substitution", Path: pathStr, Line: n.Line(), Col: n.Col()}
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
	// determine mode from first non-nil element
	for _, rv := range resolved {
		if rv == nil {
			continue
		}
		switch rv.(type) {
		case *ArrayVal:
			return r.concatArrays(resolved)
		case *ObjectVal:
			return nil, &ResolveError{Message: "objects cannot appear in concatenation", Path: path}
		default:
			return r.concatStrings(resolved), nil
		}
	}
	return &ScalarVal{V: ""}, nil
}

func (r *resolver) concatArrays(vals []Val) (Val, error) {
	result := &ArrayVal{}
	for _, v := range vals {
		if v == nil {
			continue
		}
		arr, ok := v.(*ArrayVal)
		if !ok {
			return nil, &ResolveError{Message: "cannot concatenate non-array with array"}
		}
		result.Elements = append(result.Elements, arr.Elements...)
	}
	return result, nil
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
				r.priorValues[strings.Join(segments, ".")] = existing
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

func (r *resolver) resolveInclude(inc *parser.IncludeNode) (*ObjectVal, error) {
	path := inc.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.opts.BaseDir, path)
	}

	paths := []string{path}
	if filepath.Ext(path) == "" {
		// No extension: probe known extensions per HOCON spec.
		paths = nil
		for _, ext := range includeExtensions {
			paths = append(paths, path+ext)
		}
	}

	var data []byte
	var resolvedPath string
	var lastErr error
	for _, p := range paths {
		d, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		data = d
		resolvedPath = p
		break
	}
	if data == nil {
		msg := "cannot read include file: " + lastErr.Error()
		if filepath.Ext(inc.Path) == "" {
			msg = fmt.Sprintf("cannot read include file: no file found for %q (tried %v)", inc.Path, includeExtensions)
		}
		return nil, &ResolveError{
			Message:  msg,
			FilePath: path,
		}
	}

	ast, err := parser.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	childResolver := &resolver{
		opts:          Options{BaseDir: filepath.Dir(resolvedPath)},
		resolving:     make(map[string]bool),
		resolvedCache: make(map[string]Val),
		priorValues:   make(map[string]Val),
	}
	obj, err := childResolver.resolveObject(ast, nil)
	if err != nil {
		return nil, err
	}
	obj2, err := childResolver.resolveSubstitutions(obj, obj)
	if err != nil {
		return nil, err
	}
	return obj2, nil
}
