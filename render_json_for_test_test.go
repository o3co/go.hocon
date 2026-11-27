package hocon_test

import (
	"encoding/json"
	"strings"
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
// JSON. `1.` is a number (the Lightbend number-run rule), so the renderer
// completes it to the JSON number `1.0` rather than emit `1.` or a string.
func TestRenderJSONForTest_InvalidNumberLexemeQuoted(t *testing.T) {
	cfg, err := hocon.ParseString("a = 1.")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	const want = `{"a":1.0}`
	if got != want {
		t.Fatalf("RenderJSONForTest() = %q, want %q", got, want)
	}
}

// go.hocon#215: strings and keys render as RFC 8259 JSON, not Go syntax.
// strconv.Quote wrote a control character as `\x01` (invalid JSON) and a
// non-printable astral rune such as U+E0001 as `\U000e0001` (also invalid).
// Every control character must come out as a JSON escape, everything else
// verbatim, and the result must decode back to the same strings.
func TestRenderJSONForTest_StringsAreJSONEscaped(t *testing.T) {
	src := `a = "x\u0001y"
b = "q\"b\\s/"
c = "\b\f\n\r\t\u0000\u001f"
d = "héllo 日本 😀 ` + "\U000E0001" + `"
"k\u0002ey" = 1
e = "\u007f\u0085\u2028\u2029"
`
	cfg, err := hocon.ParseString(src)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	want := `{"a":"x\u0001y","b":"q\"b\\s/","c":"\b\f\n\r\t\u0000\u001f",` +
		`"d":"héllo 日本 😀 ` + "\U000E0001" + `",` +
		`"e":"` + "\u007f\u0085\u2028\u2029" + `","k\u0002ey":1}`
	if got != want {
		t.Fatalf("RenderJSONForTest()\n got %q\nwant %q", got, want)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, got)
	}
	for key, wantVal := range map[string]any{
		"a": "x\x01y", "b": `q"b\s/`, "c": "\b\f\n\r\t\x00\x1f",
		"d": "héllo 日本 😀 \U000E0001", "e": "\u007f\u0085\u2028\u2029", "k\x02ey": float64(1),
	} {
		if decoded[key] != wantVal {
			t.Errorf("decoded[%q] = %q, want %q", key, decoded[key], wantVal)
		}
	}
}

// go.hocon#215: a string that is not valid UTF-8 cannot be written as JSON
// without changing it, so rendering fails instead of writing U+FFFD. The
// parser rejects such bytes in source (S1.1), but environment substitution
// and FromMap can still carry them. FromMap is used here rather than the
// environment: on Windows the environment is UTF-16, so an invalid byte is
// already U+FFFD by the time it is read.
func TestRenderJSONForTest_InvalidUTF8IsAnError(t *testing.T) {
	for name, values := range map[string]map[string]any{
		"value": {"v": "secret-a\xffb"},
		// With U+FFFD substitution these two distinct keys would render as the
		// same JSON member, and a JSON reader would silently keep only one.
		"keys that U+FFFD would merge": {"\xff": 1, "\uFFFD": 2},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := hocon.FromMap(values, "")
			if err != nil {
				t.Fatalf("FromMap: %v", err)
			}
			got, err := cfg.RenderJSONForTest()
			if err == nil {
				t.Fatalf("RenderJSONForTest() = %q, want an error", got)
			}
			if !strings.Contains(err.Error(), "not valid UTF-8") {
				t.Fatalf("RenderJSONForTest() error = %q, want the UTF-8 error", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error %q carries the value; it must not", err)
			}
		})
	}
}
