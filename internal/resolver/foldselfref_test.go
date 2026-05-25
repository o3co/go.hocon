// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Unit tests for foldOptionalSelfRefAbsent, containsKnownAbsentSentinel, and
// rehydrateSentinel — branch-by-branch isolated assertions.
//
// These helpers are unexported; the file uses package resolver (internal) so
// they can be called directly without going through the full parse/resolve
// pipeline.  Each test group targets one function and one container branch so
// failures are immediately locatable.
//
// Pattern mirrors ts.hocon commit 9456ef4 (tests/fold-self-ref-unit.test.ts).
package resolver

import (
	"testing"

	"github.com/o3co/go.hocon/internal/lexer"
	"github.com/o3co/go.hocon/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers: build minimal Val nodes without invoking the parser.
// ---------------------------------------------------------------------------

// fsr_optSubst builds an optional substPlaceholder for key (single segment).
func fsr_optSubst(key string) *substPlaceholder {
	return &substPlaceholder{
		node:     &parser.SubstNode{Optional: true},
		segments: []lexer.Segment{{Text: key}},
	}
}

// fsr_reqSubst builds a required substPlaceholder for key (single segment).
func fsr_reqSubst(key string) *substPlaceholder {
	return &substPlaceholder{
		node:     &parser.SubstNode{Optional: false},
		segments: []lexer.Segment{{Text: key}},
	}
}

// fsr_absentSubst builds a knownAbsent substPlaceholder (sentinel).
func fsr_absentSubst(key string) *substPlaceholder {
	sp := fsr_optSubst(key)
	sp.knownAbsent = true
	return sp
}

// fsr_scalar returns a simple ScalarVal for use as a non-placeholder Val.
func fsr_scalar(raw string) *ScalarVal {
	return &ScalarVal{Raw: raw, Type: ScalarString}
}

// fsr_concat builds a concatPlaceholder from the given vals.
func fsr_concat(vals ...Val) *concatPlaceholder {
	return &concatPlaceholder{vals: vals}
}

// fsr_array builds an ArrayVal from the given elements.
func fsr_array(els ...Val) *ArrayVal {
	return &ArrayVal{Elements: els}
}

// fsr_obj1 builds a single-key ObjectVal.
func fsr_obj1(key string, v Val) *ObjectVal {
	o := newObjectVal()
	o.set(key, v)
	return o
}

// ---------------------------------------------------------------------------
// foldOptionalSelfRefAbsent — substPlaceholder branch
// ---------------------------------------------------------------------------

// TestFoldOptionalSelfRefAbsent_SubstOptional verifies that an optional
// self-ref placeholder for the target key is marked knownAbsent and returned.
func TestFoldOptionalSelfRefAbsent_SubstOptional(t *testing.T) {
	sp := fsr_optSubst("a")
	got, ok := foldOptionalSelfRefAbsent(sp, "a")
	if !ok {
		t.Fatal("expected ok=true for optional self-ref")
	}
	result, isSP := got.(*substPlaceholder)
	if !isSP {
		t.Fatalf("expected *substPlaceholder, got %T", got)
	}
	if !result.knownAbsent {
		t.Error("expected knownAbsent=true on folded optional self-ref")
	}
}

// TestFoldOptionalSelfRefAbsent_SubstRequired verifies that a required
// self-ref returns nil,false (skip save).
func TestFoldOptionalSelfRefAbsent_SubstRequired(t *testing.T) {
	sp := fsr_reqSubst("a")
	got, ok := foldOptionalSelfRefAbsent(sp, "a")
	if ok {
		t.Fatalf("expected ok=false for required self-ref, got val=%v", got)
	}
	if got != nil {
		t.Errorf("expected nil val for required self-ref, got %T", got)
	}
}

// TestFoldOptionalSelfRefAbsent_SubstAlreadyAbsent verifies that a
// knownAbsent sentinel passes through unchanged (already folded).
func TestFoldOptionalSelfRefAbsent_SubstAlreadyAbsent(t *testing.T) {
	sp := fsr_absentSubst("a")
	got, ok := foldOptionalSelfRefAbsent(sp, "a")
	if !ok {
		t.Fatal("expected ok=true for already-absent sentinel")
	}
	if got != sp {
		t.Error("expected already-absent sentinel to be returned as-is")
	}
}

// TestFoldOptionalSelfRefAbsent_SubstDifferentKey verifies that a placeholder
// for a different key passes through unchanged.
func TestFoldOptionalSelfRefAbsent_SubstDifferentKey(t *testing.T) {
	sp := fsr_optSubst("b")
	got, ok := foldOptionalSelfRefAbsent(sp, "a")
	if !ok {
		t.Fatal("expected ok=true for non-self-ref key")
	}
	result, isSP := got.(*substPlaceholder)
	if !isSP {
		t.Fatalf("expected *substPlaceholder, got %T", got)
	}
	if result.knownAbsent {
		t.Error("knownAbsent must remain false for non-self-ref key")
	}
}

// ---------------------------------------------------------------------------
// foldOptionalSelfRefAbsent — concatPlaceholder branch
// ---------------------------------------------------------------------------

// TestFoldOptionalSelfRefAbsent_ConcatOptional verifies that a concat whose
// element is an optional self-ref has that element folded to knownAbsent.
func TestFoldOptionalSelfRefAbsent_ConcatOptional(t *testing.T) {
	cp := fsr_concat(fsr_optSubst("a"), fsr_scalar("1"))
	got, ok := foldOptionalSelfRefAbsent(cp, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result, isCP := got.(*concatPlaceholder)
	if !isCP {
		t.Fatalf("expected *concatPlaceholder, got %T", got)
	}
	sp, isSP := result.vals[0].(*substPlaceholder)
	if !isSP {
		t.Fatalf("vals[0]: expected *substPlaceholder, got %T", result.vals[0])
	}
	if !sp.knownAbsent {
		t.Error("concat: first element should be knownAbsent after fold")
	}
}

// TestFoldOptionalSelfRefAbsent_ConcatRequired verifies that a concat with a
// required self-ref returns nil,false.
func TestFoldOptionalSelfRefAbsent_ConcatRequired(t *testing.T) {
	cp := fsr_concat(fsr_reqSubst("a"), fsr_scalar("1"))
	got, ok := foldOptionalSelfRefAbsent(cp, "a")
	if ok {
		t.Fatalf("expected ok=false for required self-ref in concat, got %v", got)
	}
}

// TestFoldOptionalSelfRefAbsent_ConcatNoSelfRef verifies that a concat with
// no self-ref passes through with a new slice (but unchanged values).
func TestFoldOptionalSelfRefAbsent_ConcatNoSelfRef(t *testing.T) {
	sp := fsr_optSubst("b")
	cp := fsr_concat(sp, fsr_scalar("1"))
	got, ok := foldOptionalSelfRefAbsent(cp, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result, isCP := got.(*concatPlaceholder)
	if !isCP {
		t.Fatalf("expected *concatPlaceholder, got %T", got)
	}
	elem, isSP := result.vals[0].(*substPlaceholder)
	if !isSP {
		t.Fatalf("vals[0]: expected *substPlaceholder, got %T", result.vals[0])
	}
	if elem.knownAbsent {
		t.Error("non-self-ref subst in concat must not be marked knownAbsent")
	}
}

// ---------------------------------------------------------------------------
// foldOptionalSelfRefAbsent — ArrayVal branch
// ---------------------------------------------------------------------------

// TestFoldOptionalSelfRefAbsent_ArrayOptionalElem verifies that an array
// containing an optional self-ref element folds that element to knownAbsent.
func TestFoldOptionalSelfRefAbsent_ArrayOptionalElem(t *testing.T) {
	arr := fsr_array(fsr_optSubst("a"), fsr_scalar("x"))
	got, ok := foldOptionalSelfRefAbsent(arr, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result, isArr := got.(*ArrayVal)
	if !isArr {
		t.Fatalf("expected *ArrayVal, got %T", got)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}
	sp, isSP := result.Elements[0].(*substPlaceholder)
	if !isSP {
		t.Fatalf("Elements[0]: expected *substPlaceholder, got %T", result.Elements[0])
	}
	if !sp.knownAbsent {
		t.Error("ArrayVal element should be knownAbsent after fold")
	}
}

// TestFoldOptionalSelfRefAbsent_ArrayRequiredElem verifies that an array with
// a required self-ref element returns nil,false.
func TestFoldOptionalSelfRefAbsent_ArrayRequiredElem(t *testing.T) {
	arr := fsr_array(fsr_reqSubst("a"))
	got, ok := foldOptionalSelfRefAbsent(arr, "a")
	if ok {
		t.Fatalf("expected ok=false for required self-ref in array, got %v", got)
	}
	if got != nil {
		t.Errorf("expected nil, got %T", got)
	}
}

// TestFoldOptionalSelfRefAbsent_ArrayNoSelfRef verifies that an array with no
// self-ref passes through with a rebuilt slice but unchanged elements.
func TestFoldOptionalSelfRefAbsent_ArrayNoSelfRef(t *testing.T) {
	arr := fsr_array(fsr_scalar("hello"), fsr_scalar("world"))
	got, ok := foldOptionalSelfRefAbsent(arr, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result, isArr := got.(*ArrayVal)
	if !isArr {
		t.Fatalf("expected *ArrayVal, got %T", got)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}
}

// TestFoldOptionalSelfRefAbsent_ArrayMultipleElemsOnlyFoldsSelfRef verifies
// that in a mixed array, only the self-ref element is folded.
func TestFoldOptionalSelfRefAbsent_ArrayMultipleElemsOnlyFoldsSelfRef(t *testing.T) {
	other := fsr_optSubst("b")
	arr := fsr_array(fsr_optSubst("a"), other, fsr_scalar("x"))
	got, ok := foldOptionalSelfRefAbsent(arr, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result := got.(*ArrayVal)
	sp0 := result.Elements[0].(*substPlaceholder)
	if !sp0.knownAbsent {
		t.Error("Elements[0] (self-ref) should be knownAbsent")
	}
	sp1 := result.Elements[1].(*substPlaceholder)
	if sp1.knownAbsent {
		t.Error("Elements[1] (non-self-ref) must not be knownAbsent")
	}
}

// ---------------------------------------------------------------------------
// foldOptionalSelfRefAbsent — ObjectVal branch
// ---------------------------------------------------------------------------

// TestFoldOptionalSelfRefAbsent_ObjectOptionalField verifies that an ObjectVal
// containing an optional self-ref field folds that field's value to knownAbsent.
func TestFoldOptionalSelfRefAbsent_ObjectOptionalField(t *testing.T) {
	obj := fsr_obj1("history", fsr_optSubst("a"))
	got, ok := foldOptionalSelfRefAbsent(obj, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result, isObj := got.(*ObjectVal)
	if !isObj {
		t.Fatalf("expected *ObjectVal, got %T", got)
	}
	v, exists := result.Get("history")
	if !exists {
		t.Fatal("history field missing after fold")
	}
	sp, isSP := v.(*substPlaceholder)
	if !isSP {
		t.Fatalf("history: expected *substPlaceholder, got %T", v)
	}
	if !sp.knownAbsent {
		t.Error("ObjectVal field should be knownAbsent after fold")
	}
}

// TestFoldOptionalSelfRefAbsent_ObjectRequiredField verifies that an ObjectVal
// with a required self-ref field returns nil,false.
func TestFoldOptionalSelfRefAbsent_ObjectRequiredField(t *testing.T) {
	obj := fsr_obj1("history", fsr_reqSubst("a"))
	got, ok := foldOptionalSelfRefAbsent(obj, "a")
	if ok {
		t.Fatalf("expected ok=false for required self-ref in ObjectVal, got %v", got)
	}
	if got != nil {
		t.Errorf("expected nil, got %T", got)
	}
}

// TestFoldOptionalSelfRefAbsent_ObjectNoSelfRef verifies that an ObjectVal with
// no self-ref passes through with rebuilt fields but unchanged values.
func TestFoldOptionalSelfRefAbsent_ObjectNoSelfRef(t *testing.T) {
	obj := fsr_obj1("x", fsr_scalar("42"))
	got, ok := foldOptionalSelfRefAbsent(obj, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result, isObj := got.(*ObjectVal)
	if !isObj {
		t.Fatalf("expected *ObjectVal, got %T", got)
	}
	v, exists := result.Get("x")
	if !exists {
		t.Fatal("x field missing")
	}
	if sv, ok := v.(*ScalarVal); !ok || sv.Raw != "42" {
		t.Errorf("x: expected scalar 42, got %T/%v", v, v)
	}
}

// TestFoldOptionalSelfRefAbsent_ObjectMultipleFieldsOnlyFoldsSelfRef verifies
// that only the field with a self-ref is folded; other fields pass through.
func TestFoldOptionalSelfRefAbsent_ObjectMultipleFieldsOnlyFoldsSelfRef(t *testing.T) {
	obj := newObjectVal()
	obj.set("history", fsr_optSubst("a"))
	obj.set("other", fsr_optSubst("b"))
	got, ok := foldOptionalSelfRefAbsent(obj, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result := got.(*ObjectVal)
	history, _ := result.Get("history")
	sp := history.(*substPlaceholder)
	if !sp.knownAbsent {
		t.Error("history (self-ref) should be knownAbsent")
	}
	other, _ := result.Get("other")
	spOther := other.(*substPlaceholder)
	if spOther.knownAbsent {
		t.Error("other (non-self-ref) must not be knownAbsent")
	}
}

// TestFoldOptionalSelfRefAbsent_ObjectPreservesPriorValues verifies that
// priorValues are copied to the rebuilt ObjectVal.
func TestFoldOptionalSelfRefAbsent_ObjectPreservesPriorValues(t *testing.T) {
	obj := newObjectVal()
	obj.set("history", fsr_optSubst("a"))
	// Install a priorValue that should be preserved verbatim.
	obj.priorValues["history"] = fsr_scalar("prior")
	got, ok := foldOptionalSelfRefAbsent(obj, "a")
	if !ok {
		t.Fatal("expected ok=true")
	}
	result := got.(*ObjectVal)
	pv, hasPrior := result.priorValues["history"]
	if !hasPrior {
		t.Fatal("priorValues should be copied to rebuilt ObjectVal")
	}
	if sv, ok := pv.(*ScalarVal); !ok || sv.Raw != "prior" {
		t.Errorf("priorValues[history]: expected 'prior', got %v", pv)
	}
}

// ---------------------------------------------------------------------------
// foldOptionalSelfRefAbsent — default (scalar) branch
// ---------------------------------------------------------------------------

// TestFoldOptionalSelfRefAbsent_Scalar verifies that ScalarVal falls through
// to the default branch and is returned unchanged.
func TestFoldOptionalSelfRefAbsent_Scalar(t *testing.T) {
	sv := fsr_scalar("hello")
	got, ok := foldOptionalSelfRefAbsent(sv, "a")
	if !ok {
		t.Fatal("expected ok=true for scalar (no self-ref)")
	}
	if got != sv {
		t.Errorf("expected same ScalarVal pointer, got %T", got)
	}
}

// ---------------------------------------------------------------------------
// containsKnownAbsentSentinel — all branches
// ---------------------------------------------------------------------------

// TestContainsKnownAbsentSentinel_SubstAbsent verifies that a knownAbsent
// substPlaceholder returns true.
func TestContainsKnownAbsentSentinel_SubstAbsent(t *testing.T) {
	sp := fsr_absentSubst("a")
	if !containsKnownAbsentSentinel(sp) {
		t.Error("expected true for knownAbsent substPlaceholder")
	}
}

// TestContainsKnownAbsentSentinel_SubstNotAbsent verifies that a non-absent
// substPlaceholder returns false.
func TestContainsKnownAbsentSentinel_SubstNotAbsent(t *testing.T) {
	sp := fsr_optSubst("a")
	if containsKnownAbsentSentinel(sp) {
		t.Error("expected false for non-absent substPlaceholder")
	}
}

// TestContainsKnownAbsentSentinel_ConcatWithAbsent verifies recursive detection
// in concatPlaceholder.
func TestContainsKnownAbsentSentinel_ConcatWithAbsent(t *testing.T) {
	cp := fsr_concat(fsr_absentSubst("a"), fsr_scalar("1"))
	if !containsKnownAbsentSentinel(cp) {
		t.Error("expected true: concat contains knownAbsent element")
	}
}

// TestContainsKnownAbsentSentinel_ConcatWithoutAbsent verifies that a concat
// with no sentinel returns false.
func TestContainsKnownAbsentSentinel_ConcatWithoutAbsent(t *testing.T) {
	cp := fsr_concat(fsr_optSubst("a"), fsr_scalar("1"))
	if containsKnownAbsentSentinel(cp) {
		t.Error("expected false: concat has no knownAbsent element")
	}
}

// TestContainsKnownAbsentSentinel_ArrayWithAbsent verifies recursive detection
// inside ArrayVal elements.
func TestContainsKnownAbsentSentinel_ArrayWithAbsent(t *testing.T) {
	arr := fsr_array(fsr_absentSubst("a"), fsr_scalar("x"))
	if !containsKnownAbsentSentinel(arr) {
		t.Error("expected true: ArrayVal contains knownAbsent element")
	}
}

// TestContainsKnownAbsentSentinel_ArrayWithoutAbsent verifies that an ArrayVal
// with no sentinel returns false.
func TestContainsKnownAbsentSentinel_ArrayWithoutAbsent(t *testing.T) {
	arr := fsr_array(fsr_scalar("x"), fsr_scalar("y"))
	if containsKnownAbsentSentinel(arr) {
		t.Error("expected false: ArrayVal has no knownAbsent element")
	}
}

// TestContainsKnownAbsentSentinel_ObjectWithAbsent verifies recursive detection
// inside ObjectVal field values.
func TestContainsKnownAbsentSentinel_ObjectWithAbsent(t *testing.T) {
	obj := fsr_obj1("history", fsr_absentSubst("a"))
	if !containsKnownAbsentSentinel(obj) {
		t.Error("expected true: ObjectVal has field with knownAbsent")
	}
}

// TestContainsKnownAbsentSentinel_ObjectWithoutAbsent verifies that an ObjectVal
// with no sentinel returns false.
func TestContainsKnownAbsentSentinel_ObjectWithoutAbsent(t *testing.T) {
	obj := fsr_obj1("x", fsr_scalar("42"))
	if containsKnownAbsentSentinel(obj) {
		t.Error("expected false: ObjectVal has no knownAbsent field")
	}
}

// TestContainsKnownAbsentSentinel_Scalar verifies that ScalarVal (default
// branch) always returns false.
func TestContainsKnownAbsentSentinel_Scalar(t *testing.T) {
	if containsKnownAbsentSentinel(fsr_scalar("hello")) {
		t.Error("expected false for ScalarVal (no placeholder inside)")
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — substPlaceholder branch
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_SubstAbsent verifies that a knownAbsent placeholder is
// replaced by the replacement value.
func TestRehydrateSentinel_SubstAbsent(t *testing.T) {
	sp := fsr_absentSubst("a")
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(sp, replacement)
	sv, ok := got.(*ScalarVal)
	if !ok {
		t.Fatalf("expected *ScalarVal (replacement), got %T", got)
	}
	if sv.Raw != "base" {
		t.Errorf("expected 'base', got %q", sv.Raw)
	}
}

// TestRehydrateSentinel_SubstNotAbsent verifies that a non-absent placeholder
// is returned unchanged.
func TestRehydrateSentinel_SubstNotAbsent(t *testing.T) {
	sp := fsr_optSubst("a")
	got := rehydrateSentinel(sp, fsr_scalar("base"))
	if got != sp {
		t.Errorf("expected same substPlaceholder pointer, got %T", got)
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — concatPlaceholder branch
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_ConcatWithSentinel verifies that a concat containing a
// sentinel has the sentinel replaced and a new concatPlaceholder is returned.
func TestRehydrateSentinel_ConcatWithSentinel(t *testing.T) {
	cp := fsr_concat(fsr_absentSubst("a"), fsr_scalar("1"))
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(cp, replacement)
	result, isCP := got.(*concatPlaceholder)
	if !isCP {
		t.Fatalf("expected *concatPlaceholder, got %T", got)
	}
	sv0, ok := result.vals[0].(*ScalarVal)
	if !ok {
		t.Fatalf("vals[0]: expected *ScalarVal (replacement), got %T", result.vals[0])
	}
	if sv0.Raw != "base" {
		t.Errorf("vals[0]: expected 'base', got %q", sv0.Raw)
	}
	sv1, ok := result.vals[1].(*ScalarVal)
	if !ok {
		t.Fatalf("vals[1]: expected *ScalarVal '1', got %T", result.vals[1])
	}
	if sv1.Raw != "1" {
		t.Errorf("vals[1]: expected '1', got %q", sv1.Raw)
	}
}

// TestRehydrateSentinel_ConcatNoSentinel verifies that a concat without a
// sentinel is returned as-is (no new allocation).
func TestRehydrateSentinel_ConcatNoSentinel(t *testing.T) {
	cp := fsr_concat(fsr_scalar("x"), fsr_scalar("y"))
	got := rehydrateSentinel(cp, fsr_scalar("base"))
	if got != cp {
		t.Errorf("expected same concatPlaceholder (no sentinel), got %T", got)
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — ArrayVal branch
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_ArrayWithSentinel verifies that a sentinel element in
// an ArrayVal is replaced by the replacement as a SINGLE element (no splicing).
func TestRehydrateSentinel_ArrayWithSentinel(t *testing.T) {
	arr := fsr_array(fsr_absentSubst("a"), fsr_scalar("x"))
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(arr, replacement)
	result, isArr := got.(*ArrayVal)
	if !isArr {
		t.Fatalf("expected *ArrayVal, got %T", got)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements (single-element replacement, not splice), got %d", len(result.Elements))
	}
	sv0, ok := result.Elements[0].(*ScalarVal)
	if !ok {
		t.Fatalf("Elements[0]: expected *ScalarVal (replacement), got %T", result.Elements[0])
	}
	if sv0.Raw != "base" {
		t.Errorf("Elements[0]: expected 'base', got %q", sv0.Raw)
	}
}

// TestRehydrateSentinel_ArrayNoSentinel verifies that an ArrayVal without a
// sentinel is returned unchanged.
func TestRehydrateSentinel_ArrayNoSentinel(t *testing.T) {
	arr := fsr_array(fsr_scalar("x"), fsr_scalar("y"))
	got := rehydrateSentinel(arr, fsr_scalar("base"))
	if got != arr {
		t.Errorf("expected same ArrayVal (no sentinel), got %T", got)
	}
}

// TestRehydrateSentinel_ArrayReplacementIsArraySingleElement verifies the
// round-3 single-element semantics: when the replacement is itself an ArrayVal,
// it is inserted as ONE element (not spliced/flattened).
func TestRehydrateSentinel_ArrayReplacementIsArraySingleElement(t *testing.T) {
	arr := fsr_array(fsr_absentSubst("a"), fsr_scalar("x"))
	replacement := fsr_array(fsr_scalar("base"))
	got := rehydrateSentinel(arr, replacement)
	result, isArr := got.(*ArrayVal)
	if !isArr {
		t.Fatalf("expected *ArrayVal, got %T", got)
	}
	// Two elements: [replacement_array, "x"] — NOT ["base", "x"]
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements (no flatten), got %d: %v", len(result.Elements), result.Elements)
	}
	inner, isArr2 := result.Elements[0].(*ArrayVal)
	if !isArr2 {
		t.Fatalf("Elements[0]: expected *ArrayVal (single-element, no splice), got %T", result.Elements[0])
	}
	if len(inner.Elements) != 1 {
		t.Fatalf("inner array: expected 1 element, got %d", len(inner.Elements))
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — ObjectVal branch
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_ObjectWithSentinel verifies that a sentinel inside an
// ObjectVal field value is replaced and a new ObjectVal is returned.
func TestRehydrateSentinel_ObjectWithSentinel(t *testing.T) {
	obj := fsr_obj1("history", fsr_absentSubst("a"))
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(obj, replacement)
	result, isObj := got.(*ObjectVal)
	if !isObj {
		t.Fatalf("expected *ObjectVal, got %T", got)
	}
	v, exists := result.Get("history")
	if !exists {
		t.Fatal("history field missing after rehydrate")
	}
	sv, ok := v.(*ScalarVal)
	if !ok {
		t.Fatalf("history: expected *ScalarVal (replacement), got %T", v)
	}
	if sv.Raw != "base" {
		t.Errorf("history: expected 'base', got %q", sv.Raw)
	}
}

// TestRehydrateSentinel_ObjectNoSentinel verifies that an ObjectVal without a
// sentinel in any field is returned unchanged.
func TestRehydrateSentinel_ObjectNoSentinel(t *testing.T) {
	obj := fsr_obj1("x", fsr_scalar("42"))
	got := rehydrateSentinel(obj, fsr_scalar("base"))
	if got != obj {
		t.Errorf("expected same ObjectVal (no sentinel), got %T", got)
	}
}

// TestRehydrateSentinel_ObjectPreservesPriorValues verifies that priorValues
// are copied to the rebuilt ObjectVal during rehydration.
func TestRehydrateSentinel_ObjectPreservesPriorValues(t *testing.T) {
	obj := fsr_obj1("history", fsr_absentSubst("a"))
	obj.priorValues["history"] = fsr_scalar("old")
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(obj, replacement)
	result := got.(*ObjectVal)
	pv, hasPrior := result.priorValues["history"]
	if !hasPrior {
		t.Fatal("priorValues should be copied to rehydrated ObjectVal")
	}
	if sv, ok := pv.(*ScalarVal); !ok || sv.Raw != "old" {
		t.Errorf("priorValues[history]: expected 'old', got %v", pv)
	}
}

// TestRehydrateSentinel_ObjectMultipleFieldsOnlyChangedField verifies that
// when only one field has a sentinel, the rehydrated ObjectVal only changes
// that field; other fields are preserved.
func TestRehydrateSentinel_ObjectMultipleFieldsOnlyChangedField(t *testing.T) {
	obj := newObjectVal()
	obj.set("history", fsr_absentSubst("a"))
	obj.set("other", fsr_scalar("untouched"))
	got := rehydrateSentinel(obj, fsr_scalar("base"))
	result := got.(*ObjectVal)
	history, _ := result.Get("history")
	if sv, ok := history.(*ScalarVal); !ok || sv.Raw != "base" {
		t.Errorf("history: expected 'base', got %v", history)
	}
	other, _ := result.Get("other")
	if sv, ok := other.(*ScalarVal); !ok || sv.Raw != "untouched" {
		t.Errorf("other: expected 'untouched', got %v", other)
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — default (scalar) branch
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_Scalar verifies that ScalarVal (default branch) is
// returned unchanged.
func TestRehydrateSentinel_Scalar(t *testing.T) {
	sv := fsr_scalar("hello")
	got := rehydrateSentinel(sv, fsr_scalar("base"))
	if got != sv {
		t.Errorf("expected same ScalarVal, got %T", got)
	}
}

// ---------------------------------------------------------------------------
// foldSelfRef — uncovered branches (93.8% → supplement)
// ---------------------------------------------------------------------------

// TestFoldSelfRef_ObjectValNoSelfRef verifies that an ObjectVal with no
// self-ref returns the original value unchanged with hit=false.
func TestFoldSelfRef_ObjectValNoSelfRef(t *testing.T) {
	obj := fsr_obj1("x", fsr_scalar("val"))
	got, hit := foldSelfRef(obj, "a", fsr_scalar("replacement"))
	if hit {
		t.Error("expected hit=false when ObjectVal has no self-ref to target key")
	}
	if got != obj {
		t.Errorf("expected same ObjectVal, got %T", got)
	}
}

// TestFoldSelfRef_ObjectValPresenceCheck verifies the presence-only check
// (replacement=nil) on an ObjectVal containing a self-ref field.
func TestFoldSelfRef_ObjectValPresenceCheck(t *testing.T) {
	obj := fsr_obj1("history", fsr_optSubst("a"))
	got, hit := foldSelfRef(obj, "a", nil)
	if !hit {
		t.Error("expected hit=true for ObjectVal containing self-ref (presence check)")
	}
	if got != obj {
		t.Errorf("expected original ObjectVal returned for presence-only check, got %T", got)
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — ArrayVal lazy-copy path for nested non-sentinel change
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_ArrayNestedSentinelInConcat covers the branch where a
// non-sentinel element in an ArrayVal is itself changed by recursive rehydration
// (e.g. a concat containing a sentinel). This triggers the lazy-copy path for
// elements preceding the changed one (lines 360-366 in foldselfref.go).
func TestRehydrateSentinel_ArrayNestedSentinelInConcat(t *testing.T) {
	// Build: ["before", concat(knownAbsent, "1")]
	// The second element is a concatPlaceholder with a sentinel; rehydrate
	// recurses into it and returns a new concat — ne != e triggers lazy copy.
	concatWithSentinel := fsr_concat(fsr_absentSubst("a"), fsr_scalar("1"))
	arr := fsr_array(fsr_scalar("before"), concatWithSentinel)
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(arr, replacement)
	result, isArr := got.(*ArrayVal)
	if !isArr {
		t.Fatalf("expected *ArrayVal, got %T", got)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}
	// First element should be the unchanged scalar "before".
	sv, ok := result.Elements[0].(*ScalarVal)
	if !ok {
		t.Fatalf("Elements[0]: expected *ScalarVal 'before', got %T", result.Elements[0])
	}
	if sv.Raw != "before" {
		t.Errorf("Elements[0]: expected 'before', got %q", sv.Raw)
	}
	// Second element should be a new concatPlaceholder with sentinel replaced.
	cp, isCP := result.Elements[1].(*concatPlaceholder)
	if !isCP {
		t.Fatalf("Elements[1]: expected *concatPlaceholder, got %T", result.Elements[1])
	}
	sv0, ok0 := cp.vals[0].(*ScalarVal)
	if !ok0 {
		t.Fatalf("concat vals[0]: expected *ScalarVal (replacement), got %T", cp.vals[0])
	}
	if sv0.Raw != "base" {
		t.Errorf("concat vals[0]: expected 'base', got %q", sv0.Raw)
	}
}

// ---------------------------------------------------------------------------
// rehydrateSentinel — ObjectVal lazy-copy of preceding keys
// ---------------------------------------------------------------------------

// TestRehydrateSentinel_ObjectSentinelInSecondFieldCopiesFirst covers the
// ObjectVal branch where the sentinel is in the second field (not the first).
// The lazy-init path must copy the first field into newObj before processing
// the sentinel field (line 393 in foldselfref.go).
func TestRehydrateSentinel_ObjectSentinelInSecondFieldCopiesFirst(t *testing.T) {
	// Build: { first = "keep", second = knownAbsent }
	obj := newObjectVal()
	obj.set("first", fsr_scalar("keep"))
	obj.set("second", fsr_absentSubst("a"))
	replacement := fsr_scalar("base")
	got := rehydrateSentinel(obj, replacement)
	result, isObj := got.(*ObjectVal)
	if !isObj {
		t.Fatalf("expected *ObjectVal, got %T", got)
	}
	// "first" should be carried over by the lazy-copy loop.
	firstVal, ok := result.Get("first")
	if !ok {
		t.Fatal("first field should be copied to rebuilt ObjectVal")
	}
	if sv, ok := firstVal.(*ScalarVal); !ok || sv.Raw != "keep" {
		t.Errorf("first: expected 'keep', got %v", firstVal)
	}
	// "second" should have the sentinel replaced.
	secondVal, ok := result.Get("second")
	if !ok {
		t.Fatal("second field missing after rehydrate")
	}
	if sv, ok := secondVal.(*ScalarVal); !ok || sv.Raw != "base" {
		t.Errorf("second: expected 'base', got %v", secondVal)
	}
}
