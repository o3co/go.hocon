// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// xx.hocon#123 — a REQUIRED substitution whose target field is in the document
// but was removed by an undefined optional (S13.11) is unresolved (S13.10):
//
//	a = ${?nope}
//	b = ${a}      // Lightbend: UnresolvedSubstitution; go.hocon was {} (b dropped)
//
// Only an optional `${?x}` may itself become undefined and drop its field /
// array element / concat operand. The resolver used to pass the target's
// "undefined" straight up as the referring substitution's own result whatever
// its optional flag, so the required `${a}` silently vanished (silent data
// loss). The same applies whatever removed the target: an optional cycle, an
// optional self-reference with no prior (S13a.4), or an optional leading back
// to its own object; and through an include's path fix-up (S14c.2), at both
// the fixed-up and the original path.
//
// Hidden references (S13a.5) are never evaluated and must not start erroring,
// and a target whose earlier / later value survives the optional still
// resolves. Those are pinned below as regression guards.
//
// Fixtures: testdata/hocon/subst-removed-target/rq01–rq16 and rq29 (errors),
// rq17–rq28 (success guards), rq30–rq33 (`-pending-125`: Lightbend values
// skipped until o3co/xx.hocon#125), with `rq<NN>-inner.conf` include targets.
// Expected: testdata/expected/subst-removed-target/, `<stem>.error` = must
// fail, `<stem>-expected.json` = must succeed with this value (Lightbend
// 1.4.9). Until the xx.hocon group reaches main, the expected dir is absent
// from a `make testdata` fetch and the fixture test skips; the inline tests
// below pin the behaviour regardless.

package hocon_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

const (
	rqConfDir     = "testdata/hocon/subst-removed-target"
	rqExpectedDir = "testdata/expected/subst-removed-target"
)

// unsetEnv makes sure the named variables are absent for the test (restored on
// cleanup), so the undefined optionals below are genuinely undefined.
func unsetEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		t.Setenv(n, "")
		if err := os.Unsetenv(n); err != nil {
			t.Fatalf("Unsetenv(%s): %v", n, err)
		}
	}
}

// requireUnresolved asserts err is the resolver's ordinary unresolved-
// substitution error (the same one a path missing from the document gives)
// naming wantPath.
func requireUnresolved(t *testing.T, err error, wantPath string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an unresolved-substitution error for ${%s}, got success", wantPath)
	}
	var re *hocon.ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *hocon.ResolveError, got %T: %v", err, err)
	}
	if re.Message != "unresolved substitution" || re.Path != wantPath {
		t.Errorf("got ResolveError{Message: %q, Path: %q}, want {Message: %q, Path: %q}",
			re.Message, re.Path, "unresolved substitution", wantPath)
	}
}

func requireJSON(t *testing.T, cfg *hocon.Config, wantJSON string) {
	t.Helper()
	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("bad want JSON %q: %v", wantJSON, err)
	}
	got := make(map[string]any)
	if err := cfg.Unmarshal(&got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !jsonEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		t.Errorf("got %s, want %s", gotJSON, wantJSON)
	}
}

// TestS13_10_SubstRemovedTargetFixtures drives rq01–rq33 (rq30–rq33 skip as
// pending o3co/xx.hocon#125).
func TestS13_10_SubstRemovedTargetFixtures(t *testing.T) {
	unsetEnv(t, "GH123_UNSET", "GH123_UNSET2", "GH123_HOST", "GH123_APP_HOST")
	runSidecarFixtures(t, "subst-removed-target", rqConfDir, rqExpectedDir)
}

// TestIssue123_RequiredSubstToRemovedFieldErrors covers every position a
// required substitution can take and every way its target can be removed.
func TestIssue123_RequiredSubstToRemovedFieldErrors(t *testing.T) {
	unsetEnv(t, "GH123_UNSET", "GH123_UNSET2", "GH123_Z")
	cases := []struct {
		name string
		src  string
		path string
	}{
		{"field-value", "a = ${?GH123_UNSET}\nb = ${a}\n", "a"},
		{"forward-ref", "b = ${a}\na = ${?GH123_UNSET}\n", "a"},
		{"both-optionals-undefined", "a = ${?GH123_UNSET}${?GH123_UNSET2}\nb = ${a}\n", "a"},
		{"array-element", "a = ${?GH123_UNSET}\nb = [${a}]\n", "a"},
		{"string-concat", "a = ${?GH123_UNSET}\nb = x${a}\n", "a"},
		{"object-concat", "a = ${?GH123_UNSET}\nb = ${a} {x=1}\n", "a"},
		{"array-concat", "a = ${?GH123_UNSET}\nb = [1] ${a}\n", "a"},
		{"whitespace-concat-with-optional", "a = ${?GH123_UNSET}\nb = ${a} ${?GH123_Z}\n", "a"},
		{"nested-path", "x { y = ${?GH123_UNSET} }\nb = ${x.y}\n", "x.y"},
		{"optional-cycle", "a = ${?b}\nb = ${?a}\nc = ${a}\n", "a"},
		{"optional-self-ref", "a = ${?a}\nb = ${a}\n", "a"},
		{"optional-then-optional-self-ref", "a = ${?GH123_UNSET}\na = ${?a}\nb = ${a}\n", "a"},
		{"optional-closes-cycle", "o = ${a.n}\na { n = ${?a} }\n", "a.n"},
		// A later object MERGES with b (it does not hide it, cf.
		// hidden-nested-reference below), so b.c = ${a} is evaluated.
		{"merged-not-hidden", "a = ${?GH123_UNSET}\nb { c = ${a} }\nb = { d = 1 }\n", "a"},
		// The undefined optional keeps b's prior value, and that prior is the
		// required ${a} to the removed field.
		{"kept-prior-is-required-ref", "a = ${?GH123_UNSET}\nb = ${a}\nb = ${?GH123_UNSET2}\n", "a"},
		// A required self-reference looks back to the value below it; when the
		// only value below is a removed optional there is nothing to find.
		{"self-ref-to-removed-prior", "a = ${?GH123_UNSET}\na = ${a}\n", "a"},
		{"self-ref-concat-to-removed-prior", "a = ${?GH123_UNSET}\na = ${a}x\n", "a"},
		{"self-ref-array-to-removed-prior", "a = ${?GH123_UNSET}\na = [${a}]\n", "a"},
		{"self-ref-object-concat-to-removed-prior", "a = ${?GH123_UNSET}\na = ${a} {x=1}\n", "a"},
		{"optional-then-required-self-ref", "a = ${?GH123_UNSET}\na = ${?a}${a}\n", "a"},
		{"nested-self-ref-to-removed-prior", "x { y = ${?GH123_UNSET} }\nx { y = ${x.y}z }\n", "x.y"},
		{"self-ref-inside-object-to-removed-prior", "foo = ${?GH123_UNSET}\nfoo = { b = ${foo} }\n", "foo"},
		// Already an error before the fix (a is not an object, so a.c is not
		// in the document); kept so both shapes stay one error kind.
		{"path-into-removed-field", "a = ${?GH123_UNSET}\nb = ${a.c}\n", "a.c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := hocon.ParseString(tc.src)
			requireUnresolved(t, err, tc.path)
		})
	}
}

// TestIssue123_IncludeFixUp covers the include path fix-up (S14c.2): the
// included `${x.y}` is fixed up to `${common.x.y}`; when that field is in the
// document but removed, the lookup ends there (error), and when it is not in
// the document the original `x.y` is tried — an error again if that one is
// removed, its value if it has one.
func TestIssue123_IncludeFixUp(t *testing.T) {
	unsetEnv(t, "GH123_UNSET", "GH123_UNSET2")
	cases := []struct {
		name  string
		main  string
		inner string
		want  string // "" = must fail with ${common.x.y} unresolved
	}{
		{"fixed-up-path-removed", `common { include "inner.conf" }` + "\n", "x.y = ${?GH123_UNSET}\na = ${x.y}\n", ""},
		{"both-paths-removed", `common { include "inner.conf" }` + "\nx.y = ${?GH123_UNSET2}\n", "x.y = ${?GH123_UNSET}\na = ${x.y}\n", ""},
		// The fixed-up field is found (and removed), so the original x.y is not
		// a fallback even though it has a value (Lightbend 1.4.9).
		{"fixed-up-removed-original-set", `common { include "inner.conf" }` + "\nx.y = 1\n", "x.y = ${?GH123_UNSET}\na = ${x.y}\n", ""},
		{"original-path-removed", `common { include "inner.conf" }` + "\nx.y = ${?GH123_UNSET}\n", "a = ${x.y}\n", ""},
		// The original x.y is removed by the optional, but its earlier value
		// stays (the S14c.2 fallback honours the target's prior).
		{"original-path-prior-kept", `common { include "inner.conf" }` + "\nx.y = 1\nx.y = ${?GH123_UNSET}\n", "a = ${x.y}\n", `{"common":{"a":1},"x":{"y":1}}`},
		{"original-path-ok", `common { include "inner.conf" }` + "\nx.y = 1\n", "a = ${x.y}\n", `{"common":{"a":1},"x":{"y":1}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mainPath := filepath.Join(dir, "main.conf")
			if err := os.WriteFile(mainPath, []byte(tc.main), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "inner.conf"), []byte(tc.inner), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := hocon.ParseFile(mainPath)
			if tc.want == "" {
				requireUnresolved(t, err, "common.x.y")
				return
			}
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			requireJSON(t, cfg, tc.want)
		})
	}
}

// TestIssue123_UnchangedCases are regression guards: optional references to a
// removed field, targets whose value survives the optional, hidden references
// (S13a.5) and an emptied parent object all keep resolving as before.
func TestIssue123_UnchangedCases(t *testing.T) {
	unsetEnv(t, "GH123_UNSET")
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"optional-ref-to-removed", "a = ${?GH123_UNSET}\nb = ${?a}\n", `{}`},
		{"prior-value-kept", "a = 1\na = ${?GH123_UNSET}\nb = ${a}\n", `{"a":1,"b":1}`},
		{"prior-object-kept", "a { x = 1 }\na = ${?GH123_UNSET}\nb = ${a}\n", `{"a":{"x":1},"b":{"x":1}}`},
		{"later-value-wins", "a = ${?GH123_UNSET}\na = 2\nb = ${a}\n", `{"a":2,"b":2}`},
		{"hidden-reference", "a = ${?GH123_UNSET}\nb = ${a}\nb = 5\n", `{"b":5}`},
		{"hidden-nested-reference", "a = ${?GH123_UNSET}\nb { c = ${a} }\nb = 5\n", `{"b":5}`},
		{"empty-parent-object", "x { y = ${?GH123_UNSET} }\nb = ${x}\n", `{"b":{},"x":{}}`},
		{"plus-equals", "a += 1\nb = ${a}\n", `{"a":[1],"b":[1]}`},
		// The optional machinery on its own, without a required reference.
		{"optional-cycle", "a = ${?b}\nb = ${?a}\n", `{}`},
		{"optional-closes-cycle", "a { n = ${?a} }\n", `{"a":{}}`},
		{"optional-ref-into-cycle", "o = ${?a.n}\na { n = ${?a} }\n", `{"a":{}}`},
		{"optional-self-ref", "a = ${?a}\n", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := hocon.ParseString(tc.src)
			if err != nil {
				t.Fatalf("ParseString(%q): %v", tc.src, err)
			}
			requireJSON(t, cfg, tc.want)
		})
	}
}

// TestIssue123_PendingMultiLevelPrior holds the Lightbend values for shapes
// whose target survives only through a value TWO layers down (rq30–rq33).
// The resolver keeps a single-level prior per key, so the bottom layer is lost
// and the field looks removed: before this fix the reference was dropped
// silently, now it errors. Both are wrong; the fix is o3co/xx.hocon#125's
// multi-level prior, and this change ships in the same minor as it. Remove the
// skip when #125 lands — the assertions are the Lightbend 1.4.9 values.
func TestIssue123_PendingMultiLevelPrior(t *testing.T) {
	unsetEnv(t, "GH123_HOST", "GH123_APP_HOST", "GH123_UNSET", "GH123_UNSET2")
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"two-overrides-then-reference", "h = localhost\nh = ${?GH123_HOST}\nh = ${?GH123_APP_HOST}\nurl = \"http://\"${h}\n", `{"h":"localhost","url":"http://localhost"}`},
		{"two-optionals-then-reference", "a = 1\na = ${?GH123_UNSET}\na = ${?GH123_UNSET2}\nb = ${a}\n", `{"a":1,"b":1}`},
		{"forward-reference-two-optionals", "b = ${a}\na = 1\na = ${?GH123_UNSET}\na = ${?GH123_UNSET2}\n", `{"a":1,"b":1}`},
		{"self-ref-chain-then-reference", "a = 1\na = ${?GH123_UNSET}\na = ${a}\nb = ${a}\n", `{"a":1,"b":1}`},
		{"self-ref-through-optional", "a = 1\na = ${?GH123_UNSET}\na = ${a}\n", `{"a":1}`},
		{"self-ref-concat-through-optional", "a = 1\na = ${?GH123_UNSET}\na = ${a}x\n", `{"a":"1x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Skip("pending o3co/xx.hocon#125: the multi-level prior is lost, so the surviving bottom value is not found")
			cfg, err := hocon.ParseString(tc.src)
			if err != nil {
				t.Fatalf("ParseString(%q): %v", tc.src, err)
			}
			requireJSON(t, cfg, tc.want)
		})
	}
}

// TestIssue123_PendingHiddenInMergedObject holds the Lightbend values for a
// reference that a later object merge hides: `b { c = ${a} }` then
// `b { c = 5 }` never evaluates the first c (HOCON.md L776). The resolver
// resolves the whole earlier object before merging (o3co/xx.hocon#126, which
// already errors for `${nope}` there), so with this fix a removed target
// errors there too. Remove the skip when #126 lands.
func TestIssue123_PendingHiddenInMergedObject(t *testing.T) {
	unsetEnv(t, "GH123_UNSET", "GH123_B")
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"object-then-object", "a = ${?GH123_UNSET}\nb { c = ${a} }\nb { c = 5 }\n", `{"b":{"c":5}}`},
		{"object-then-assigned-object", "a = ${?GH123_UNSET}\nb { c = ${a} }\nb = { c = 5 }\n", `{"b":{"c":5}}`},
		{"concat-hidden", "a = ${?GH123_UNSET}\nb { c = ${a}x }\nb { c = 5 }\n", `{"b":{"c":5}}`},
		{"nested", "a = ${?GH123_UNSET}\nx { y { c = ${a} } }\nx { y { c = 5 } }\n", `{"x":{"y":{"c":5}}}`},
		{"defaults-then-override", "base = ${?GH123_B}\nservice { endpoint = ${base}\"/api\" }\nservice { endpoint = \"http://x\" }\n", `{"service":{"endpoint":"http://x"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Skip("pending o3co/xx.hocon#126: a field hidden by a later object merge is still evaluated")
			cfg, err := hocon.ParseString(tc.src)
			if err != nil {
				t.Fatalf("ParseString(%q): %v", tc.src, err)
			}
			requireJSON(t, cfg, tc.want)
		})
	}
}

// TestIssue123_EnvNotConsulted: the target field is in the document, so the
// lookup ends there — an environment variable of the same name is not a
// fallback for it, for a required or an optional reference (Lightbend 1.4.9).
func TestIssue123_EnvNotConsulted(t *testing.T) {
	unsetEnv(t, "GH123_UNSET")
	t.Setenv("gh123a", "E")

	_, err := hocon.ParseString("gh123a = ${?GH123_UNSET}\nb = ${gh123a}\n")
	requireUnresolved(t, err, "gh123a")

	cfg, err := hocon.ParseString("gh123a = ${?GH123_UNSET}\nb = ${?gh123a}\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	requireJSON(t, cfg, `{}`)
}

// TestIssue123_DeferredResolution: the rule applies to Resolve() of a deferred
// tree; AllowUnresolved leaves the reference as an unresolved placeholder like
// any other; and a fallback layer that supplies the target makes it resolve.
func TestIssue123_DeferredResolution(t *testing.T) {
	unsetEnv(t, "GH123_UNSET")
	deferred := hocon.DefaultParseOptions().WithResolveSubstitutions(false)
	src := "a = ${?GH123_UNSET}\nb = ${a}\n"

	t.Run("strict", func(t *testing.T) {
		c, err := hocon.ParseStringWithOptions(src, deferred)
		if err != nil {
			t.Fatalf("ParseStringWithOptions: %v", err)
		}
		_, err = c.Resolve(hocon.DefaultResolveOptions())
		requireUnresolved(t, err, "a")
	})

	t.Run("allow-unresolved", func(t *testing.T) {
		c, err := hocon.ParseStringWithOptions(src, deferred)
		if err != nil {
			t.Fatalf("ParseStringWithOptions: %v", err)
		}
		r, err := c.Resolve(hocon.DefaultResolveOptions().WithAllowUnresolved(true))
		if err != nil {
			t.Fatalf("Resolve(allowUnresolved): %v", err)
		}
		if r.IsResolved() {
			t.Error("expected unresolved: b must stay a ${a} placeholder")
		}
		if r.Has("a") {
			t.Error("a is removed by the undefined optional and must not exist")
		}
		defer func() {
			rec := recover()
			ce, ok := rec.(*hocon.ConfigError)
			if !ok || !errors.Is(ce, hocon.ErrNotResolved) {
				t.Fatalf("expected NotResolved panic on b, got %T %v", rec, rec)
			}
		}()
		_ = r.GetString("b")
	})

	t.Run("fallback-supplies-target", func(t *testing.T) {
		r, err := hocon.ParseStringWithOptions(src, deferred)
		if err != nil {
			t.Fatalf("ParseStringWithOptions: %v", err)
		}
		f, err := hocon.ParseStringWithOptions("a = 1\n", deferred)
		if err != nil {
			t.Fatalf("ParseStringWithOptions: %v", err)
		}
		out, err := r.WithFallback(f).Resolve(hocon.DefaultResolveOptions())
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		requireJSON(t, out, `{"a":1,"b":1}`)
	})
}
