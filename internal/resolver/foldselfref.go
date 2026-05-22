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
func foldSelfRef(v Val, fullKey string, replacement Val) (Val, bool) {
	switch vv := v.(type) {
	case *substPlaceholder:
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
//   - prior has self-ref AND old == nil        → skip save         → (nil,    false)
//
// The skip case (no old prior to fold against) preserves the existing
// "unresolved self-referential substitution" error path at resolveSubst.
// Callers must not write to priorValues when the second return value is
// false; leaving priorValues untouched is what makes the error path fire.
func foldOrSkipPrior(prior Val, fullKey string, old Val) (Val, bool) {
	folded, hasSelfRef := foldSelfRef(prior, fullKey, old)
	if !hasSelfRef {
		return prior, true
	}
	if old == nil {
		return nil, false
	}
	return folded, true
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
