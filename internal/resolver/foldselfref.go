// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

// containsSelfRef reports whether v contains at least one substPlaceholder
// whose dotted-path key equals fullKey.
//
// Scope: walks concatPlaceholder and substPlaceholder only. Self-references
// embedded inside ArrayVal elements or ObjectVal fields are out of scope —
// those represent a separate pattern (e.g. `a = [${a}, 1]`) that #118's fix
// does not address.
func containsSelfRef(v Val, fullKey string) bool {
	switch vv := v.(type) {
	case *substPlaceholder:
		return substFullKey(vv) == fullKey
	case *concatPlaceholder:
		for _, e := range vv.vals {
			if containsSelfRef(e, fullKey) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// foldSelfRef returns v with every substPlaceholder whose dotted-path key
// equals fullKey replaced by replacement. If v contains no such self-ref,
// v is returned unchanged.
//
// Used at prior-save time to break self-referential chains: when an
// assignment `key = ${key} [...]` is about to overwrite an earlier value,
// the new prior would itself contain `${key}` and produce infinite recursion
// during resolution. Substituting the actual prior value for `${key}` in
// the concat yields a self-ref-free tree that resolveSubst can walk safely.
//
// Scope matches containsSelfRef: walks only concat / subst placeholders.
func foldSelfRef(v Val, fullKey string, replacement Val) Val {
	switch vv := v.(type) {
	case *substPlaceholder:
		if substFullKey(vv) == fullKey {
			return replacement
		}
		return v
	case *concatPlaceholder:
		anyChanged := false
		newVals := make([]Val, len(vv.vals))
		for i, e := range vv.vals {
			ne := foldSelfRef(e, fullKey, replacement)
			newVals[i] = ne
			if ne != e {
				anyChanged = true
			}
		}
		if !anyChanged {
			return v
		}
		return &concatPlaceholder{vals: newVals, line: vv.line, col: vv.col}
	default:
		return v
	}
}

// substFullKey returns the dotted-path key of a substPlaceholder's segments.
// Segments are already relativized at this point if the placeholder lives
// inside an included file under a nested pathPrefix.
func substFullKey(sp *substPlaceholder) string {
	return segmentsToKey(segTexts(sp.segments))
}
