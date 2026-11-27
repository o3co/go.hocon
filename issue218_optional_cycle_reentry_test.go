// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// go.hocon#218 — a substitution cycle written twice (`a = ${?b}` /
// `b = ${?a}` twice) overflowed the stack and killed the process: the
// in-progress branch of the resolver re-entered a key's saved prior forever.
// The interim guard (resolver.priorReentry) treats a re-entry that sees no
// new progress as an unbreakable cycle; the full fix is the resolver
// redesign in o3co/xx.hocon#67 / #125. Expected values are Lightbend 1.4.9.
//
// Fixtures: testdata/hocon/optional-cycle-repeat/oc01–oc06 (o3co/xx.hocon#136).

package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hocon "github.com/o3co/go.hocon"
)

// TestIssue218_OptionalCycleRepeatFixtures drives oc01–oc06.
func TestIssue218_OptionalCycleRepeatFixtures(t *testing.T) {
	runSidecarFixtures(t, "optional-cycle-repeat",
		"testdata/hocon/optional-cycle-repeat", "testdata/expected/optional-cycle-repeat")
}

func TestIssue218_UnbreakableCycleThroughPriors(t *testing.T) {
	ok := []struct{ name, src, want string }{
		{"optional-twice", "a = ${?b}\nb = ${?a}\na = ${?b}\nb = ${?a}\n", `{}`},
		{"optional-thrice", "a = ${?b}\nb = ${?a}\na = ${?b}\nb = ${?a}\na = ${?b}\nb = ${?a}\n", `{}`},
		{"three-keys-twice", "a = ${?b}\nb = ${?c}\nc = ${?a}\na = ${?b}\nb = ${?c}\nc = ${?a}\n", `{}`},
		{"nested-path", "o.a = ${?o.b}\no.b = ${?o.a}\no.a = ${?o.b}\no.b = ${?o.a}\n", `{"o":{}}`},
		{"referenced-in-concat", "a = ${?b}\nb = ${?a}\na = ${?b}\nb = ${?a}\nc = ${?a}x\n", `{"c":"x"}`},
		{"later-value-breaks-it", "a = ${?b}\nb = ${?a}\na = ${?b}\nb = ${?a}\na = 1\n", `{"a":1,"b":1}`},
		// A legal re-entry: the second pass through o.y's prior sees `a` in
		// progress and stops there, so the guard must let it through.
		{"legal-reentry", "o = { y = ${?a} }\na = ${o.y}\no.x = 5\no.y += [3]\n", `{"a":[[3]],"o":{"x":5,"y":[[3]]}}`},
		{"self-ref-chain", "a = 1\na = ${a}x\na = ${a}y\nb = ${a}\n", `{"a":"1xy","b":"1xy"}`},
		// Also legal: the second pass through a's prior runs with x's prefix
		// prior in progress, so it stops on its own. This loads today, so the
		// guard must not refuse it (Lightbend errors here; that difference is
		// left to the resolver redesign).
		// Review counter-examples: re-entries that terminate because the field
		// path or the prefix-prior guards changed in between. They load today
		// and must keep their value.
		{"legal-reentry-field-path", "o = { y = ${?k} }\nk = [${?o.y}]\nk = ${?m}\nm = ${?k}\no = ${?k}\n", `{"k":[],"m":[],"o":[[]]}`},
		{"legal-reentry-field-path-2", "o = { y = ${?k} }\nk = [${?o.y}, 1]\nk = ${?m}\nm = ${?k}\no = ${?k}\n", `{"k":[1],"m":[1],"o":[[1],1]}`},
		{"legal-reentry-prefix-prior-2", "o = ${?k}z\no.y = ${?o.y}\nk = ${?o.x}\nk = ${k}z\n", `{"k":"z","o":{}}`},
		{"legal-reentry-prefix-prior", "x = ${a}\nx = ${a}\na = ${?x.y}\na = ${?a}ok\n", `{"a":"ok","x":"ok"}`},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := hocon.ParseString(tc.src)
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			got, err := cfg.RenderJSONForTest()
			if err != nil {
				t.Fatalf("RenderJSONForTest: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
	// The required form of an unbreakable cycle is an error, never a crash.
	for name, src := range map[string]string{
		"required-twice": "a = ${b}\nb = ${a}\na = ${b}\nb = ${a}\n",
		"mixed-twice":    "a = ${?b}\nb = ${a}\na = ${?b}\nb = ${a}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := hocon.ParseString(src); err == nil {
				t.Error("expected a resolve error, got success")
			}
		})
	}
}

// TestIssue218_LargeRingIsNotExponential: a ring of optional keys written
// twice. Cutting the cycle used to retry every key's saved prior, doubling the
// work at each level (n=16 took about a minute); it must finish promptly.
func TestIssue218_LargeRingIsNotExponential(t *testing.T) {
	const n = 40
	var b strings.Builder
	for pass := 0; pass < 2; pass++ {
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "k%d = ${?k%d}\n", i, (i+1)%n)
		}
	}
	done := make(chan string, 1)
	go func() {
		cfg, err := hocon.ParseString(b.String())
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		got, _ := cfg.RenderJSONForTest()
		done <- got
	}()
	select {
	case got := <-done:
		if got != `{}` {
			t.Errorf("got %s, want {}", got)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a 40-key ring written twice did not resolve within 20s")
	}
}

// TestIssue218_CycleThroughIncludeTerminates: the repetition can come from an
// included file (go crashed here even where rs did not).
func TestIssue218_CycleThroughIncludeTerminates(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inner.conf")
	if err := os.WriteFile(inner, []byte("a = ${?b}\nb = ${?a}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.conf")
	if err := os.WriteFile(main, []byte("a = ${?b}\nb = ${?a}\ninclude \"inner.conf\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(main)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got, _ := cfg.RenderJSONForTest(); got != `{}` {
		t.Errorf("got %s, want {}", got)
	}
}

// TestIssue218_ConcatCycleTerminates: a cycle written twice through string
// concatenation no longer crashes. The value still differs from Lightbend's
// ({"a":"yxyx","b":"yxy"}); getting it right is the resolver redesign
// (o3co/xx.hocon#67 / #125), so only termination is asserted here.
func TestIssue218_ConcatCycleTerminates(t *testing.T) {
	_, err := hocon.ParseString("a = ${?b}x\nb = ${?a}y\na = ${?b}x\nb = ${?a}y\n")
	if err != nil && !strings.Contains(err.Error(), "resolve") {
		t.Errorf("unexpected error: %v", err)
	}
}
