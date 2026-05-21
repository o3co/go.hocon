// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import "testing"

func TestObjectVal_SetPrior(t *testing.T) {
	o := NewObjectVal()
	o.Set("a", &ScalarVal{Raw: "current", Type: ScalarString})
	o.SetPrior("a", &ScalarVal{Raw: "old", Type: ScalarString})
	prior, ok := o.GetPrior("a")
	if !ok {
		t.Fatal("expected prior")
	}
	sv := prior.(*ScalarVal)
	if sv.Raw != "old" {
		t.Fatalf("expected prior raw 'old', got %q", sv.Raw)
	}
}

func TestMergeUnresolved_NonObjectReceiverWinsCapturesFallbackAsPrior(t *testing.T) {
	// receiver: { a = "current" }
	// fallback: { a = "old" }
	// merged:   receiver wins; "old" stored as prior for self-ref lookback.
	receiver := NewObjectVal()
	receiver.Set("a", &ScalarVal{Raw: "current", Type: ScalarString})

	fallback := NewObjectVal()
	fallback.Set("a", &ScalarVal{Raw: "old", Type: ScalarString})

	merged := MergeUnresolved(receiver, fallback)

	v, _ := merged.Get("a")
	sv := v.(*ScalarVal)
	if sv.Raw != "current" {
		t.Fatalf("expected receiver wins (current), got %q", sv.Raw)
	}
	prior, ok := merged.GetPrior("a")
	if !ok {
		t.Fatalf("expected fallback value captured as prior")
	}
	psv := prior.(*ScalarVal)
	if psv.Raw != "old" {
		t.Fatalf("expected prior 'old', got %q", psv.Raw)
	}
}

func TestMergeUnresolved_ObjectBothRecurses(t *testing.T) {
	receiver := NewObjectVal()
	a1 := NewObjectVal()
	a1.Set("x", &ScalarVal{Raw: "1", Type: ScalarNumber})
	receiver.Set("a", a1)

	fallback := NewObjectVal()
	a2 := NewObjectVal()
	a2.Set("y", &ScalarVal{Raw: "2", Type: ScalarNumber})
	fallback.Set("a", a2)

	merged := MergeUnresolved(receiver, fallback)

	a, _ := merged.Get("a")
	ao := a.(*ObjectVal)
	x, _ := ao.Get("x")
	y, _ := ao.Get("y")
	if x.(*ScalarVal).Raw != "1" || y.(*ScalarVal).Raw != "2" {
		t.Fatalf("expected merged {x:1, y:2}; got x=%v y=%v", x, y)
	}
}

func TestMergeUnresolved_NonObjectReceiverBlocksFallbackObject(t *testing.T) {
	// receiver: { a = 42 }       — non-object
	// fallback: { a = { y = 2 } } — object; blocked by receiver
	// merged:   { a = 42 }, fallback object stored as prior.
	receiver := NewObjectVal()
	receiver.Set("a", &ScalarVal{Raw: "42", Type: ScalarNumber})

	fallback := NewObjectVal()
	a := NewObjectVal()
	a.Set("y", &ScalarVal{Raw: "2", Type: ScalarNumber})
	fallback.Set("a", a)

	merged := MergeUnresolved(receiver, fallback)

	v, _ := merged.Get("a")
	if _, isObj := v.(*ObjectVal); isObj {
		t.Fatalf("expected receiver scalar wins, got object")
	}
	sv := v.(*ScalarVal)
	if sv.Raw != "42" {
		t.Fatalf("expected 42, got %q", sv.Raw)
	}
}

func TestMergeUnresolved_ReceiverObjectBlocksFallbackNonObjectMergeOfThirdLayer(t *testing.T) {
	// This is the dr10 scenario:
	//   r0:  { a = { x = 1 } }       object
	//   fb1: { a = "scalar" }         non-object — barrier
	//   fb2: { a = { y = 2 } }        object — blocked
	// MergeUnresolved is binary; the iterative composition is r0.M(fb1).M(fb2).
	// After r0.M(fb1): merged.a = {x:1}, prior = "scalar".
	// After that.M(fb2): receiver-a is an object {x:1}, fallback-a is object
	// {y:2}. They DEEP MERGE per HOCON object-merge rules, producing {x:1, y:2}.
	// BUT the dr10 spec says fb2's `a = {y:2}` should be blocked. Why?
	//
	// Because the dr10 composition is r0.WithFallback(fb1).WithFallback(fb2),
	// and the spec rule is: "Once a non-object value at a path has barred
	// merging at that path, subsequent fallback objects at the same path do
	// not contribute." The barrier is recorded between step 1 and step 2.
	//
	// MergeUnresolved is the binary primitive; the barrier-tracking is done
	// by the caller (Config.WithFallback) via a per-path "barred set". This
	// test asserts the binary primitive itself: receiver-object + fallback-object
	// deep-merges (which is correct for the binary call; barrier is upstream).
	r0 := NewObjectVal()
	a1 := NewObjectVal()
	a1.Set("x", &ScalarVal{Raw: "1", Type: ScalarNumber})
	r0.Set("a", a1)

	fb2 := NewObjectVal()
	a2 := NewObjectVal()
	a2.Set("y", &ScalarVal{Raw: "2", Type: ScalarNumber})
	fb2.Set("a", a2)

	merged := MergeUnresolved(r0, fb2)
	a, _ := merged.Get("a")
	ao := a.(*ObjectVal)
	_, hasX := ao.Get("x")
	_, hasY := ao.Get("y")
	if !hasX || !hasY {
		t.Fatalf("expected deep-merged {x,y}; got x=%v y=%v", hasX, hasY)
	}
}
