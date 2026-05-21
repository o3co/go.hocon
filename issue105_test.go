package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
)

// Issue #105 (cgordon): empty or comment-only included files should
// contribute an empty config instead of erroring. The narrower scope —
// include path only — is implemented; top-level empty parses
// (`ParseString("")`) remain invalid per S3.1 (HOCON.md L130).

func TestIssue105_EmptyIncludeFile(t *testing.T) {
	dir := t.TempDir()
	emptyFile := filepath.Join(dir, "empty.conf")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashEmpty := strings.ReplaceAll(emptyFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashEmpty)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v (expected empty include to be no-op)", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
}

func TestIssue105_CommentOnlyIncludeFile_Hash(t *testing.T) {
	dir := t.TempDir()
	commentFile := filepath.Join(dir, "comments.conf")
	if err := os.WriteFile(commentFile, []byte("# only a comment\n# another\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashC := strings.ReplaceAll(commentFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashC)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v (expected hash-comment-only include to be no-op)", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
}

func TestIssue105_CommentOnlyIncludeFile_DoubleSlash(t *testing.T) {
	dir := t.TempDir()
	commentFile := filepath.Join(dir, "comments.conf")
	if err := os.WriteFile(commentFile, []byte("// only a comment\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashC := strings.ReplaceAll(commentFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashC)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v (expected //-comment-only include to be no-op)", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
}

func TestIssue105_WhitespaceOnlyIncludeFile(t *testing.T) {
	dir := t.TempDir()
	wsFile := filepath.Join(dir, "ws.conf")
	if err := os.WriteFile(wsFile, []byte("   \n\t\n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashWs := strings.ReplaceAll(wsFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashWs)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v (expected whitespace-only include to be no-op)", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
}

// TestIssue105_UnicodeWhitespaceOnlyIncludeFile pins the multi-byte
// whitespace path. HOCON's whitespace set (per HOCON.md §Whitespace) covers
// NBSP, all Unicode Zs members, U+2028 (line sep), U+2029 (para sep), BOM,
// etc. An included file containing only these characters must also be
// treated as empty by the carve-out.
func TestIssue105_UnicodeWhitespaceOnlyIncludeFile(t *testing.T) {
	dir := t.TempDir()
	uwsFile := filepath.Join(dir, "uws.conf")
	// Build content from Go \u escapes to avoid editor-encoding surprises:
	// NBSP (U+00A0), en-quad (U+2000), line separator (U+2028), then LF.
	content := []byte("   \n")
	if err := os.WriteFile(uwsFile, content, 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashUws := strings.ReplaceAll(uwsFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashUws)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v (expected Unicode-whitespace-only include to be no-op)", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
}

func TestIssue105_BOMOnlyIncludeFile(t *testing.T) {
	dir := t.TempDir()
	bomFile := filepath.Join(dir, "bom.conf")
	// UTF-8 BOM followed by nothing semantic.
	if err := os.WriteFile(bomFile, []byte{0xEF, 0xBB, 0xBF, '\n'}, 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashB := strings.ReplaceAll(bomFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashB)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v (expected BOM-only include to be no-op)", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
}

// TestIssue105_BlockCommentInIncludeIsRejected pins the narrow scope: HOCON
// recognises only `#` and `//` comments. A `/* ... */` block-comment-only
// include is NOT silently treated as empty; it falls through to the parser
// which reports the syntax error. This guards against the carve-out
// becoming a backdoor for masking malformed include files.
func TestIssue105_BlockCommentInIncludeIsRejected(t *testing.T) {
	dir := t.TempDir()
	bcFile := filepath.Join(dir, "block.conf")
	if err := os.WriteFile(bcFile, []byte("/* multi\nline\ncomment */\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashBc := strings.ReplaceAll(bcFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashBc)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := hocon.ParseFile(mainFile); err == nil {
		t.Error("expected block-comment-only include to error (HOCON only supports # and //)")
	}
}

// TestIssue105_TopLevelEmptyStillRejected pins the narrower scope: only
// the include path treats empty/comment-only as no-op. ParseString of an
// empty top-level document still errors per S3.1 (HOCON.md L130).
func TestIssue105_TopLevelEmptyStillRejected(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"whitespace", "   \n  "},
		{"hash-comment", "# only comment\n"},
		{"slash-comment", "// only comment\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := hocon.ParseString(tc.src); err == nil {
				t.Errorf("ParseString(%q) succeeded; spec S3.1 requires rejection", tc.src)
			}
		})
	}
}

// TestIssue105_NonEmptyIncludeStillParses pins the negative direction: a
// non-empty include that has actual content must still be parsed normally
// (regression guard for the emptiness probe).
func TestIssue105_NonEmptyIncludeStillParses(t *testing.T) {
	dir := t.TempDir()
	cFile := filepath.Join(dir, "c.conf")
	if err := os.WriteFile(cFile, []byte("# leading comment\nb = 2\n# trailing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "parent.conf")
	slashC := strings.ReplaceAll(cFile, "\\", "/")
	if err := os.WriteFile(mainFile, []byte(fmt.Sprintf("include \"%s\"\na = 1\n", slashC)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := hocon.ParseFile(mainFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a=%d, want 1", got)
	}
	if got := cfg.GetInt64("b"); got != 2 {
		t.Errorf("b=%d, want 2 (non-empty include must still parse)", got)
	}
}
