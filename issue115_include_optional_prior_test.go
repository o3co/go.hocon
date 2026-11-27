// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// xx.hocon#115 — an included root-level value keeps the including file's prior.
//
// S13.11 (HOCON.md L632): an undefined optional that an included file assigns
// to a root-level key leaves the including file's earlier value in place:
//
//	# main.conf                  # inner.conf
//	a = 1                        a = ${?UNSET}
//	include "inner.conf"         // → {"a":1}
//
// S14b.3 (L1000): an included root-level `a = ${b}` merges with the including
// file's earlier object, as it does when both lines are in one file.
//
// rs/ts/py dropped the including file's value; go already matches Lightbend
// 1.4.9 on every row, so this file only pins that behaviour. Stacks of three
// or more layers (`a = 1` / include with `a = ${?U}` and `a = ${?V}`) are
// o3co/xx.hocon#125 and not covered here.
//
// Fixtures: testdata/hocon/include-optional-prior/io01–io16 (with
// `io<NN>-…-inner.conf` include targets; io07 nests a second one). Expected:
// testdata/expected/include-optional-prior/<stem>-expected.json (all success).
// Until the xx.hocon group reaches main, the expected dir is absent from a
// `make testdata` fetch and the fixture test skips; the inline tests below
// pin the behaviour regardless. Later rows of the group (io17+, the #67-window
// acceptance rows) are not part of this pin and are left out of the run.

package hocon_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

const (
	ioConfDir     = "testdata/hocon/include-optional-prior"
	ioExpectedDir = "testdata/expected/include-optional-prior"
)

// ioLastPinned is the last fixture row this file pins (io01–io16).
const ioLastPinned = 16

// unsetGH115Env makes sure the fixtures' undefined optionals are undefined:
// GH115_UNSET itself, and the names a relativized or dotted optional also
// tries in the environment (`c.GH115_UNSET` for io09, `gh115_missing.b` for
// io08).
func unsetGH115Env(t *testing.T) {
	t.Helper()
	for _, n := range []string{"GH115_UNSET", "c.GH115_UNSET", "gh115_missing.b"} {
		t.Setenv(n, "")
		if err := os.Unsetenv(n); err != nil {
			t.Fatalf("Unsetenv(%s): %v", n, err)
		}
	}
}

// TestS13_11_IncludeOptionalPriorFixtures drives io01–io16. It copies those
// rows (with their include targets and sidecars) to a temp dir first, so rows
// a later `make testdata` brings in are not run against this pin.
func TestS13_11_IncludeOptionalPriorFixtures(t *testing.T) {
	unsetGH115Env(t)
	if _, err := os.Stat(ioExpectedDir); os.IsNotExist(err) {
		t.Skipf("include-optional-prior expected dir missing at %s; run `make testdata`", ioExpectedDir)
	}
	confDir, expectedDir := t.TempDir(), t.TempDir()
	copyPinnedIORows(t, ioConfDir, confDir)
	copyPinnedIORows(t, ioExpectedDir, expectedDir)
	runSidecarFixtures(t, "include-optional-prior", confDir, expectedDir)
}

// copyPinnedIORows copies every file of rows io01..ioLastPinned from src to dst.
func copyPinnedIORows(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", src, err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "io") || len(name) < 4 {
			continue
		}
		row, err := strconv.Atoi(name[2:4])
		if err != nil || row < 1 || row > ioLastPinned {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
}

// TestIssue115_IncludedValueKeepsPrior carries each fixture inline with its
// Lightbend 1.4.9 value: files maps a name to its content, "main.conf" is the
// one parsed. Every other file also gets a `gh115_loaded_<file> = true`
// marker, asserted (under scope, where the include sits inside an object) and
// then dropped before the comparison, so a skipped include cannot pass by
// leaving main.conf's value alone.
func TestIssue115_IncludedValueKeepsPrior(t *testing.T) {
	unsetGH115Env(t)
	cases := []struct {
		name  string
		files map[string]string
		scope string // object the includes sit in ("" = root)
		want  string
	}{
		// S13.11 at a root-level include: the gap rs/ts/py had.
		{"fuzz-seed-25", map[string]string{
			"main.conf":  "bar.a = 82.61  null\ninclude \"inner.conf\"\n",
			"inner.conf": "bar = ${?GH115_UNSET}\n",
		}, "", `{"bar":{"a":"82.61  null"}}`},
		{"prior-object-and-reference", map[string]string{
			"main.conf":  "a { x = 1 }\ninclude \"inner.conf\"\nb = ${a}\n",
			"inner.conf": "a = ${?GH115_UNSET}\n",
		}, "", `{"a":{"x":1},"b":{"x":1}}`},
		{"prior-scalar", map[string]string{
			"main.conf":  "a = 1\ninclude \"inner.conf\"\n",
			"inner.conf": "a = ${?GH115_UNSET}\n",
		}, "", `{"a":1}`},
		{"prior-array", map[string]string{
			"main.conf":  "a = [1]\ninclude \"inner.conf\"\n",
			"inner.conf": "a = ${?GH115_UNSET}\n",
		}, "", `{"a":[1]}`},
		{"both-optionals-undefined", map[string]string{
			"main.conf":  "a = 1\ninclude \"inner.conf\"\n",
			"inner.conf": "a = ${?GH115_UNSET}${?GH115_UNSET}\n",
		}, "", `{"a":1}`},
		{"object-concat-keeps-prior", map[string]string{
			"main.conf":  "a { x = 1 }\ninclude \"inner.conf\"\n",
			"inner.conf": "a = ${?GH115_UNSET} { y = 2 }\n",
		}, "", `{"a":{"x":1,"y":2}}`},
		{"nested-include", map[string]string{
			"main.conf":        "a = 1\ninclude \"inner.conf\"\n",
			"inner.conf":       "include \"inner-inner.conf\"\n",
			"inner-inner.conf": "a = ${?GH115_UNSET}\ndeep = true\n",
		}, "", `{"a":1,"deep":true}`},
		{"optional-path-reference", map[string]string{
			"main.conf":  "a = 1\ninclude \"inner.conf\"\n",
			"inner.conf": "a = ${?gh115_missing.b}\n",
		}, "", `{"a":1}`},
		// Shapes already right in all four impls.
		{"include-under-key", map[string]string{
			"main.conf":  "c { a = 1 }\nc { include \"inner.conf\" }\n",
			"inner.conf": "a = ${?GH115_UNSET}\n",
		}, "c", `{"c":{"a":1}}`},
		{"include-under-key-nested", map[string]string{
			"main.conf":  "a { x = 1 }\ninclude \"inner.conf\"\n",
			"inner.conf": "a { x = ${?GH115_UNSET} }\n",
		}, "", `{"a":{"x":1}}`},
		{"optional-after-included-value", map[string]string{
			"main.conf":  "include \"inner.conf\"\na = ${?GH115_UNSET}\n",
			"inner.conf": "a = 1\n",
		}, "", `{"a":1}`},
		{"optional-inside-included-file", map[string]string{
			"main.conf":  "include \"inner.conf\"\n",
			"inner.conf": "a = 1\na = ${?GH115_UNSET}\n",
		}, "", `{"a":1}`},
		{"dotted-key", map[string]string{
			"main.conf":  "c { a = 1 }\ninclude \"inner.conf\"\n",
			"inner.conf": "c.a = ${?GH115_UNSET}\n",
		}, "", `{"c":{"a":1}}`},
		{"self-ref-across-include", map[string]string{
			"main.conf":  "a = 1\ninclude \"inner.conf\"\n",
			"inner.conf": "a = ${?a} 2\n",
		}, "", `{"a":"1 2"}`},
		{"no-include", map[string]string{
			"main.conf": "a = 1\na = ${?GH115_UNSET}\n",
		}, "", `{"a":1}`},
		// S14b.3: the same include-merge rule without an optional.
		{"object-subst-merges-prior", map[string]string{
			"main.conf":  "a { x = 1 }\ninclude \"inner.conf\"\nb { y = 1 }\n",
			"inner.conf": "a = ${b}\n",
		}, "", `{"a":{"x":1,"y":1},"b":{"y":1}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var markers []string
			for name, content := range tc.files {
				if name != "main.conf" {
					m := "gh115_loaded_" + strings.NewReplacer(".", "_", "-", "_").Replace(name)
					content += m + " = true\n"
					markers = append(markers, m)
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := hocon.ParseFile(filepath.Join(dir, "main.conf"))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			got := make(map[string]any)
			if err := cfg.Unmarshal(&got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			scoped := got
			if tc.scope != "" {
				scoped, _ = got[tc.scope].(map[string]any)
			}
			for _, m := range markers {
				if scoped[m] != true {
					t.Fatalf("marker %s missing under %q: the included file was not loaded (got %v)", m, tc.scope, got)
				}
				delete(scoped, m)
			}
			requireIOJSON(t, got, tc.want)
		})
	}
}

func requireIOJSON(t *testing.T, got map[string]any, wantJSON string) {
	t.Helper()
	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("bad want JSON %q: %v", wantJSON, err)
	}
	if !jsonEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		t.Errorf("got %s, want %s", gotJSON, wantJSON)
	}
}
