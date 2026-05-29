package hocon_test

import (
	"testing"

	hocon "github.com/o3co/go.hocon"
)

func TestRenderJSONForTest_Canonical(t *testing.T) {
	cfg, err := hocon.ParseString("a = 1\nb = \"x\"\nc { d = true }\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	const want = `{"a":1,"b":"x","c":{"d":true}}`
	if got != want {
		t.Fatalf("RenderJSONForTest() = %q, want %q", got, want)
	}
}

// xx.hocon#50: leading-zero numeric VALUE literals must render as valid,
// canonical JSON numbers matching Lightbend/rs.hocon ("023"->23, "08.53"->8.53,
// "-023"->-23). Previously go emitted the raw lexeme verbatim ("023"), which is
// not valid JSON and diverged from rs/Lightbend. This is render-only: GetString
// still returns the preserved lexeme (S10.11) — see
// TestE8_ValueStartLeadingZero_PreservesLexeme.
func TestRenderJSONForTest_LeadingZeroNumberCanonicalized(t *testing.T) {
	cfg, err := hocon.ParseString("a = 01\nb = 023\nc = 08.53\nd = -023\ne = 007\nf = 00\ng = 000.5\nh = 0\ni = 0.5\nj = 1.0\nk = 100\nl = -08.53\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	const want = `{"a":1,"b":23,"c":8.53,"d":-23,"e":7,"f":0,"g":0.5,"h":0,"i":0.5,"j":1.0,"k":100,"l":-8.53}`
	if got != want {
		t.Fatalf("RenderJSONForTest() = %q, want %q", got, want)
	}
}

// xx.hocon#50 is intentionally scoped to leading zeros. The broader
// numeric-canonical family (exponent, trailing-zero, negative-zero) still
// renders verbatim and diverges from Lightbend/rs (which emit 1000.0 / 1.5 / 0).
// These outputs are valid JSON, just not yet canonical. Pinned so the follow-up
// full-canonicalization work (xx.hocon#53) must update this test deliberately
// rather than changing the rendering silently.
func TestRenderJSONForTest_DeferredNumericFamilyUnchanged(t *testing.T) {
	cfg, err := hocon.ParseString("a = 1e3\nb = 1.50\nc = -0\nd = 0e5")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	const want = `{"a":1e3,"b":1.50,"c":-0,"d":0e5}`
	if got != want {
		t.Fatalf("RenderJSONForTest() = %q, want %q (deferred family — see xx.hocon#53)", got, want)
	}
}

// xx.hocon#50 review (Copilot, ts #144): a numeric lexeme that is not a valid
// JSON number (e.g. `1.`, trailing dot) must be quoted, never emitted as invalid
// JSON. go classifies `1.` as a string and ts as a number, but both render the
// same valid `"1."` — the renderer never emits a malformed JSON number.
func TestRenderJSONForTest_InvalidNumberLexemeQuoted(t *testing.T) {
	cfg, err := hocon.ParseString("a = 1.")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	const want = `{"a":"1."}`
	if got != want {
		t.Fatalf("RenderJSONForTest() = %q, want %q", got, want)
	}
}
