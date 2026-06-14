package hocon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/o3co/go.hocon"
)

// Regression tests for go.hocon#147: a self-reference nested below an
// outer-object-merge boundary must resolve against the pre-merge value, not
// error with "unresolved self-referential substitution". This was a live defect
// up to v1.6.1 (a nested self-ref below the merge boundary had no lookback
// target) and was fixed on develop by #135 (defer substitution resolution across
// includes). These cross-impl shapes — found via an audit against ts.hocon and
// rs.hocon, which both resolve them (rs.hocon#135 / #136) — pin the resolved
// behavior against regression.
func TestIssue147NestedSelfRefBelowMerge(t *testing.T) {
	cases := []struct{ name, in, path, want string }{
		{
			name: "single-file outer-object merge",
			in: `a = { b = { child = { f1 = "original" } } }
a = { b = { child = { f1 = ${a.b.child.f1}" appended" } } }`,
			path: "a.b.child.f1",
			want: "original appended",
		},
		{
			name: "dotted-key outer merge",
			in: `a.b.child.f1 = "original"
a.b.child = { f1 = ${a.b.child.f1}" appended", g = 1 }`,
			path: "a.b.child.f1",
			want: "original appended",
		},
		{
			// Chained (length 3): each step's pre-merge value already holds a
			// self-ref, so the recorded prior must be folded self-ref-free —
			// recording it unfolded would recurse forever at resolve time.
			name: "chained length-3 nested self-ref",
			in: `a = { child = { f1 = "base" } }
a = { child = { f1 = ${a.child.f1}"-x" } }
a = { child = { f1 = ${a.child.f1}"-y" } }`,
			path: "a.child.f1",
			want: "base-x-y",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			cfg, err := hocon.ParseString(c.in)
			if err != nil {
				t.Fatalf("unexpected resolve error: %v", err)
			}
			if got := cfg.GetString(c.path); got != c.want {
				t.Errorf("%s = %q, want %q", c.path, got, c.want)
			}
		})
	}
}

// Same defect across an include-merge boundary (the rs.hocon#135 shape class).
func TestIssue147NestedSelfRefThroughInclude(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("file1.conf", `a.b.child.f1 = "original"`+"\n")
	write("file2.conf", `a.b.child = { f1 = ${a.b.child.f1}" appended", extra = 1 }`+"\n")
	write("main.conf",
		`include "`+filepath.ToSlash(filepath.Join(dir, "file1.conf"))+`"`+"\n"+
			`include "`+filepath.ToSlash(filepath.Join(dir, "file2.conf"))+`"`+"\n")

	cfg, err := hocon.ParseFile(filepath.Join(dir, "main.conf"))
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if got := cfg.GetString("a.b.child.f1"); got != "original appended" {
		t.Errorf("a.b.child.f1 = %q, want %q", got, "original appended")
	}
}

// Same shape at the extensionless-include merge site, where `include "shared"`
// probes and merges multiple files (shared.json + shared.conf) through a
// distinct code path. Pins that this variant resolves too.
func TestIssue147NestedSelfRefThroughExtensionlessInclude(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("shared.json", `{ "a": { "b": { "child": { "f1": "original" } } } }`+"\n")
	write("shared.conf", `a.b.child = { f1 = ${a.b.child.f1}" appended" }`+"\n")
	write("main.conf", `include "`+filepath.ToSlash(filepath.Join(dir, "shared"))+`"`+"\n")

	cfg, err := hocon.ParseFile(filepath.Join(dir, "main.conf"))
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	if got := cfg.GetString("a.b.child.f1"); got != "original appended" {
		t.Errorf("a.b.child.f1 = %q, want %q", got, "original appended")
	}
}
