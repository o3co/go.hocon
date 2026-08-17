// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/o3co/go.hocon"
)

// xx.hocon#55 / E17: a substitution that originates in an included file is
// relativized, so `${a.b}` inside a file mounted at `foo` carries prefixLen=1
// and has two candidate env-var names — the full base `foo.a.b` and the bare
// base `a.b`.
//
// Lightbend 1.4.6 consults only the bare base. All four o3co implementations
// consult both; that is the accepted divergence recorded as E17. What E17 makes
// normative is the ORDER: full base first, then bare.
//
// Before this fix go.hocon was the odd one out for the scalar form only — the
// bare lookup lived inside the S14c.2 original-path block and therefore ran
// before the full-base lookup at the bottom of resolveSubst. The list form
// (`${a.b[]}`, resolveEnvList) already tried full first, so the two forms
// disagreed with each other as well as with ts/rs/py.
//
// These names cannot be set from a shell (POSIX env names are
// [A-Za-z_][A-Za-z0-9_]*), which is why the divergence is not user-surfaced and
// why the regression has to be pinned through os.Setenv rather than a fixture.

// writeEnvBaseFixture writes the two-file include tree the E17 cases share and
// returns the path of the entry point. `subst` is the child's value expression.
func writeEnvBaseFixture(t *testing.T, subst string) string {
	t.Helper()
	dir := t.TempDir()
	child := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(child, []byte("v = "+subst+"\n"), 0o600); err != nil {
		t.Fatalf("write child.conf: %v", err)
	}
	main := filepath.Join(dir, "main.conf")
	if err := os.WriteFile(main, []byte("foo {\n  include \"child.conf\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write main.conf: %v", err)
	}
	return main
}

func TestIssue55EnvBaseOrderScalar(t *testing.T) {
	const fullName, bareName = "foo.a.b", "a.b"

	tests := []struct {
		name string
		full string // "" = leave unset
		bare string
		want string
	}{
		// The load-bearing case: both bases resolvable, full must win. Pre-fix
		// go.hocon returned "BARE" here while ts/rs/py returned "FULL".
		{name: "both set — full base wins", full: "FULL", bare: "BARE", want: "FULL"},
		// Guards against "fixed" by simply dropping the bare lookup: with only
		// the bare base set it must still resolve (this is the candidate
		// Lightbend uses, so removing it would diverge further, not less).
		{name: "bare base only", full: "", bare: "BARE", want: "BARE"},
		// The divergence E17 accepts: Lightbend would leave this unresolved.
		{name: "full base only", full: "FULL", bare: "", want: "FULL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for name, val := range map[string]string{fullName: tc.full, bareName: tc.bare} {
				if val == "" {
					unsetEnvForTest(t, name)
					continue
				}
				t.Setenv(name, val)
			}

			cfg, err := hocon.ParseFile(writeEnvBaseFixture(t, "${a.b}"))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if got := cfg.GetString("foo.v"); got != tc.want {
				t.Errorf("foo.v = %q, want %q", got, tc.want)
			}
		})
	}
}

// The list form was already full-first; pin it so the two forms cannot drift
// apart again from the other side.
func TestIssue55EnvBaseOrderList(t *testing.T) {
	const fullName, bareName = "foo.a.b_0", "a.b_0"

	t.Setenv(fullName, "FULL0")
	t.Setenv(bareName, "BARE0")

	cfg, err := hocon.ParseFile(writeEnvBaseFixture(t, "${a.b[]}"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	got := cfg.GetStringSlice("foo.v")
	if len(got) != 1 || got[0] != "FULL0" {
		t.Errorf("foo.v = %v, want [FULL0]", got)
	}
}
