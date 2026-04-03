package hocon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/o3co/go.hocon"
)

func mustParseCfg(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := hocon.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString error: %v", err)
	}
	return cfg
}

func TestConfig_GetString(t *testing.T) {
	cfg := mustParseCfg(t, `key = "hello"`)
	if got := cfg.GetString("key"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestConfig_GetString_Missing_Panics(t *testing.T) {
	cfg := mustParseCfg(t, `key = "hello"`)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing key")
		}
	}()
	cfg.GetString("missing")
}

func TestConfig_GetString_Null_Panics(t *testing.T) {
	cfg := mustParseCfg(t, `key = null`)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for null value")
		}
	}()
	cfg.GetString("key")
}

func TestConfig_GetInt64(t *testing.T) {
	cfg := mustParseCfg(t, `n = 42`)
	if got := cfg.GetInt64("n"); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestConfig_GetInt(t *testing.T) {
	cfg := mustParseCfg(t, `n = 99`)
	if got := cfg.GetInt("n"); got != 99 {
		t.Errorf("got %d, want 99", got)
	}
}

func TestConfig_GetFloat64(t *testing.T) {
	cfg := mustParseCfg(t, `f = 3.14`)
	if got := cfg.GetFloat64("f"); got != 3.14 {
		t.Errorf("got %v, want 3.14", got)
	}
}

func TestConfig_GetBool(t *testing.T) {
	cfg := mustParseCfg(t, `b = true`)
	if !cfg.GetBool("b") {
		t.Error("expected true")
	}
}

func TestConfig_GetDuration(t *testing.T) {
	tests := []struct {
		src  string
		want time.Duration
	}{
		{`d = "10ms"`, 10 * time.Millisecond},
		{`d = "2s"`, 2 * time.Second},
		{`d = "1h"`, time.Hour},
		{`d = "1d"`, 24 * time.Hour},
	}
	for _, tc := range tests {
		cfg := mustParseCfg(t, tc.src)
		got := cfg.GetDuration("d")
		if got != tc.want {
			t.Errorf("src=%q: got %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestConfig_GetBytes(t *testing.T) {
	tests := []struct {
		src  string
		want int64
	}{
		{`b = "100B"`, 100},
		{`b = "1KB"`, 1000},
		{`b = "1KiB"`, 1024},
		{`b = "1MB"`, 1_000_000},
		{`b = "1MiB"`, 1024 * 1024},
	}
	for _, tc := range tests {
		cfg := mustParseCfg(t, tc.src)
		got := cfg.GetBytes("b")
		if got != tc.want {
			t.Errorf("src=%q: got %d, want %d", tc.src, got, tc.want)
		}
	}
}

func TestConfig_Has(t *testing.T) {
	cfg := mustParseCfg(t, "a=1\nb=null")
	if !cfg.Has("a") {
		t.Error("expected Has(a)=true")
	}
	if !cfg.Has("b") {
		t.Error("expected Has(b)=true for null value")
	}
	if cfg.Has("missing") {
		t.Error("expected Has(missing)=false")
	}
}

func TestConfig_Keys(t *testing.T) {
	cfg := mustParseCfg(t, "a=1\nb=2\nc=3")
	keys := cfg.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %v", keys)
	}
	want := []string{"a", "b", "c"}
	for i, k := range want {
		if keys[i] != k {
			t.Errorf("keys[%d]=%q, want %q", i, keys[i], k)
		}
	}
}

func TestConfig_GetConfig(t *testing.T) {
	cfg := mustParseCfg(t, "server {\n  host=localhost\n  port=8080\n}")
	srv := cfg.GetConfig("server")
	if srv.GetString("host") != "localhost" {
		t.Error("expected localhost")
	}
}

func TestConfig_NestedPath(t *testing.T) {
	cfg := mustParseCfg(t, "a.b.c = 42")
	if cfg.GetInt64("a.b.c") != 42 {
		t.Error("expected 42 via nested path")
	}
}

func TestConfig_StringOption_Some(t *testing.T) {
	cfg := mustParseCfg(t, `key="val"`)
	opt := cfg.GetStringOption("key")
	if !opt.IsSome() {
		t.Error("expected Some")
	}
	v, _ := opt.Get()
	if v != "val" {
		t.Errorf("got %q", v)
	}
}

func TestConfig_StringOption_None_Missing(t *testing.T) {
	cfg := mustParseCfg(t, `key="val"`)
	if cfg.GetStringOption("missing").IsSome() {
		t.Error("expected None for missing")
	}
}

func TestConfig_StringOption_None_Null(t *testing.T) {
	cfg := mustParseCfg(t, `key=null`)
	if cfg.GetStringOption("key").IsSome() {
		t.Error("expected None for null")
	}
}

func TestConfig_GetStringSlice(t *testing.T) {
	cfg := mustParseCfg(t, `arr=["a","b","c"]`)
	s := cfg.GetStringSlice("arr")
	if len(s) != 3 || s[0] != "a" || s[2] != "c" {
		t.Errorf("unexpected: %v", s)
	}
}

func TestConfig_GetConfigSlice(t *testing.T) {
	cfg := mustParseCfg(t, `items=[{n=1},{n=2}]`)
	sl := cfg.GetConfigSlice("items")
	if len(sl) != 2 {
		t.Fatalf("expected 2, got %d", len(sl))
	}
	if sl[0].GetInt64("n") != 1 {
		t.Error("expected n=1")
	}
}

func TestConfig_EnvVarInt(t *testing.T) {
	t.Setenv("HOCON_TEST_PORT", "50052")
	cfg := mustParseCfg(t, "server {\n  port = 50051\n  port = ${?HOCON_TEST_PORT}\n}")
	if got := cfg.GetInt("server.port"); got != 50052 {
		t.Errorf("expected 50052, got %d", got)
	}
}

func TestConfig_EnvVarFloat(t *testing.T) {
	t.Setenv("HOCON_TEST_RATIO", "3.14")
	cfg := mustParseCfg(t, "ratio = 1.0\nratio = ${?HOCON_TEST_RATIO}")
	if got := cfg.GetFloat64("ratio"); got != 3.14 {
		t.Errorf("expected 3.14, got %f", got)
	}
}

func TestConfig_EnvVarBool(t *testing.T) {
	t.Setenv("HOCON_TEST_ENABLED", "true")
	cfg := mustParseCfg(t, "enabled = false\nenabled = ${?HOCON_TEST_ENABLED}")
	if got := cfg.GetBool("enabled"); !got {
		t.Errorf("expected true, got %v", got)
	}
}

func TestConfig_OptionalSubstitutionFallback(t *testing.T) {
	// Regression test: when ${?VAR} is unset, the prior value of the key must be kept.
	cfg := mustParseCfg(t, "server {\n  host = \"0.0.0.0\"\n  host = ${?HOST_UNSET_XYZ}\n}")
	got := cfg.GetString("server.host")
	if got != "0.0.0.0" {
		t.Errorf("expected \"0.0.0.0\", got %q", got)
	}
}

func TestConfig_OptionalSubstitutionFallback_SubstPrior(t *testing.T) {
	// Regression: prior value is itself a substitution (not a literal).
	// permissionVerifier.timeout should resolve via clients.default.timeout → 30s.
	src := `
clients {
  default {
    timeout = 30s
    timeout = ${?CLIENT_DEFAULT_TIMEOUT_UNSET_XYZ}
  }
  permissionVerifier {
    timeout = ${clients.default.timeout}
    timeout = ${?CLIENT_PERMISSION_VERIFIER_TIMEOUT_UNSET_XYZ}
  }
}`
	cfg := mustParseCfg(t, src)
	defaultTimeout := cfg.GetDuration("clients.default.timeout")
	if defaultTimeout.String() != "30s" {
		t.Errorf("clients.default.timeout: expected 30s, got %s", defaultTimeout)
	}
	got := cfg.GetDuration("clients.permissionVerifier.timeout")
	if got.String() != "30s" {
		t.Errorf("clients.permissionVerifier.timeout: expected 30s, got %s", got)
	}
}

func TestEmptyEnvVar(t *testing.T) {
	t.Setenv("HOCON_EMPTY", "")
	cfg, err := hocon.ParseString(`val = ${HOCON_EMPTY}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := cfg.GetString("val")
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestUnsetEnvVarOptional(t *testing.T) {
	const envKey = "HOCON_TEST_UNSET_VAR"
	os.Unsetenv(envKey)
	t.Cleanup(func() { os.Unsetenv(envKey) })
	cfg, err := hocon.ParseString(fmt.Sprintf(`val = ${?%s}`, envKey))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Has("val") {
		t.Error("expected val to not exist for unset optional env var")
	}
}

func TestEmptyEnvVarOptional(t *testing.T) {
	t.Setenv("HOCON_EMPTY_OPTIONAL", "")
	cfg, err := hocon.ParseString(`val = ${?HOCON_EMPTY_OPTIONAL}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Has("val") {
		t.Fatal("expected val to exist for empty-but-set optional env var")
	}
	got := cfg.GetString("val")
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestQuotedPathLookup(t *testing.T) {
	cfg, err := hocon.ParseString(`"a.b" = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Has(`"a.b"`) {
		t.Error("expected Has to return true for quoted key")
	}
	if got := cfg.GetInt64(`"a.b"`); got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestNestedQuotedPathLookup(t *testing.T) {
	cfg, err := hocon.ParseString(`server { "web.api" { port = 8080 } }`)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetInt64(`server."web.api".port`); got != 8080 {
		t.Errorf("got %d, want 8080", got)
	}
}

func TestEscapedQuoteInPath(t *testing.T) {
	// The lexer unescapes \" → " when parsing quoted keys.
	// splitPath must do the same when scanning quoted segments.
	cfg, err := hocon.ParseString(`"a\"b" = 42`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Has(`"a\"b"`) {
		t.Error(`expected Has to return true for key with escaped quote`)
	}
	if got := cfg.GetInt64(`"a\"b"`); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestConfig_WithFallback(t *testing.T) {
	base := mustParseCfg(t, "a=1\nb=2")
	over := mustParseCfg(t, "b=99\nc=3")
	merged := over.WithFallback(base)
	if merged.GetInt64("a") != 1 {
		t.Error("a should come from fallback")
	}
	if merged.GetInt64("b") != 99 {
		t.Error("b should be from receiver (over)")
	}
	if merged.GetInt64("c") != 3 {
		t.Error("c should be from receiver")
	}
}

// --- String concatenation ---

func TestConfig_StringConcat_AdjacentStringsPreservesSpace(t *testing.T) {
	// Whitespace between adjacent values is preserved in concatenation.
	cfg := mustParseCfg(t, `url = "http://" "example.com"`)
	if got := cfg.GetString("url"); got != "http:// example.com" {
		t.Errorf("got %q, want %q", got, "http:// example.com")
	}
}

func TestConfig_StringConcat_NoSpace(t *testing.T) {
	// Substitution immediately followed by quoted string (no space) concatenates without space.
	cfg := mustParseCfg(t, `
base = "http://example.com"
url  = ${base}"/path"
`)
	if got := cfg.GetString("url"); got != "http://example.com/path" {
		t.Errorf("got %q, want %q", got, "http://example.com/path")
	}
}

func TestConfig_StringConcat_UnquotedWords(t *testing.T) {
	// Adjacent unquoted words separated by space produce a string with that space.
	cfg := mustParseCfg(t, `msg = hello world`)
	if got := cfg.GetString("msg"); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestConfig_StringConcat_SubstitutionAndLiteral(t *testing.T) {
	cfg := mustParseCfg(t, `
base = "https://api.example.com"
url  = ${base}"/users"
`)
	if got := cfg.GetString("url"); got != "https://api.example.com/users" {
		t.Errorf("got %q, want %q", got, "https://api.example.com/users")
	}
}

func TestConfig_StringConcat_ArraySelfRef(t *testing.T) {
	// Self-referential array concatenation: a = ${a}[extra] (no space — space inserts string node)
	cfg := mustParseCfg(t, `
a = [1, 2]
a = ${a}[3, 4]
`)
	got := cfg.GetInt64Slice("a")
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("got %v, want [1 2 3 4]", got)
	}
}

// --- BOM handling ---

func TestConfig_BOM_ParseFile(t *testing.T) {
	path := filepath.Join("testdata", "hocon", "bom.conf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("bom.conf not found")
	}
	cfg, err := hocon.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := cfg.GetString("foo"); got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

func TestConfig_BOM_ParseString(t *testing.T) {
	// UTF-8 BOM followed by HOCON content
	src := "\xEF\xBB\xBFfoo = bar"
	cfg, err := hocon.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString with BOM: %v", err)
	}
	if got := cfg.GetString("foo"); got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}
