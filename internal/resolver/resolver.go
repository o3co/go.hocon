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
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/o3co/go.hocon/internal/lexer"
	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/properties"
)

// segTexts extracts the Text field from a slice of lexer.Segment into a []string.
// Used wherever the resolver needs plain string path segments.
func segTexts(segs []lexer.Segment) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = s.Text
	}
	return out
}

// strSegs converts a []string into []lexer.Segment with zero positions.
// Used when constructing substPlaceholder from field keys (which are []string).
func strSegs(texts []string) []lexer.Segment {
	out := make([]lexer.Segment, len(texts))
	for i, t := range texts {
		out[i] = lexer.Segment{Text: t}
	}
	return out
}

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

// SetPrior records a "prior value" for key (used by E12 WithFallback to
// propagate fallback values as priors for self-reference lookback in phase 2).
func (o *ObjectVal) SetPrior(key string, v Val) {
	o.priorValues[key] = v
}

// GetPrior returns the prior value associated with key, if any.
func (o *ObjectVal) GetPrior(key string) (Val, bool) {
	v, ok := o.priorValues[key]
	return v, ok
}

// MergeUnresolved performs the E12 WithFallback merge of two unresolved trees.
// Receiver's keys win; on non-object collision (or when receiver is non-object),
// the fallback's value is recorded as a prior on the result for cross-layer
// self-reference lookback in phase 2. Both-object collisions recurse — unless
// a composition barrier is active at the key.
//
// Composition barrier (HOCON.md §Object Merge L1485, dr10): when the receiver
// has a non-object priorValues[k] from an earlier merge layer (e.g. the
// scalar in `obj.WithFallback(scalar).WithFallback(otherObj)`), the recursion
// into both-object collision is suppressed at that key and the fallback's
// object is discarded.  The barrier is signalled by receiver.priorValues[k]
// being non-object — no external bookkeeping required from callers.
func MergeUnresolved(receiver, fallback *ObjectVal) *ObjectVal {
	if receiver == nil {
		return fallback
	}
	if fallback == nil {
		return receiver
	}
	result := newObjectVal()
	// 1. Seed with fallback keys (so receiver-only / new fallback-only keys
	//    co-exist).
	for _, k := range fallback.Keys() {
		v, _ := fallback.Get(k)
		result.set(k, v)
	}
	// Carry fallback's priorValues.
	for k, v := range fallback.priorValues {
		result.priorValues[k] = v
	}
	// 2. Apply receiver: receiver wins; on non-object collision capture
	//    fallback's value (existing) as prior for cross-layer self-ref lookback.
	for _, k := range receiver.Keys() {
		rv, _ := receiver.Get(k)
		if existing, hasExisting := result.values[k]; hasExisting {
			// both objects → recurse, but only when no composition barrier is in effect.
			// A composition barrier exists when the receiver already has a non-object
			// priorValues[k] from a previous merge (e.g. r.WithFallback(scalar).WithFallback(obj)):
			// the scalar in the middle layer bars subsequent object layers from contributing.
			// HOCON.md §Object Merge L1485 / dr10.
			if recObj, recOk := rv.(*ObjectVal); recOk {
				if existObj, existOk := existing.(*ObjectVal); existOk {
					// Check composition barrier: receiver's own prior for k is non-object.
					barrierActive := false
					if recPrior, hasPrior := receiver.priorValues[k]; hasPrior {
						if _, priorIsObj := recPrior.(*ObjectVal); !priorIsObj {
							barrierActive = true
						}
					}
					if !barrierActive {
						result.set(k, MergeUnresolved(recObj, existObj))
						continue
					}
					// Barrier active: receiver's object wins; discard fallback's object.
					// Do NOT update result.priorValues[k] — the existing prior (the scalar)
					// already represents the barrier and must not be overwritten by the object.
					_ = existObj // fallback's object is intentionally discarded
					result.set(k, rv)
					continue
				}
			}
			// non-object collision: receiver wins; capture fallback's
			// value (existing) as prior for cross-layer self-ref lookback.
			result.priorValues[k] = existing
		}
		result.set(k, rv)
	}
	// 3. Receiver's own priorValues take precedence over any priors propagated
	//    from fallback. This preserves the receiver's full self-reference
	//    history across iterated WithFallback calls — including priors that
	//    were installed by an earlier merge where the receiver itself was a
	//    merged result and a key came from that earlier fallback.
	for k, v := range receiver.priorValues {
		result.priorValues[k] = v
	}
	return result
}

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

// ScalarType discriminates the kind of scalar value.
type ScalarType int

const (
	ScalarString ScalarType = iota
	ScalarNumber
	ScalarBoolean
	ScalarNull
)

// ScalarVal holds a resolved primitive as its raw string representation
// plus a type discriminant. The raw string is what appeared in the source
// (or was produced by concatenation / env var lookup).
type ScalarVal struct {
	// Raw is the scalar's string representation. For unquoted tokens this is
	// the literal source text. For quoted strings the lexer has already
	// decoded escape sequences, so Raw contains the decoded value.
	Raw  string
	Type ScalarType
}

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
	// PackageLookup, if non-nil, is called by resolveInclude for package(...) includes.
	// It receives the identifier and file and returns the registered HOCON source bytes,
	// or (nil, non-nil error) on miss. When nil, the resolver returns a ResolveError for
	// any package include encountered.
	// E11: child resolvers created during include resolution MUST copy this field from the
	// parent so that package includes within included files are resolved correctly.
	PackageLookup func(identifier, file string) ([]byte, error)
	// UseSystemEnvironment, when false, disables fallback to os.LookupEnv for
	// unresolved substitution paths (E12 ResolveOptions.UseSystemEnvironment).
	// Default zero-value is false; the public Config layer must pass true to
	// preserve current behaviour for fused parseAndResolve.
	UseSystemEnvironment bool
	// AllowUnresolved, when true, leaves unresolved (non-optional) substitution
	// placeholders in place instead of returning a ResolveError (E12 ResolveOptions.AllowUnresolved).
	AllowUnresolved bool
}

// ResolveError wraps resolution failures.
type ResolveError struct {
	Message  string
	Path     string
	Line     int
	Col      int
	FilePath string
	// Cause is the underlying error that triggered this resolve error, if any.
	// Callers can use errors.Is/As to inspect the root cause.
	Cause error
}

func (e *ResolveError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("resolve error at path %q: %s", e.Path, e.Message)
	}
	return "resolve error: " + e.Message
}

// Unwrap returns the underlying cause, enabling errors.Is/As traversal.
func (e *ResolveError) Unwrap() error {
	return e.Cause
}

// Resolve transforms an AST into a fully resolved Result. Preserved for
// backward compatibility — equivalent to BuildTree(...) followed by
// ResolveTree(...) with UseSystemEnvironment=true (current behaviour).
func Resolve(root *parser.ObjectNode, opts Options) (*Result, error) {
	// Phase 1: build tree.
	tree, err := BuildTree(root, opts)
	if err != nil {
		return nil, err
	}
	// Phase 2: resolve substitutions. Preserve current behaviour: env is
	// always consulted for fused parseAndResolve; AllowUnresolved=false.
	phase2Opts := opts
	phase2Opts.UseSystemEnvironment = true
	phase2Opts.AllowUnresolved = false
	return ResolveTree(tree, phase2Opts)
}

// BuildTree runs phase 1 — parsing AST into an ObjectVal tree containing
// substitution/concat placeholders for any unresolved values. Includes are
// fully expanded (file/url/package). Phase 2 (ResolveTree) replaces the
// placeholders.
func BuildTree(root *parser.ObjectNode, opts Options) (*ObjectVal, error) {
	if opts.BaseDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, &ResolveError{Message: "cannot determine working directory: " + err.Error()}
		}
		opts.BaseDir = wd
	}
	stack := make([]string, 0, 8)
	r := &resolver{
		opts:          opts,
		resolving:     make(map[string]bool),
		resolvedCache: make(map[string]Val),
		priorValues:   make(map[string]Val),
		includeStack:  &stack,
	}
	return r.resolveObject(root, opts.Fallback, nil)
}

// ResolveTree runs phase 2 — replacing substitution and concat placeholders
// in tree with their resolved values. tree may have been merged from multiple
// BuildTree results via MergeUnresolved (E12 WithFallback). The opts'
// UseSystemEnvironment and AllowUnresolved fields control phase-2 behaviour;
// BaseDir / Fallback / PackageLookup are not consulted (those are phase-1 inputs).
func ResolveTree(tree *ObjectVal, opts Options) (*Result, error) {
	r := &resolver{
		opts:          opts,
		resolving:     make(map[string]bool),
		resolvedCache: make(map[string]Val),
		priorValues:   make(map[string]Val),
		includeStack:  nil, // not used in phase 2
		lenient:       opts.AllowUnresolved,
	}
	// Re-populate top-level priorValues from the tree (phase 1 stored them on
	// each ObjectVal; phase 2's self-reference lookback consults both the
	// per-object map AND the resolver's top-level map).
	for k, v := range tree.priorValues {
		r.priorValues[k] = v
	}
	out, err := r.resolveSubstitutions(tree, tree)
	if err != nil {
		return nil, err
	}
	return &Result{Root: out}, nil
}

// ContainsPlaceholders reports whether any value in the tree is an unresolved
// substitution or concat placeholder. Used by Config.IsResolved.
func ContainsPlaceholders(tree *ObjectVal) bool {
	if tree == nil {
		return false
	}
	for _, k := range tree.Keys() {
		v, _ := tree.Get(k)
		if valContainsPlaceholders(v) {
			return true
		}
	}
	return false
}

func valContainsPlaceholders(v Val) bool {
	switch vv := v.(type) {
	case *substPlaceholder, *concatPlaceholder:
		return true
	case *ObjectVal:
		return ContainsPlaceholders(vv)
	case *ArrayVal:
		for _, e := range vv.Elements {
			if valContainsPlaceholders(e) {
				return true
			}
		}
	}
	return false
}

type resolver struct {
	opts           Options
	resolving      map[string]bool // cycle detection
	resolvedCache  map[string]Val  // previously resolved values for self-reference
	priorValues    map[string]Val  // previous value before self-referential overwrite (first pass)
	includeStack   *[]string       // shared stack for circular include detection (normalized paths)
	lenient        bool            // when true, unresolved substitutions are left as placeholders instead of erroring
	inIncludeChild bool            // when true, this resolver is the sub-resolver for an `include` directive; optional substitutions are preserved (#45) rather than dropped, so they can resolve against the parent's prior on the deep-merge pass
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
			// Apply the included object's fields as if they had been written
			// inline at the include's position: included assignments override
			// earlier parent assignments, and for non-object collisions the
			// parent's prior value is recorded so a self-referential ${k}
			// inside the include resolves to the parent's value (#106).
			for _, k := range included.keys {
				iv := included.values[k]
				// Full path key for self-ref detection: substPlaceholder.segments
				// are already relativized to pathPrefix+k by relativizeVals at
				// resolveInclude's end (see L1376-1378), so fold comparisons
				// must use the same fully-qualified path.
				fullKey := segmentsToKey(append(append([]string{}, pathPrefix...), k))
				if existing, exists := obj.Get(k); exists {
					if eo, eok := existing.(*ObjectVal); eok {
						if io, iok := iv.(*ObjectVal); iok {
							// both objects → deep merge with included on top
							obj.set(k, deepMerge(io, eo))
							// #120: save existing as prior even on object+object
							// merge, so a self-referential `${k}` in the included
							// file's body (e.g. `o = { history = ${o}, v = 2 }`
							// inside an included file shadowing parent's `o`)
							// can resolve to the parent's prior value. Without
							// this save the merged val retains `${k}` but no
							// prior is recorded → "unresolved self-ref" error.
							prior, doSave := foldOrSkipPrior(existing, fullKey, obj.priorValues[k])
							if doSave {
								if len(pathPrefix) == 0 {
									r.priorValues[fullKey] = prior
								}
								obj.priorValues[k] = prior
							}
							continue
						}
					}
					// Non-object override: prior value for self-reference lookback.
					// If included carries its own prior chain for k, that prior
					// is the immediately-prior value within the include's own
					// dup-key chain (what an inline-equivalent reader would
					// see). Otherwise the parent's existing value becomes the
					// prior.
					prior := existing
					if inclPrior, hasInclPrior := included.priorValues[k]; hasInclPrior {
						prior = inclPrior
					}
					// #118: fold self-ref in `prior` against the previous prior so
					// the saved value is self-ref-free. Without this, chained
					// includes (`branches = ${branches} ["x"]` in each file) save
					// a self-referential concat and resolveSubst loops forever.
					prior, doSave := foldOrSkipPrior(prior, fullKey, obj.priorValues[k])
					if doSave {
						// Write r.priorValues ONLY at top of the current resolver's
						// scope. When the include is nested (pathPrefix != []), `k`
						// is just a leaf name and would collide with an unrelated
						// top-level key of the same name; see setPath L1239-1247
						// for the same rule applied to regular field overwrite.
						if len(pathPrefix) == 0 {
							r.priorValues[fullKey] = prior
						}
						obj.priorValues[k] = prior
					}
				} else if inclPrior, hasInclPrior := included.priorValues[k]; hasInclPrior {
					// No collision in parent, but include's own dup-key chain
					// produced a prior. Preserve it (with the same scope rule).
					// #118: same fold-or-skip treatment as the collision branch.
					// obj.priorValues[k] may carry an older prior from a prior
					// include-merge or direct overwrite even though obj.values[k]
					// is unset (priors survive subsequent shadowing).
					prior, doSave := foldOrSkipPrior(inclPrior, fullKey, obj.priorValues[k])
					if doSave {
						if len(pathPrefix) == 0 {
							r.priorValues[fullKey] = prior
						}
						obj.priorValues[k] = prior
					}
				}
				obj.set(k, iv)
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
			r.setPath(obj, field.Key, combined, field.Key)
			continue
		}

		// normal assignment — handle duplicate key merging
		if len(field.Key) == 1 {
			key := field.Key[0]
			// fullKey is the fully-qualified path for self-ref detection and
			// r.priorValues bookkeeping. Includes pathPrefix so the
			// nested-object form `r { x = ${r.x} [...] }` (where the inner
			// resolveObject runs with pathPrefix=[r]) detects self-references
			// to "r.x" instead of the bare leaf "x". Surfaced by Copilot
			// review on PR #121.
			fullKey := segmentsToKey(append(append([]string{}, pathPrefix...), key))
			if existing, ok := obj.Get(key); ok {
				if eo, eok := existing.(*ObjectVal); eok {
					if nv, nok := val.(*ObjectVal); nok {
						val = deepMerge(nv, eo) // new over existing: nv=dst wins
					}
				}
				// Always save existing as prior (whether or not val deep-merged
				// with it) so a self-referential `${key}` in val — wrapped in
				// concat, embedded as an array element, or as an object field
				// value — can refer back. Pre-#120 the save was gated on
				// !merged, which silently dropped the object-merge case
				// (`o = {v:1}; o = {history: ${o}, v:2}`) where the merged val
				// retained `${o}` but no prior was recorded. The unconditional
				// save costs one map write per overwrite — negligible vs the
				// correctness gain. foldOrSkipPrior folds an existing that
				// itself contains `${key}` against the OLD prior so the saved
				// value is always self-ref-free (#118 chain invariant).
				priorToSave, doSave := foldOrSkipPrior(existing, fullKey, r.priorValues[fullKey])
				if doSave {
					r.priorValues[fullKey] = priorToSave
					obj.priorValues[key] = priorToSave // per-object scope for optional-substitution fallback
				}
				// non-object: last value wins (fall through to obj.set below)
			}
			obj.set(key, val)
		} else {
			r.setPath(obj, field.Key, val, field.Key)
		}
	}
	return obj, nil
}

func (r *resolver) resolveNode(node parser.Node, ctx *ObjectVal, pathPrefix []string) (Val, error) {
	switch n := node.(type) {
	case *parser.ScalarNode:
		st := ScalarString
		switch n.ValueType {
		case "number":
			st = ScalarNumber
		case "boolean":
			st = ScalarBoolean
		case "null":
			st = ScalarNull
		}
		return &ScalarVal{Raw: n.Raw, Type: st}, nil
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
		// leave substitution nodes for second pass — consume pre-parsed lexer segments directly
		return &substPlaceholder{node: n, segments: n.Segments.Segments, listSuffix: n.Segments.ListSuffix}, nil
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
	node       *parser.SubstNode
	segments   []lexer.Segment // path segments — source of truth
	prefixLen  int             // 0 for normal, >0 for relativized (number of prefix segments)
	listSuffix bool            // true when '[]' suffix present — triggers resolveEnvList (S13c)
	// knownAbsent is an internal sentinel for a folded optional self-ref with
	// no prior value. It resolves to undefined without performing a lookup.
	knownAbsent bool
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
	return &concatPlaceholder{vals: vals, line: n.Line(), col: n.Col()}, nil
}

type concatPlaceholder struct {
	vals []Val
	line int // 1-based source line of the concat value (from first AST node)
	col  int // 1-based source column of the concat value (from first AST node)
}

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
		return r.resolveConcat(vv.vals, root, path, vv.line, vv.col)
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
	if s.knownAbsent {
		return nil, nil
	}

	n := s.node
	segStrs := segTexts(s.segments)
	key := segmentsToKey(segStrs)

	// Fast path: if this key is already fully resolved (e.g. `b = ${a}` where
	// `a` was processed first), return the cached value immediately.  This
	// avoids re-entering the placeholder resolution loop and prevents the
	// "foofoo" double-expansion that would otherwise occur when the cached
	// value gets returned by the inner ${?a} re-entrant call.
	if !r.resolving[key] {
		if cached, ok := r.resolvedCache[key]; ok {
			return cached, nil
		}
	}

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

	val, ok := r.lookupPath(root, segStrs)
	if ok {
		// If the found value is still a placeholder that IS (or contains) this
		// same substitution node s, we have a genuine self-referential definition
		// (e.g. `a = ${?a}foo`).  We must NOT recursively resolve that value or
		// it would double-expand ("foofoo").  Instead, use the prior value or
		// short-circuit.
		//
		// Crucially, only fire when val contains s itself (pointer equality).
		// If val is a different concat that merely *mentions* the same path name
		// (e.g. `b = ${a}` where `a = ${?a}foo`), the lookup returns a/a's
		// definition which contains a *different* substPlaceholder node — the
		// isSelfRef branch must NOT fire, and the normal resolveVal path at the
		// bottom of this block will resolve a's value correctly (the inner
		// ${?a} hit r.resolving["a"]==true and returns nil, giving "foo").
		//
		// Spec deviation: the S13a.13 spec ★1 decision #1 specified path-equality
		// preservation for self-ref detection. Round-2 multi-agent-review surfaced
		// a false-positive on external lookups; the criterion was tightened to
		// AST-node pointer-identity (sp == s) — strictly narrower than path-
		// equality. Spec amendment deferred to a follow-up xx.hocon PR (see
		// Phase 6 #3f close-out notes).
		// #120 extension: walk ArrayVal / ObjectVal interiors so self-references
		// embedded as array elements or object field values also trigger the
		// prior-resolution branch. Original pointer-identity criterion preserved
		// throughout — only the search scope widens. containsSubstByIdentity
		// (foldselfref.go) does the recursive walk.
		isSelfRef := containsSubstByIdentity(val, s)
		if isSelfRef {
			if prior, ok2 := r.priorValues[key]; ok2 {
				return r.resolveVal(prior, root, key)
			}
			// For nested paths, check the parent object's per-object priorValues.
			if len(segStrs) > 1 {
				if parent, pok := r.lookupPathObj(root, segStrs[:len(segStrs)-1]); pok {
					lastKey := segStrs[len(segStrs)-1]
					if prior2, ok3 := parent.priorValues[lastKey]; ok3 {
						return r.resolveVal(prior2, root, key)
					}
				}
			}
			// Spec HOCON.md L841: no prior + self-ref → short-circuit.
			// Do NOT resolve the found current value (which is the concat-in-progress);
			// that would produce "foofoo" for `a = ${?a}foo` with no prior `a`.
			if n.Optional {
				return nil, nil
			}
			// Lenient mode: defer the placeholder so a later resolution pass
			// (with prior values supplied by a parent or fallback) can complete
			// it. Used by include resolution (#106): the included file's own
			// `${k}` against a parent-only `k` reaches this branch because the
			// child resolver has no prior; the parent's include-merge step
			// installs the prior and a subsequent ResolveTree pass succeeds.
			if r.lenient {
				return s, nil
			}
			return nil, &ResolveError{Message: "unresolved self-referential substitution", Path: key, Line: n.Line(), Col: n.Col()}
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
			if len(segStrs) > 0 {
				var parentObj *ObjectVal
				if len(segStrs) == 1 {
					parentObj = root
				} else {
					parentObj, _ = r.lookupPathObj(root, segStrs[:len(segStrs)-1])
				}
				if parentObj != nil {
					lastKey := segStrs[len(segStrs)-1]
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
		if len(segStrs) == 1 {
			if resolvedObj, rOk := resolved.(*ObjectVal); rOk {
				prior := r.findPrior(root, segStrs, key)
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

	// S13c: env-var list expansion — when '[]' suffix is present, delegate to
	// resolveEnvList. This branch runs BEFORE scalar env fallback (S13c.5: when
	// listSuffix=true, the bare scalar env var must NOT be consulted as fallback).
	//
	// E12: also gated on UseSystemEnvironment. When env access is disabled,
	// the list substitution behaves like any other unresolved substitution:
	// leave the placeholder if lenient; required-substitution error otherwise.
	if s.listSuffix {
		if !r.opts.UseSystemEnvironment {
			if n.Optional {
				return nil, nil // optional ${?X[]} → drop the field
			}
			if r.lenient {
				return s, nil // leave placeholder for re-resolution
			}
			return nil, &ResolveError{
				Message: fmt.Sprintf("required env-var list ${%s[]} cannot be resolved (UseSystemEnvironment=false)", strings.Join(segStrs, ".")),
				Path:    strings.Join(segStrs, "."),
				Line:    n.Line(),
				Col:     n.Col(),
			}
		}
		return r.resolveEnvList(s, segStrs, n)
	}

	// Relativized path not found — fall back to original (non-relativized) path.
	if s.prefixLen > 0 && len(segStrs) > s.prefixLen {
		originalStrs := segStrs[s.prefixLen:]
		originalKey := segmentsToKey(originalStrs)
		origVal, origOk := r.lookupPath(root, originalStrs)
		if origOk {
			resolved, err := r.resolveVal(origVal, root, originalKey)
			if err != nil {
				return nil, err
			}
			return resolved, nil
		}
		// Also try env var with original path (raw dot-join, no quoting)
		if r.opts.UseSystemEnvironment {
			if ev, ok := os.LookupEnv(strings.Join(originalStrs, ".")); ok {
				return &ScalarVal{Raw: ev, Type: ScalarString}, nil
			}
		}
	}

	// env var fallback — use raw dot-join (no quoting) to match Lightbend behavior
	if r.opts.UseSystemEnvironment {
		if ev, ok := os.LookupEnv(strings.Join(segStrs, ".")); ok {
			return &ScalarVal{Raw: ev, Type: ScalarString}, nil
		}
	}
	// In an `include` child resolver, do NOT drop optional substitutions yet
	// — the parent resolver's deep-merge pass may supply the value via the
	// parent's prior (#45). The placeholder is preserved; the parent's
	// strict / final pass then decides: resolved → use it; still missing →
	// drop per the optional rule. User-facing AllowUnresolved=true mode is
	// NOT scoped here: it keeps the documented contract that optional
	// substitutions drop (required-but-unsatisfied placeholders are still
	// preserved via the r.lenient branch below).
	if r.inIncludeChild {
		return s, nil
	}
	if n.Optional {
		return nil, nil // field will be dropped (user-facing default + AllowUnresolved=true mode)
	}
	if r.lenient {
		return s, nil
	}
	return nil, &ResolveError{Message: "unresolved substitution", Path: key, Line: n.Line(), Col: n.Col()}
}

// resolveEnvList implements S13c env-var list expansion for ${X[]} / ${?X[]}.
//
// Algorithm (spec HOCON.md L900-L917, extra-spec conventions E6/E7):
//  1. Build candidate bases: if prefixLen > 0, try fully-qualified segments
//     first, then bare (stripped of prefix). Otherwise just the full name.
//  2. For each candidate base, peek X_0. If present, scan X_0, X_1, … to
//     first absent index and return ArrayVal of ScalarString elements.
//     Empty-string element value is preserved (stop = key absent, not empty).
//  3. If no candidate base has _0: optional → nil (key dropped); required →
//     ResolveError. S13c.5: does NOT fall through to scalar env fallback.
func (r *resolver) resolveEnvList(s *substPlaceholder, segStrs []string, n *parser.SubstNode) (Val, error) {
	// Build candidate bases in order: relativized-full first (if applicable), then bare.
	var bases []string
	full := strings.Join(segStrs, ".")
	if s.prefixLen > 0 && len(segStrs) > s.prefixLen {
		bareStrs := segStrs[s.prefixLen:]
		bare := strings.Join(bareStrs, ".")
		bases = []string{full, bare}
	} else {
		bases = []string{full}
	}

	for _, base := range bases {
		key0 := base + "_0"
		if _, ok := os.LookupEnv(key0); ok {
			// First base with _0 wins entirely — scan to first missing index.
			var elems []Val
			for i := 0; ; i++ {
				k := fmt.Sprintf("%s_%d", base, i)
				v, present := os.LookupEnv(k)
				if !present {
					break
				}
				elems = append(elems, &ScalarVal{Raw: v, Type: ScalarString})
			}
			return &ArrayVal{Elements: elems}, nil
		}
	}

	// No candidate base had _0 — required/optional handling (S13c.3/S13c.4).
	// S13c.5: do NOT fall through to scalar env var fallback.
	if n.Optional {
		return nil, nil // field will be dropped
	}
	return nil, &ResolveError{
		Message: fmt.Sprintf("required env-var list ${%s[]} has no %s_0 element in the environment", strings.Join(segStrs, "."), strings.Join(segStrs, ".")),
		Path:    strings.Join(segStrs, "."),
		Line:    n.Line(),
		Col:     n.Col(),
	}
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
			if prior, ok2 := parentObj.priorValues[lastKey]; ok2 {
				return prior
			}
		}
	}
	return nil
}

func isSeparator(v Val) bool {
	if s, ok := v.(*ScalarVal); ok {
		if s.Type == ScalarString && s.Raw == " " {
			return true
		}
	}
	return false
}

func (r *resolver) resolveConcat(vals []Val, root *ObjectVal, path string, line, col int) (Val, error) {
	// resolve each val
	var resolved []Val
	for _, v := range vals {
		rv, err := r.resolveVal(v, root, path)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, rv)
	}

	// Lenient guard (E12 § "s10 × AllowUnresolved"): if any operand could not be
	// resolved (the lenient path returned the placeholder unchanged), defer the
	// entire concat as a concatPlaceholder. The deferred placeholder carries a
	// mix of fully-resolved values (scalars/objects/arrays) and residual sub-
	// placeholders; a subsequent ResolveTree call will re-pass each through
	// resolveVal — safe because resolveVal is idempotent for concrete values.
	// In lenient (AllowUnresolved) mode, if any resolved element is still a
	// placeholder, defer the whole concat rather than error or produce a broken
	// string. The placeholder will be resolved by a subsequent ResolveTree call
	// (e.g. after WithFallback merges the missing value).
	if r.lenient {
		for _, rv := range resolved {
			if _, isSP := rv.(*substPlaceholder); isSP {
				return &concatPlaceholder{vals: resolved, line: line, col: col}, nil
			}
			if _, isCP := rv.(*concatPlaceholder); isCP {
				return &concatPlaceholder{vals: resolved, line: line, col: col}, nil
			}
		}
	}

	// Classify non-nil, non-separator elements to detect array/object involvement
	// and whether any concrete (non-nil, non-separator) value is present.
	hasArrayOrObject := false
	hasConcreteValue := false
	for _, rv := range resolved {
		if rv == nil || isSeparator(rv) {
			continue
		}
		hasConcreteValue = true
		switch rv.(type) {
		case *ArrayVal, *ObjectVal:
			hasArrayOrObject = true
		}
	}

	// Per HOCON spec § "Optional substitution materialisation in concat contexts":
	// when the entire concat consists only of undefined optional substitutions
	// (all operands resolved to nil, no real scalar/array/object content),
	// the field is omitted — return nil so the parent drops this key.
	// This differs from a mixed concat like `${?x} "tail"` where the separator
	// and the literal "tail" count as concrete values.
	if !hasConcreteValue {
		return nil, nil
	}

	// Pure-scalar concat: pass the full resolved slice (including whitespace-separator
	// tokens) to concatStrings so that spaces between tokens are preserved.
	if !hasArrayOrObject {
		return r.concatStrings(resolved), nil
	}

	// Build the meaningful (non-nil, non-separator) slice for the pairwise fold.
	// When any array or object is present in the concat, ALL parser-inserted
	// separator tokens are stripped — including those between scalar tokens in
	// a mixed concat. This matches Lightbend's behaviour: in array/object
	// context, separator whitespace has no semantic role. Pure scalar concat
	// (handled above when !hasArrayOrObject) keeps separators so string concat
	// can preserve `"foo bar"` style whitespace.
	var meaningful []Val
	for _, rv := range resolved {
		if rv == nil || isSeparator(rv) {
			continue
		}
		meaningful = append(meaningful, rv)
	}

	if len(meaningful) == 0 {
		return r.concatStrings(resolved), nil
	}

	// True left-to-right pairwise fold per spec §"Multi-piece concat is
	// left-to-right pairwise (NORMATIVE)" and Lightbend ConfigConcatenation.consolidate.
	//
	// joinPair accumulates left-to-right so that adjacent objects are merged
	// (S10.3) before a list partner triggers numericObjectToArray.  A prior
	// single-pass classify-then-bulk approach produced wrong results when
	// adjacent objects had overlapping numeric keys (na03e regression).
	acc := meaningful[0]
	for _, v := range meaningful[1:] {
		next, err := r.joinPair(acc, v, line, col)
		if err != nil {
			return nil, err
		}
		acc = next
	}
	return acc, nil
}

// joinPair combines two adjacent resolved values per the HOCON concat rules.
// It is the inner step of the left-to-right pairwise fold in resolveConcat.
// All whitespace-separator tokens have already been stripped from the input.
//
// Spec §Value concatenation (L373/L385): type mismatches raise *ResolveError.
// Phase 6 #3b replaces all permissive-coercion fallbacks with errors.
func (r *resolver) joinPair(left, right Val, line, col int) (Val, error) {
	lArr, lIsArr := left.(*ArrayVal)
	rArr, rIsArr := right.(*ArrayVal)
	lObj, lIsObj := left.(*ObjectVal)
	rObj, rIsObj := right.(*ObjectVal)

	switch {
	case lIsObj && rIsObj:
		// S10.3: object + object → deep merge (right wins on key conflict).
		return mergeObjectConcat(lObj, rObj), nil

	case lIsArr && rIsObj:
		// Array + Object: S15 numeric conversion first; error if not convertible.
		if converted, ok := numericObjectToArray(rObj); ok {
			return concatTwoArrays(lArr, converted), nil
		}
		// S10.4/S10.19: non-numeric object cannot concat with array.
		return nil, &ResolveError{Message: fmt.Sprintf(
			"cannot concatenate %s with %s: value concatenation requires same-kind operands (spec L385)",
			valTypeName(left), valTypeName(right),
		), Line: line, Col: col}

	case lIsObj && rIsArr:
		// Object + Array: S15 numeric conversion first; error if not convertible.
		if converted, ok := numericObjectToArray(lObj); ok {
			return concatTwoArrays(converted, rArr), nil
		}
		// S10.4/S10.19: non-numeric object cannot concat with array.
		return nil, &ResolveError{Message: fmt.Sprintf(
			"cannot concatenate %s with %s: value concatenation requires same-kind operands (spec L385)",
			valTypeName(left), valTypeName(right),
		), Line: line, Col: col}

	case lIsArr && rIsArr:
		// Array + Array → plain concatenation.
		return concatTwoArrays(lArr, rArr), nil

	case lIsArr:
		// S10.13: array + scalar is not a string concat (spec L373).
		return nil, &ResolveError{Message: fmt.Sprintf(
			"cannot concatenate %s with %s: arrays and objects may not appear in string value concatenation (spec L373)",
			valTypeName(left), valTypeName(right),
		), Line: line, Col: col}

	case rIsArr:
		// S10.13: scalar + array is not a string concat (spec L373).
		return nil, &ResolveError{Message: fmt.Sprintf(
			"cannot concatenate %s with %s: arrays and objects may not appear in string value concatenation (spec L373)",
			valTypeName(left), valTypeName(right),
		), Line: line, Col: col}

	default:
		// Remaining cases: Scalar+Scalar (valid string-concat), or Object+Scalar /
		// Scalar+Object (both invalid per S10.13).
		_, lIsObjD := left.(*ObjectVal)
		_, rIsObjD := right.(*ObjectVal)
		if lIsObjD || rIsObjD {
			// S10.13: object may not appear in string value concatenation.
			return nil, &ResolveError{Message: fmt.Sprintf(
				"cannot concatenate %s with %s: arrays and objects may not appear in string value concatenation (spec L373)",
				valTypeName(left), valTypeName(right),
			), Line: line, Col: col}
		}
		// Pure scalar+scalar → string concat per S10 string-concat rules.
		return r.concatStrings([]Val{left, right}), nil
	}
}

// valTypeName returns a human-readable type name for a Val, used in error messages.
// For scalars the precise subtype (null/boolean/number/string) is returned so that
// error messages satisfy the spec requirement to name left/right types.
func valTypeName(v Val) string {
	switch vv := v.(type) {
	case *ArrayVal:
		return "array"
	case *ObjectVal:
		return "object"
	case *ScalarVal:
		switch vv.Type {
		case ScalarNull:
			return "null"
		case ScalarBoolean:
			return "boolean"
		case ScalarNumber:
			return "number"
		default:
			return "string"
		}
	default:
		return "scalar"
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

// concatTwoArrays concatenates two ArrayVals into a new ArrayVal.
func concatTwoArrays(left, right *ArrayVal) *ArrayVal {
	result := &ArrayVal{Elements: make([]Val, 0, len(left.Elements)+len(right.Elements))}
	result.Elements = append(result.Elements, left.Elements...)
	result.Elements = append(result.Elements, right.Elements...)
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
	return &ScalarVal{Raw: sb.String(), Type: ScalarString}
}

func valToString(v Val) string {
	switch vv := v.(type) {
	case *ScalarVal:
		return vv.Raw
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

func (r *resolver) setPath(obj *ObjectVal, segments []string, val Val, fullPath []string) {
	if len(segments) == 1 {
		key := segments[0]
		fullKey := segmentsToKey(fullPath)
		// Deep merge if both existing and new values are objects
		if existing, ok := obj.Get(key); ok {
			if eo, eok := existing.(*ObjectVal); eok {
				if nv, nok := val.(*ObjectVal); nok {
					val = deepMerge(nv, eo) // new over existing: nv=dst wins
				}
			}
			// #120: always save existing as prior (mirror of the top-level
			// resolveObject duplicate-key change). The previous !merged gate
			// silently dropped the multi-segment object-merge case
			// (`r.s = {v=1}; r.s = {history = ${r.s}, v=2}`) where the merged
			// val retained `${r.s}` but no prior was recorded.
			// See foldOrSkipPrior for the three-way decision (save / fold / skip).
			priorToSave, doSave := foldOrSkipPrior(existing, fullKey, r.priorValues[fullKey])
			if doSave {
				// Write r.priorValues keyed by the FULL dotted path. The previous
				// implementation deliberately avoided r.priorValues because it
				// used the bare leaf key (which would collide with an unrelated
				// top-level key of the same name). The full-path key has no
				// collision risk: "foo.a" never matches a top-level "a".
				// resolveSubst at L685 looks up r.priorValues[key] where key is
				// already the full dotted path of the placeholder, so the entry
				// is reachable from the self-ref branch with resolving=true.
				r.priorValues[fullKey] = priorToSave
				obj.priorValues[key] = priorToSave
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
	r.setPath(childObj, segments[1:], val, fullPath)
	obj.set(segments[0], childObj)
}

// includeExtensions is the list of extensions to probe when an include path
// has no extension, per the HOCON spec (properties first, then JSON, then HOCON).
var includeExtensions = []string{".properties", ".json", ".conf"}

func (r *resolver) resolveInclude(inc *parser.IncludeNode, pathPrefix []string) (*ObjectVal, error) {
	// E11: dispatch package includes to loadPackageInclude.
	if inc.IsPackage {
		obj, err := r.loadPackageInclude(inc.PkgID, inc.PkgFile)
		if err != nil {
			return nil, err
		}
		if len(pathPrefix) > 0 {
			relativizeVals(obj, pathPrefix)
		}
		return obj, nil
	}

	path := inc.Path
	if !filepath.IsAbs(path) {
		if inc.IsFile {
			// file() includes resolve relative to the process working directory,
			// NOT relative to the including file's directory (BaseDir).
			wd, err := os.Getwd()
			if err != nil {
				return nil, &ResolveError{Message: "cannot determine working directory: " + err.Error()}
			}
			path = filepath.Join(wd, path)
		} else {
			path = filepath.Join(r.opts.BaseDir, path)
		}
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
		newSegments := make([]lexer.Segment, 0, len(prefixSegments)+len(vv.segments))
		newSegments = append(newSegments, strSegs(prefixSegments)...)
		newSegments = append(newSegments, vv.segments...)
		newNode.Path = segmentsToKey(segTexts(newSegments))
		return &substPlaceholder{
			node:        newNode,
			segments:    newSegments,
			prefixLen:   vv.prefixLen + len(prefixSegments),
			listSuffix:  vv.listSuffix,
			knownAbsent: vv.knownAbsent,
		}
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

	// Lightbend-compat carve-out for #105: an empty or comment-only include
	// file contributes nothing instead of erroring with "empty file is not a
	// valid HOCON document". Top-level empty parses (ParseString("")) remain
	// invalid per spec S3.1 (HOCON.md L130); this carve-out applies ONLY to
	// the include path so the common optional-override-file pattern works.
	if isEmptyOrCommentOnlyHocon(data) {
		return newObjectVal(), nil
	}

	obj, err := r.parseAndResolve(data, path)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// isEmptyOrCommentOnlyHocon reports whether the given HOCON source has no
// semantic content — i.e. only HOCON whitespace (per the lexer's full
// definition — including NBSP, Unicode Zs, U+2028/U+2029, BOM, etc.) and
// the two HOCON line-comment forms (# and //). Block comments are NOT a
// HOCON syntax; if `/* ... */` appears, it falls through and the parser
// produces its proper error. Used by loadIncludeFile to short-circuit the
// S3.1 empty-document rejection for included files (Lightbend-compat for
// #105).
func isEmptyOrCommentOnlyHocon(data []byte) bool {
	s := string(data)
	for i := 0; i < len(s); {
		// Decode one rune so we treat all HOCON whitespace (NBSP, U+2028,
		// U+FEFF, etc. — multi-byte under UTF-8) consistently with the lexer.
		r, size := utf8.DecodeRuneInString(s[i:])
		// HOCON whitespace (per lexer.isHoconWhitespace) covers LF as well as
		// BOM at any position, not just the leading byte — so a single
		// IsHoconWhitespace check is sufficient.
		if lexer.IsHoconWhitespace(r) {
			i += size
			continue
		}
		if r == '#' {
			// # line comment — skip until newline or EOF.
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if r == '/' && i+1 < len(s) && s[i+1] == '/' {
			// // line comment — skip until newline or EOF.
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		return false
	}
	return true
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
		opts: Options{
			BaseDir:       filepath.Dir(filePath),
			PackageLookup: r.opts.PackageLookup, // E11: propagate so nested package includes resolve (Codex must-fix #1)
		},
		resolving:      make(map[string]bool),
		resolvedCache:  make(map[string]Val),
		priorValues:    make(map[string]Val),
		includeStack:   r.includeStack,
		lenient:        true, // don't error on unresolved substitutions; leave as placeholders
		inIncludeChild: true, // preserve optional substitutions for #45 (resolve against parent's prior on deep-merge)
	}
	obj, err := childResolver.resolveObject(ast, nil, nil)
	if err != nil {
		return nil, err
	}
	return childResolver.resolveSubstitutions(obj, obj)
}

// loadPackageInclude resolves a package(...) include directive (E11).
// It looks up (identifier, file) via opts.PackageLookup, performs cycle detection
// using a length-prefixed key in the shared includeStack, then parses and resolves
// the registered content.
//
// Per E11 decision 4: lookup miss is always a hard error, regardless of inc.Required.
func (r *resolver) loadPackageInclude(identifier, file string) (*ObjectVal, error) {
	// Build cycle-detection key: length-prefixed to avoid ambiguity (E11 decision 8 / design Decision 10).
	// Format: "package:<len(identifier)>:<identifier>:<file>"
	cycleKey := fmt.Sprintf("package:%d:%s:%s", len(identifier), identifier, file)
	for _, p := range *r.includeStack {
		if p == cycleKey {
			return nil, &ResolveError{
				Message: fmt.Sprintf("circular include: package(%q, %q)", identifier, file),
			}
		}
	}
	*r.includeStack = append(*r.includeStack, cycleKey)
	defer func() {
		*r.includeStack = (*r.includeStack)[:len(*r.includeStack)-1]
	}()

	// Lookup content from registry (via injected callback).
	var content []byte
	if r.opts.PackageLookup != nil {
		var lookupErr error
		content, lookupErr = r.opts.PackageLookup(identifier, file)
		if lookupErr != nil {
			// Preserve the original error cause so callers can distinguish a registry miss
			// from other lookup failures (I/O error, permission denied, etc.) via errors.Is/As.
			return nil, &ResolveError{
				Message: fmt.Sprintf("package(%q, %q): %s; "+
					"if this is a missing registration, ensure the providing package is imported with _ %q in your application",
					identifier, file, lookupErr.Error(), identifier),
				Cause: lookupErr,
			}
		}
	} else {
		return nil, &ResolveError{
			Message: fmt.Sprintf("package(%q, %q) not found in registry; "+
				"ensure the providing package is imported with _ %q in your application",
				identifier, file, identifier),
		}
	}

	// Empty content is not a failure — contributes empty object per E11 decision 4 note.
	if len(content) == 0 {
		return newObjectVal(), nil
	}

	// Build a synthetic virtual path used as a descriptive label in error messages.
	// parseAndResolvePackage inherits r.opts.BaseDir for any nested file includes
	// within the package content (unusual but valid); BaseDir is not derived from
	// virtualPath — it comes from the resolver options already set on r.
	virtualPath := fmt.Sprintf("package:%s:%s", identifier, file)
	obj, err := r.parseAndResolvePackage(content, virtualPath)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// parseAndResolvePackage is like parseAndResolve but for package content (no BaseDir from filePath).
// virtualPath is a descriptive label of the form "package:<identifier>:<file>" used in error messages.
// Uses opts.BaseDir as the base for any nested file includes.
func (r *resolver) parseAndResolvePackage(data []byte, virtualPath string) (*ObjectVal, error) {
	ast, err := parser.ParseBytes(data)
	if err != nil {
		// Wrap raw parser error with package context so the user knows which package failed.
		return nil, &ResolveError{
			Message:  fmt.Sprintf("in %s: %s", virtualPath, err.Error()),
			FilePath: virtualPath,
		}
	}
	childResolver := &resolver{
		opts: Options{
			BaseDir:       r.opts.BaseDir, // inherit parent BaseDir for nested file includes
			PackageLookup: r.opts.PackageLookup,
		},
		resolving:      make(map[string]bool),
		resolvedCache:  make(map[string]Val),
		priorValues:    make(map[string]Val),
		includeStack:   r.includeStack,
		lenient:        true,
		inIncludeChild: true, // preserve optional substitutions for #45 (resolve against parent's prior on deep-merge)
	}
	obj, err := childResolver.resolveObject(ast, nil, nil)
	if err != nil {
		// Wrap resolve error with package context.
		return nil, &ResolveError{
			Message:  fmt.Sprintf("in %s: %s", virtualPath, err.Error()),
			FilePath: virtualPath,
		}
	}
	result, err := childResolver.resolveSubstitutions(obj, obj)
	if err != nil {
		return nil, &ResolveError{
			Message:  fmt.Sprintf("in %s: %s", virtualPath, err.Error()),
			FilePath: virtualPath,
		}
	}
	return result, nil
}

// propsToObjectVal converts a flat map[string]string (from a .properties file)
// into a nested *ObjectVal applying the HOCON.md L1485 "object wins" rule.
//
// Keys are processed in sorted order (sort.Strings) so conflict resolution is
// deterministic regardless of the input line order in the .properties file
// (per HOCON.md L1476–1479: Java properties do not preserve file order). The
// sort guarantees parent paths (e.g. "a") are processed before child paths
// (e.g. "a.b"), so the only conflict shape that surfaces at iteration time is
// non-leaf-meets-existing-scalar — handled by replacing the scalar with a new
// object and descending (the string is discarded per L1487 "object wins throws
// out at most one value, the string"). The reverse case (leaf scalar meets
// existing object) cannot occur under this ordering.
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
				// Leaf: sort.Strings above guarantees parent paths process
				// before child paths, so an existing object at this slot
				// cannot occur — object-wins (HOCON.md L1485) is enforced
				// by the non-leaf scalar-replace branch below.
				cur.set(part, &ScalarVal{Raw: value, Type: ScalarString})
				break
			}
			existing, has := cur.values[part]
			if !has {
				child := newObjectVal()
				cur.set(part, child)
				cur = child
				continue
			}
			if child, isObj := existing.(*ObjectVal); isObj {
				cur = child
				continue
			}
			// Existing value is a scalar but we need to descend: replace the
			// scalar with a new object (object wins, scalar discarded per
			// HOCON.md L1487).
			child := newObjectVal()
			cur.set(part, child)
			cur = child
		}
	}
	return root
}
