package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// TestIssue106_IncludeScalarOverridesParent verifies that when an include
// appears after a key already assigned in the parent, the included assignment
// overrides the parent — matching Lightbend's "as if inlined at include
// position" semantics.
//
// Reported by cgordon (go.hocon#106).
func TestIssue106_IncludeScalarOverridesParent(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte("a = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("a = 1\ninclude \"%s\"\n", slashChild)), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetInt64("a"); got != 2 {
		t.Errorf("a=%d, want 2 (include must override parent's earlier assignment)", got)
	}
}

// TestIssue106_ParentScalarOverridesInclude verifies the reverse direction:
// when a parent assignment appears AFTER an include, the parent's assignment
// wins. This is the simple "last write wins" rule and should already work,
// but we pin it as a regression test alongside the include-override fix.
func TestIssue106_ParentScalarOverridesInclude(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte("a = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 5\n", slashChild)), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetInt64("a"); got != 5 {
		t.Errorf("a=%d, want 5 (parent assignment after include must win)", got)
	}
}

// TestIssue106_SelfRefAppendThroughInclude verifies that an included file may
// reference the parent's value of a key via self-substitution (`${k}`) and
// append to it. Lightbend resolves this as if the include's body appeared
// inline; go.hocon previously failed with "unresolved self-referential
// substitution" because the include merge did not record the parent's value
// as a priorValue for the include's assignment.
//
// Reported by cgordon (go.hocon#106).
func TestIssue106_SelfRefAppendThroughInclude(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte(`steps = ${steps} [
  { name = child }
]`), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	parentSrc := fmt.Sprintf(`steps = [
  { name = base }
]

include "%s"
`, slashChild)
	if err := os.WriteFile(mainFile, []byte(parentSrc), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	steps := cfg.GetConfigSlice("steps")
	if got := len(steps); got != 2 {
		t.Fatalf("expected 2 steps (base + child), got %d", got)
	}
	if got := steps[0].GetString("name"); got != "base" {
		t.Errorf("steps[0].name=%q, want base", got)
	}
	if got := steps[1].GetString("name"); got != "child" {
		t.Errorf("steps[1].name=%q, want child", got)
	}
}

// TestIssue106_ControlSameFileAppend pins the existing same-file
// self-referential append behavior so the fix for the include path does not
// regress it.
func TestIssue106_ControlSameFileAppend(t *testing.T) {
	cfg, err := hocon.ParseString(`steps = [
  { name = base }
]

steps = ${steps} [
  { name = child }
]
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	steps := cfg.GetConfigSlice("steps")
	if got := len(steps); got != 2 {
		t.Fatalf("expected 2 steps, got %d", got)
	}
}

// TestIssue106_NestedIncludePriorDoesNotLeakTopLevel pins the
// multi-agent-review finding (Claude + Codex convergence): when a nested
// include overrides a key inside a wrapper object, the parent's prior must
// NOT leak into the resolver-wide priorValues map under the same leaf name.
// Otherwise an unrelated top-level self-referential substitution with the
// same leaf key would incorrectly find the nested prior. Mirrors the
// S13a13 TestS13a13_NestedPriorDoesNotCollideWithTopLevel scenario for the
// include path.
func TestIssue106_NestedIncludePriorDoesNotLeakTopLevel(t *testing.T) {
	dir := t.TempDir()
	leafFile := filepath.Join(dir, "leaf.conf")
	if err := os.WriteFile(leafFile, []byte("a = innerB\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "p.conf")
	slashLeaf := strings.ReplaceAll(leafFile, "\\", "/")
	src := fmt.Sprintf(`nested {
  a = innerA
  include "%s"
}
a = ${?a}suffix
`, slashLeaf)
	if err := os.WriteFile(mainFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("nested.a"); got != "innerB" {
		t.Errorf("nested.a=%q, want innerB (include override)", got)
	}
	if got := cfg.GetString("a"); got != "suffix" {
		t.Errorf("a=%q, want \"suffix\" (top-level ${?a} has no prior; nested include must not leak)", got)
	}
}

// TestIssue106_SequentialIncludesOverrideParent verifies that two consecutive
// includes each correctly override the parent's value. Each include's
// prior-capture must use the previous include's value (not the parent's
// original value) so the chain reads like inline assignments.
func TestIssue106_SequentialIncludesOverrideParent(t *testing.T) {
	dir := t.TempDir()
	c1 := filepath.Join(dir, "c1.conf")
	c2 := filepath.Join(dir, "c2.conf")
	if err := os.WriteFile(c1, []byte("a = 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c2, []byte("a = 3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashC1 := strings.ReplaceAll(c1, "\\", "/")
	slashC2 := strings.ReplaceAll(c2, "\\", "/")
	src := fmt.Sprintf("a = 1\ninclude \"%s\"\ninclude \"%s\"\n", slashC1, slashC2)
	if err := os.WriteFile(mainFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetInt64("a"); got != 3 {
		t.Errorf("a=%d, want 3 (last include wins)", got)
	}
}

// TestIssue106_LenientSelfRefDeferredPlaceholder pins the lenient-mode change
// to resolveSubst: a required self-referential substitution with no prior
// value used to error, but under lenient mode (AllowUnresolved=true) it now
// defers the placeholder so a subsequent ResolveTree pass (with a prior
// supplied externally) can complete it. Used by the include path; this test
// keeps the AllowUnresolved code path honest.
func TestIssue106_LenientSelfRefDeferredPlaceholder(t *testing.T) {
	// Parse without resolution, then resolve in lenient (AllowUnresolved) mode.
	// `a = ${a} "suffix"` is a required self-ref against itself; under lenient
	// mode we expect the placeholder to remain (no error). A subsequent merge
	// supplying a prior `a` should let it complete.
	cfg, err := hocon.ParseStringWithOptions(`a = ${a} "suffix"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(hocon.DefaultResolveOptions().WithAllowUnresolved(true))
	if err != nil {
		t.Fatalf("Resolve(AllowUnresolved=true): %v", err)
	}
	if resolved.IsResolved() {
		t.Error("expected IsResolved()=false because self-ref had no prior; resolver should defer the placeholder")
	}
}

// TestIssue106_NestedObjectsDeepMerge verifies that the include-merge fix
// does not break the "both-object collision deep-merges" rule. Parent and
// include each contribute disjoint sub-keys under the same parent key; the
// merge must union them.
func TestIssue106_NestedObjectsDeepMerge(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte(`server { port = 8080 }`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf(`server { host = "localhost" }
include "%s"
`, slashChild)), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("server.host"); got != "localhost" {
		t.Errorf("server.host=%q, want localhost", got)
	}
	if got := cfg.GetInt64("server.port"); got != 8080 {
		t.Errorf("server.port=%d, want 8080", got)
	}
}
