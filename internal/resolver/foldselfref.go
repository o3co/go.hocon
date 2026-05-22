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
// Scope: walks substPlaceholder / concatPlaceholder only. Self-references
// embedded inside ArrayVal elements or ObjectVal fields are out of scope —
// those represent a separate pattern (e.g. `a = [${a}, 1]`) tracked in #120.
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
