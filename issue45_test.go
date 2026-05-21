package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// Issue #45 (Copilot review on include relativization PR): in lenient mode
// — used by the include child resolver — unresolved optional substitutions
// (`${?path}`) were dropped immediately, so an included file referencing a
// parent-scope value never saw the parent's value supplied later.
//
// Fix: in lenient mode the placeholder is preserved so a subsequent
// ResolveTree pass (with priors supplied by the parent) can resolve it.
// Final / strict resolution still drops optional substitutions with no
// value.

func TestIssue45_OptionalSubstThroughIncludeResolvesAgainstParent(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte(`result = ${?parent_val}`), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	src := fmt.Sprintf(`parent_val = "hello"
include "%s"
`, slashChild)
	if err := os.WriteFile(mainFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetString("result"); got != "hello" {
		t.Errorf("result=%q, want \"hello\" (parent_val supplied by parent must resolve through include)", got)
	}
}

// TestIssue45_OptionalSubstStillDroppedInStrictMode pins the strict
// semantics: at the final (non-lenient) resolution pass, optional
// substitutions with no value still drop their field per the HOCON optional
// substitution rule. The fix changes only the lenient pass.
//
// Uses the deferred path + WithUseSystemEnvironment(false) so the process
// environment cannot accidentally satisfy the substitution and produce a
// false positive (Copilot review #1 on PR #111).
func TestIssue45_OptionalSubstStillDroppedInStrictMode(t *testing.T) {
	cfg, err := hocon.ParseStringWithOptions(
		`result = ${?some_var}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(
		hocon.DefaultResolveOptions().WithUseSystemEnvironment(false),
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	opt := resolved.GetStringOption("result")
	if opt.IsSome() {
		v, _ := opt.Get()
		t.Errorf("expected result absent, got %q", v)
	}
}

// TestIssue45_OptionalSubstThroughIncludeStillDropsIfMissing pins the
// missing-everywhere case: an optional substitution in an included file
// that is never supplied anywhere should still drop the field after the
// final resolution pass.
//
// Uses the deferred path + WithUseSystemEnvironment(false) so the process
// environment cannot accidentally satisfy the substitution (Copilot review
// #2 on PR #111).
func TestIssue45_OptionalSubstThroughIncludeStillDropsIfMissing(t *testing.T) {
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child.conf")
	if err := os.WriteFile(childFile, []byte(`result = ${?some_var}`), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashChild := strings.ReplaceAll(childFile, "\\", "/")
	src := fmt.Sprintf(`include "%s"
sentinel = 1
`, slashChild)
	if err := os.WriteFile(mainFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFileWithOptions(
		mainFile,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolved, err := cfg.Resolve(
		hocon.DefaultResolveOptions().WithUseSystemEnvironment(false),
	)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.GetStringOption("result").IsSome() {
		t.Error("expected result absent (optional unresolved in both child and parent)")
	}
	if got := resolved.GetInt64("sentinel"); got != 1 {
		t.Errorf("sentinel=%d, want 1 (include should not abort parent's other fields)", got)
	}
}
