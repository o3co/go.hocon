// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import (
	"math"
	"strings"
	"testing"

	"github.com/o3co/go.hocon/internal/lexer"
)

// go.hocon#218: unit tests for the interim guard's helpers. They walk trees
// that share children (current values and saved priors do), so each container
// must be counted once.

func subst(segs ...string) *substPlaceholder {
	s := &substPlaceholder{}
	for _, t := range segs {
		s.segments = append(s.segments, lexer.Segment{Text: t})
	}
	return s
}

func sharedTree() (*ObjectVal, *ArrayVal, *concatPlaceholder) {
	arr := &ArrayVal{Elements: []Val{subst("a", "b", "c"), &ScalarVal{Raw: strings.Repeat("x", 32)}}}
	cat := &concatPlaceholder{vals: []Val{subst("d"), arr}}
	child := newObjectVal()
	child.set("arr", arr)
	child.set("cat", cat)
	root := newObjectVal()
	root.set("x", child)
	root.set("y", child) // shared child
	root.priorValues = map[string]Val{"x": child, "z": arr, "w": cat}
	return root, arr, cat
}

func TestIssue218_MaxSubstLenVisitsSharedContainersOnce(t *testing.T) {
	root, _, _ := sharedTree()
	if got := maxSubstLenOf(root); got != 3 {
		t.Errorf("maxSubstLenOf = %d, want 3", got)
	}
}

func TestIssue218_SourceNodesCountsSharedContainersOnce(t *testing.T) {
	root, _, _ := sharedTree()
	// root, child, arr (+2 elements), cat (+1 subst; arr already counted).
	if got := sourceNodes(root, map[Val]struct{}{}); got != 7 {
		t.Errorf("sourceNodes = %d, want 7", got)
	}
}

func TestIssue218_ValueWeightCountsSharedContainersOnce(t *testing.T) {
	long := strings.Repeat("k", 48)                  // a key of 48 bytes weighs 48/16 = 3
	leaf := &ScalarVal{Raw: strings.Repeat("y", 48)} // 1 + 48/16 = 4
	arr := &ArrayVal{Elements: []Val{leaf, leaf}}
	inner := newObjectVal()
	inner.set(long, &ScalarVal{Raw: "1"}) // key 3 + scalar 1
	obj := newObjectVal()
	obj.set("a", arr)
	obj.set("b", arr) // shared array: its contents count once
	obj.set("c", &ScalarVal{Raw: "z"})
	obj.set(long+"d", inner)
	obj.set(long+"e", inner) // a distinct parent key counts even though its value is shared
	// obj 1
	// + arr 1 + leaf 4 twice (scalars are not tracked as shared) = 9, once
	// + c 1
	// + key long+"d" 3 + inner (1 + key 3 + scalar 1 = 5)
	// + key long+"e" 3 (inner already counted)
	if got := valueWeight(obj, map[Val]struct{}{}); got != 1+9+1+3+5+3 {
		t.Errorf("valueWeight = %d, want %d", got, 1+9+1+3+5+3)
	}
	if got := valueWeight(subst("q"), map[Val]struct{}{}); got != 1 {
		t.Errorf("valueWeight(other) = %d, want 1", got)
	}
}

func TestIssue218_ValueBudgetForDoesNotOverflow(t *testing.T) {
	for _, n := range []int{0, 1, 199, 200, 214_749, math.MaxInt32, math.MaxInt} {
		got := valueBudgetFor(n)
		want := maxValueBudget
		if n < maxValueBudget/valueNodesPerSourceNode {
			want = n * valueNodesPerSourceNode
		}
		if got != want || got < 0 {
			t.Errorf("valueBudgetFor(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestIssue218_ChargeValueInitializesTheCappedBudget(t *testing.T) {
	root := newObjectVal()
	arr := &ArrayVal{}
	for i := 0; i < 300; i++ { // more than maxValueBudget / valueNodesPerSourceNode
		arr.Elements = append(arr.Elements, &ScalarVal{Raw: "1"})
	}
	root.set("pad", arr)
	r := &resolver{cuts: 1}
	if err := r.chargeValue(&ScalarVal{Raw: "x"}, root, "a", 1, 1); err != nil {
		t.Fatalf("chargeValue: %v", err)
	}
	if !r.valueBudgetSet || r.valueBudget != maxValueBudget-1 {
		t.Errorf("valueBudget = %d (set %v), want %d", r.valueBudget, r.valueBudgetSet, maxValueBudget-1)
	}
}

func TestIssue218_CycleStateCoversEveryComponent(t *testing.T) {
	r := &resolver{
		resolving:             map[string]bool{"b": true, "a": true},
		resolvingPrefixPriors: map[string]bool{"o": true},
		resolvingPriors:       map[string]priorReentry{"a": {resolving: 1, cached: 2, prefixPriors: 3, fieldPath: 4}},
		resolvedCache:         map[string]Val{"k": &ScalarVal{Raw: "1"}},
		resolvingFieldPath:    []string{"o", "x"},
	}
	got := r.cycleState(true)
	for _, want := range []string{"?", "\x00a\x00b\x01", "\x00o\x01", "\x00a=1,2,3,4", "\x011\x01o.x"} {
		if !strings.Contains(got, want) {
			t.Errorf("cycleState %q lacks %q", got, want)
		}
	}
	if r.cycleState(false) == got {
		t.Error("cycleState ignores optional")
	}
}

func TestIssue218_ChargeValueOnlyAfterACut(t *testing.T) {
	root := newObjectVal()
	root.set("a", &ScalarVal{Raw: "1"})
	big := &ArrayVal{}
	for i := 0; i < 100; i++ {
		big.Elements = append(big.Elements, &ScalarVal{Raw: "v"})
	}
	r := &resolver{}
	if err := r.chargeValue(big, root, "a", 0, 0); err != nil {
		t.Fatalf("charged before any cut: %v", err)
	}
	r.cuts = 1
	r.valueBudget, r.valueBudgetSet = 50, true
	if err := r.chargeValue(big, root, "a", 1, 1); err == nil {
		t.Error("expected the budget error after a cut")
	}
}
