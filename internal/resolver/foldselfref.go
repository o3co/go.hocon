// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

// foldSelfRef walks v and rewrites every substPlaceholder whose dotted-path
// key equals fullKey by substituting replacement. The boolean return reports
// whether at least one such self-reference was found in v (regardless of
// whether substitution actually occurred).
//
// Pass replacement=nil to perform a presence-only check without rewriting:
// the returned Val is v unchanged and the boolean still reports correctly.
// This single-walk API combines what would otherwise be two passes (one to
// detect, one to rewrite) into one.
//
// Scope: walks substPlaceholder / concatPlaceholder AND ArrayVal elements /
// ObjectVal field values recursively. Together with foldOrSkipPrior at the
// originating prior-save sites this fixes the full chain-class for #118
// (concat/subst patterns) and #120 (array-element / object-field interior
// self-references).
//
// knownAbsent exclusion: substPlaceholder nodes with knownAbsent=true are
// internal sentinels produced by foldOptionalSelfRefAbsent (xx.hocon#27 sr15).
// They are NOT rewritten and NOT counted as self-ref hits — the fast-path
// at the substPlaceholder case returns (v, false) immediately so callers do
// not double-fold or misclassify a sentinel-bearing prior as needing rewrite.
// Rehydration of sentinels happens separately in rehydrateSentinel during
// MergeUnresolved when a fallback prior is available.
func foldSelfRef(v Val, fullKey string, replacement Val) (Val, bool) {
	switch vv := v.(type) {
	case *substPlaceholder:
		if vv.knownAbsent {
			return v, false
		}
		if substFullKey(vv) != fullKey {
			return v, false
		}
		if replacement != nil {
			return replacement, true
		}
		return v, true
	case *concatPlaceholder:
		var newVals []Val
		anyHit := false
		for i, e := range vv.vals {
			ne, hit := foldSelfRef(e, fullKey, replacement)
			if hit {
				if !anyHit && replacement != nil {
					// First hit — lazy-init the rewritten slice so the
					// no-self-ref common path allocates nothing.
					newVals = make([]Val, len(vv.vals))
					copy(newVals[:i], vv.vals[:i])
				}
				anyHit = true
			}
			if newVals != nil {
				newVals[i] = ne
			}
		}
		if !anyHit {
			return v, false
		}
		if replacement == nil {
			return v, true
		}
		return &concatPlaceholder{vals: newVals, line: vv.line, col: vv.col}, true
	case *ArrayVal:
		// #120: array element self-ref (e.g. `a = [${a}, "x"]` chained).
		var newEls []Val
		anyHit := false
		for i, e := range vv.Elements {
			ne, hit := foldSelfRef(e, fullKey, replacement)
			if hit {
				if !anyHit && replacement != nil {
					newEls = make([]Val, len(vv.Elements))
					copy(newEls[:i], vv.Elements[:i])
				}
				anyHit = true
			}
			if newEls != nil {
				newEls[i] = ne
			}
		}
		if !anyHit {
			return v, false
		}
		if replacement == nil {
			return v, true
		}
		return &ArrayVal{Elements: newEls}, true
	case *ObjectVal:
		// #120: object field-value self-ref (e.g. `o = { history = ${o}, ... }`).
		// Walk each field's value. Fold yields a new ObjectVal with rewritten
		// values; keys and priorValues are preserved from the original.
		var newObj *ObjectVal
		anyHit := false
		for _, k := range vv.keys {
			val := vv.values[k]
			nv, hit := foldSelfRef(val, fullKey, replacement)
			if hit {
				if !anyHit && replacement != nil {
					newObj = newObjectVal()
					// Copy keys + values up to (but not including) the current
					// field. Subsequent fields written below either get the
					// rewritten value (current) or the original (later loop iters).
					for _, kk := range vv.keys {
						if kk == k {
							break
						}
						newObj.set(kk, vv.values[kk])
					}
					// Carry over priorValues so per-object look-back continues
					// to find them post-fold.
					for pk, pv := range vv.priorValues {
						newObj.priorValues[pk] = pv
					}
				}
				anyHit = true
			}
			if newObj != nil {
				if hit {
					newObj.set(k, nv)
				} else {
					newObj.set(k, val)
				}
			}
		}
		if !anyHit {
			return v, false
		}
		if replacement == nil {
			return v, true
		}
		return newObj, true
	default:
		return v, false
	}
}

// foldOrSkipPrior decides how to save a self-reference-aware prior at one
// of the three prior-save sites (direct assignment, include-merge non-object
// override, setPath nested assignment). Cases:
//
//   - prior has no self-ref to fullKey         → save prior as-is  → (prior,  true)
//   - prior has self-ref AND old != nil        → fold against old  → (folded, true)
//   - optional self-ref AND old == nil         → fold to absent    → (folded, true)
//   - required self-ref AND old == nil         → skip save         → (nil,    false)
//
// The no-prior optional case preserves S13a.13's "optional self-ref with no
// prior resolves to undefined" rule while still saving concat literal pieces
// for the next overwrite.
func foldOrSkipPrior(prior Val, fullKey string, old Val) (Val, bool) {
	folded, hasSelfRef := foldSelfRef(prior, fullKey, old)
	if !hasSelfRef {
		return prior, true
	}
	if old == nil {
		return foldOptionalSelfRefAbsent(prior, fullKey)
	}
	return folded, true
}

func foldOptionalSelfRefAbsent(v Val, fullKey string) (Val, bool) {
	switch vv := v.(type) {
	case *substPlaceholder:
		if vv.knownAbsent || substFullKey(vv) != fullKey {
			return v, true
		}
		if !vv.node.Optional {
			return nil, false
		}
		absent := *vv
		absent.knownAbsent = true
		return &absent, true
	case *concatPlaceholder:
		newVals := make([]Val, len(vv.vals))
		for i, e := range vv.vals {
			folded, ok := foldOptionalSelfRefAbsent(e, fullKey)
			if !ok {
				return nil, false
			}
			newVals[i] = folded
		}
		return &concatPlaceholder{vals: newVals, line: vv.line, col: vv.col}, true
	case *ArrayVal:
		newEls := make([]Val, len(vv.Elements))
		for i, e := range vv.Elements {
			folded, ok := foldOptionalSelfRefAbsent(e, fullKey)
			if !ok {
				return nil, false
			}
			newEls[i] = folded
		}
		return &ArrayVal{Elements: newEls}, true
	case *ObjectVal:
		// Walk live keys via vv.keys, rebuild each field value.  priorValues are
		// copied verbatim afterwards — the same pattern used by foldSelfRef
		// (see its ObjectVal branch) and deepMerge.
		//
		// ObjectVal invariant: priorValues may contain keys that are NOT in the
		// live keys slice (e.g. priors carried over from a fallback layer via
		// MergeUnresolved where the live key was overridden).  Copying priorValues
		// directly without iterating them through vv.keys is intentional and
		// correct — the rebuilt object must preserve the full prior chain.
		newObj := newObjectVal()
		for _, k := range vv.keys {
			folded, ok := foldOptionalSelfRefAbsent(vv.values[k], fullKey)
			if !ok {
				return nil, false
			}
			newObj.set(k, folded)
		}
		for pk, pv := range vv.priorValues {
			newObj.priorValues[pk] = pv
		}
		return newObj, true
	default:
		return v, true
	}
}

// containsKnownAbsentSentinel reports whether v was produced by
// foldOptionalSelfRefAbsent — i.e. it contains at least one knownAbsent
// substPlaceholder. Such a value means "this prior was folded under the
// assumption that no local prior existed at BuildTree time; ${?key} inside
// it should resolve to absent."
//
// When MergeUnresolved applies receiver priorValues over fallback priors, a
// prior that contains a knownAbsent sentinel must not overwrite a real
// (non-sentinel) fallback prior: the fallback's value should serve as the
// prior for the cross-layer self-reference fold instead.
//
// Recurses into substPlaceholder, concatPlaceholder, ArrayVal elements, and
// ObjectVal field values. ScalarVal is never a sentinel and returns false.
//
// Round-2 review (PR #126, Codex P2 + Claude M1): the original version only
// checked substPlaceholder and concatPlaceholder, missing the cases where
// foldOptionalSelfRefAbsent places sentinels inside ArrayVal.Elements or
// ObjectVal.values (e.g. `a = [${?a}, "x"]` with no prior).
func containsKnownAbsentSentinel(v Val) bool {
	switch vv := v.(type) {
	case *substPlaceholder:
		return vv.knownAbsent
	case *concatPlaceholder:
		for _, e := range vv.vals {
			if containsKnownAbsentSentinel(e) {
				return true
			}
		}
		return false
	case *ArrayVal:
		for _, e := range vv.Elements {
			if containsKnownAbsentSentinel(e) {
				return true
			}
		}
		return false
	case *ObjectVal:
		for _, k := range vv.keys {
			if containsKnownAbsentSentinel(vv.values[k]) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// rehydrateSentinel replaces knownAbsent substPlaceholder nodes within v
// with replacement. It is used by MergeUnresolved to fold a receiver prior
// that was built under a "no local prior" assumption against a real fallback
// prior, so that deferred resolution can correctly chain the fallback value
// through all assignments.
//
// For example: receiver prior = concat(knownAbsent, "1"), fallback = "base"
// → rehydrate → concat("base", "1") which resolves to "base1". That "base1"
// value then serves as the effective prior for the outer a = ${?a}2 concat,
// yielding "base12" as the final resolved value.
//
// ArrayVal semantics (round-3 correction, PR #126): the replacement is always
// inserted as a SINGLE element where the sentinel was — NO splicing, even if
// the replacement is itself an ArrayVal. This matches HOCON spec: `${?a}` in
// array literal `[${?a}, "x"]` resolves to the prior value as a single
// element, NOT flattened. For the deferred-resolution + fallback case,
// receiver prior `[knownAbsent, "x"]` with fallback `["base"]` rehydrates to
// `[["base"], "x"]` (single-element insertion), matching the immediate-resolve
// equivalent of `a=["base"]; a=[${?a}, "x"]; a=[${?a}, "y"]` →
// `[[["base"], "x"], "y"]`. (Earlier round-2 commit used splice flatten,
// producing the inconsistent `[["base","x"], "y"]` — corrected by single-
// element semantics here.)
//
// ObjectVal semantics: recurse into each field value and rehydrate in place.
// The replacement is passed unchanged to each field's sentinel — the sentinel
// inside an ObjectVal field represents "${?key}" referencing the whole prior,
// so the whole fallback prior is the correct replacement for each field.
//
// Only knownAbsent substPlaceholder nodes are replaced; live (non-absent)
// substPlaceholder nodes and all other Val types are passed through unchanged.
func rehydrateSentinel(v Val, replacement Val) Val {
	switch vv := v.(type) {
	case *substPlaceholder:
		if !vv.knownAbsent {
			return v
		}
		return replacement
	case *concatPlaceholder:
		newVals := make([]Val, len(vv.vals))
		changed := false
		for i, e := range vv.vals {
			ne := rehydrateSentinel(e, replacement)
			newVals[i] = ne
			if ne != e {
				changed = true
			}
		}
		if !changed {
			return v
		}
		return &concatPlaceholder{vals: newVals, line: vv.line, col: vv.col}
	case *ArrayVal:
		// Rehydrate sentinel elements within the array.
		// Always place the replacement as a SINGLE element where the sentinel
		// was, NOT spliced. This matches HOCON spec semantics: `${?a}` inside
		// array literal `[${?a}, "x"]` resolves to the prior value AS a single
		// element of the new array (no flattening), so the equivalent immediate-
		// resolve case `a=["base"]; a=[${?a},"x"]; a=[${?a},"y"]` produces
		// `[[["base"],"x"],"y"]` (triple nesting). The deferred+fallback case
		// must produce the same triple-nested result for spec equivalence.
		//
		// xx.hocon#27 round-3 review #126: previous splice-when-array-replacement
		// behavior produced `[["base","x"],"y"]` (one-level flatten), inconsistent
		// with go.hocon's own immediate-resolve output. Verified via probe.
		var newEls []Val
		changed := false
		for _, e := range vv.Elements {
			if sp, isSP := e.(*substPlaceholder); isSP && sp.knownAbsent {
				if !changed {
					newEls = make([]Val, 0, len(vv.Elements))
					// Copy all elements seen so far (before the first sentinel hit)
					// using the original slice up to this point.
					for _, prev := range vv.Elements {
						if prev == e {
							break
						}
						newEls = append(newEls, prev)
					}
				}
				changed = true
				newEls = append(newEls, replacement)
			} else {
				ne := rehydrateSentinel(e, replacement)
				if ne != e {
					if !changed {
						newEls = make([]Val, 0, len(vv.Elements))
						for _, prev := range vv.Elements {
							if prev == e {
								break
							}
							newEls = append(newEls, prev)
						}
					}
					changed = true
					newEls = append(newEls, ne)
				} else if changed {
					newEls = append(newEls, e)
				}
			}
		}
		if !changed {
			return v
		}
		return &ArrayVal{Elements: newEls}
	case *ObjectVal:
		// Rehydrate sentinel values in each field. Preserve key order and
		// priorValues (same pattern as foldSelfRef's ObjectVal branch).
		var newObj *ObjectVal
		for _, k := range vv.keys {
			val := vv.values[k]
			nv := rehydrateSentinel(val, replacement)
			if nv != val {
				if newObj == nil {
					newObj = newObjectVal()
					for _, kk := range vv.keys {
						if kk == k {
							break
						}
						newObj.set(kk, vv.values[kk])
					}
					for pk, pv := range vv.priorValues {
						newObj.priorValues[pk] = pv
					}
				}
			}
			if newObj != nil {
				newObj.set(k, nv)
			}
		}
		if newObj == nil {
			return v
		}
		return newObj
	default:
		return v
	}
}

// selfRefFullKey reports the dotted path of the self-reference inside a
// desugared `+=` value — the `${?path.to.key} [...]` concat shape — whose FINAL
// path segment equals key. deepMerge uses it to recognise a deferred `+=` chain
// value carried unresolved from an include (#135) and stitch the earlier merge
// counterpart in as the self-ref prior instead of dropping it. The final-segment
// match (rather than full-path equality) is robust to include relativization: a
// `${?items}` mounted under `srv` becomes `${?srv.items}` while the merge key is
// still the leaf `items`. Returns ("", false) when v has no such self-reference.
//
// Scope is the `${?k} [..]` concat the `+=` desugar (FieldNode.AppendToConcat)
// produces — the only shape that reaches deepMerge's self-ref-stitch branch,
// which runs after containsKnownAbsentSentinel(dv) is false and after the
// both-objects case, so no knownAbsent node or bare array/object self-ref
// arrives here.
func selfRefFullKey(v Val, key string) (string, bool) {
	switch vv := v.(type) {
	case *substPlaceholder:
		segs := segTexts(vv.segments)
		if len(segs) > 0 && segs[len(segs)-1] == key {
			return segmentsToKey(segs), true
		}
	case *concatPlaceholder:
		for _, e := range vv.vals {
			if fk, ok := selfRefFullKey(e, key); ok {
				return fk, true
			}
		}
	}
	return "", false
}

// substFullKey returns the dotted-path key of a substPlaceholder's segments.
// Segments are already relativized at this point if the placeholder lives
// inside an included file under a nested pathPrefix.
func substFullKey(sp *substPlaceholder) string {
	return segmentsToKey(segTexts(sp.segments))
}

// containsSubstByIdentity reports whether v contains the given substPlaceholder
// node (pointer identity, not path equality). Used by resolveSubst's self-ref
// detection where a lookup returns a value containing the same placeholder node
// being currently resolved — distinct from foldSelfRef's path-equality walk used
// at prior-save time.
//
// Recurses through concatPlaceholder, ArrayVal, and ObjectVal so all the chain
// shapes #120 covers are handled: `${a}` as a concat operand, an ArrayVal
// element, or an ObjectVal field value.
func containsSubstByIdentity(v Val, target *substPlaceholder) bool {
	switch vv := v.(type) {
	case *substPlaceholder:
		if vv.knownAbsent {
			return false
		}
		return vv == target
	case *concatPlaceholder:
		for _, e := range vv.vals {
			if containsSubstByIdentity(e, target) {
				return true
			}
		}
		return false
	case *ArrayVal:
		for _, e := range vv.Elements {
			if containsSubstByIdentity(e, target) {
				return true
			}
		}
		return false
	case *ObjectVal:
		for _, k := range vv.keys {
			if containsSubstByIdentity(vv.values[k], target) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
