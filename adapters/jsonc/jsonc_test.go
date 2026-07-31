// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package jsonc_test

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/jsonc"
)

func parse(t *testing.T, src string) *hocon.Config {
	t.Helper()
	cfg, err := jsonc.Parse([]byte(src), "test.jsonc")
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return cfg
}

func TestCommentsAndTrailingCommas(t *testing.T) {
	cfg := parse(t, `{
	  // a line comment
	  "a": 1, /* a block
	              comment spanning lines */
	  "b": [1, 2, ],
	  "c": { "d": true, },
	}`)

	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
	if got := cfg.GetInt64Slice("b"); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("b = %#v, want [1 2]", got)
	}
	if !cfg.GetBool("c.d") {
		t.Error("c.d = false, want true")
	}
}

// Comment markers inside string literals are data, not syntax.
func TestCommentMarkersInsideStringsSurvive(t *testing.T) {
	cfg := parse(t, `{
	  "url": "https://example.com/a//b",
	  "block": "a /* not a comment */ b",
	  "comma": "trailing, inside"
	}`)

	for path, want := range map[string]string{
		"url":   "https://example.com/a//b",
		"block": "a /* not a comment */ b",
		"comma": "trailing, inside",
	} {
		if got := cfg.GetString(path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestEscapedQuoteDoesNotEndString(t *testing.T) {
	cfg := parse(t, `{"a": "he said \"//\" loudly", "b": 1}`)
	if got := cfg.GetString("a"); got != `he said "//" loudly` {
		t.Errorf("a = %q", got)
	}
	if got := cfg.GetInt64("b"); got != 1 {
		t.Errorf("b = %d, want 1", got)
	}
}

// F0.5: an integer stays an integer instead of round-tripping through float64.
func TestLargeIntegerKeepsPrecision(t *testing.T) {
	cfg := parse(t, `{"big": 9007199254740993}`)
	if got := cfg.GetInt64("big"); got != 9007199254740993 {
		t.Errorf("big = %d, want 9007199254740993 (float64 would give ...92)", got)
	}
}

func TestIntegerOverflowIsError(t *testing.T) {
	_, err := jsonc.Parse([]byte(`{"big": 99999999999999999999}`), "test.jsonc")
	if err == nil {
		t.Fatal("out-of-range integer accepted, want error")
	}
	if !strings.Contains(err.Error(), "F0.5") {
		t.Errorf("error %q does not cite the spec item F0.5", err)
	}
}

func TestFloatsAndScientificNotation(t *testing.T) {
	cfg := parse(t, `{"ratio": 0.5, "sci": 1e3}`)
	if got := cfg.GetString("ratio"); got != "0.5" {
		t.Errorf("ratio rendered as %q, want 0.5", got)
	}
	if got := cfg.GetInt64("sci"); got != 1000 {
		t.Errorf("sci = %d, want 1000", got)
	}
}

// F0.3: a config root has to be an object.
func TestArrayRootRejected(t *testing.T) {
	_, err := jsonc.Parse([]byte(`[1, 2]`), "test.jsonc")
	if err == nil {
		t.Fatal("array root accepted, want error")
	}
	if !strings.Contains(err.Error(), "F0.3") {
		t.Errorf("error %q does not cite the spec item F0.3", err)
	}
}

func TestSyntaxErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unterminated block comment": `{"a": 1 /* oops`,
		"unterminated string":        `{"a": "oops}`,
		"trailing data":              `{"a": 1} {"b": 2}`,
		"not json at all":            `nonsense`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := jsonc.Parse([]byte(src), "test.jsonc"); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
		})
	}
}

// F3.2: trailing content after the top-level value is an error via a strict
// EOF check. A one-token peek (Decoder.More) reports false on a closing
// bracket, so stray closers used to slip through.
func TestTrailingGarbageRejected(t *testing.T) {
	for name, src := range map[string]string{
		"stray closing brace":    `{"a":1} }`,
		"stray closing bracket":  `{"a":1} ]`,
		"pile of closers":        `{"a":1} }}]]`,
		"closer then second doc": `{"a":1} } {"b":2}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := jsonc.Parse([]byte(src), "test.jsonc")
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
			if !strings.Contains(err.Error(), "after the top-level value") {
				t.Errorf("error %q does not name the trailing data", err)
			}
		})
	}
}

// The strict EOF check runs on the stripped text: whitespace and comments
// after the top-level value are exactly what JSONC allows there.
func TestTrailingWhitespaceAndCommentsAccepted(t *testing.T) {
	for name, src := range map[string]string{
		"trailing newlines":      "{\"a\": 1}\n\n",
		"trailing spaces":        `{"a": 1}   `,
		"trailing line comment":  "{\"a\": 1} // done",
		"trailing block comment": "{\"a\": 1}\n/* the end */\n",
		"comment then newline":   "{\"a\": 1} /* fin */ // bye\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := parse(t, src)
			if got := cfg.GetInt64("a"); got != 1 {
				t.Errorf("a = %d, want 1", got)
			}
		})
	}
}

// F3.2: a comment is replaced by whitespace, never the empty string, so it
// always separates tokens. 1/*x*/2 must stay two tokens and fail the decode
// rather than silently merging into 12.
func TestCommentSeparatesTokens(t *testing.T) {
	for name, src := range map[string]string{
		"block comment between digits":         `{"a": 1/*x*/2}`,
		"block comment between array elements": `{"a": [1/*x*/2]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := jsonc.Parse([]byte(src), "test.jsonc"); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", src)
			}
		})
	}
}

// F3.2: a // comment ends at any line break, including a lone CR. Old Mac
// line endings still turn up in checked-in config, and treating CR as
// ordinary text swallows the rest of the file.
func TestLineCommentEndsAtCarriageReturn(t *testing.T) {
	for name, src := range map[string]string{
		"lone CR": "{\"a\":1, //c\r\"b\":2}",
		"CRLF":    "{\"a\":1, //c\r\n\"b\":2}",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := parse(t, src)
			if got := cfg.GetInt64("a"); got != 1 {
				t.Errorf("a = %d, want 1", got)
			}
			if got := cfg.GetInt64("b"); got != 2 {
				t.Errorf("b = %d, want 2 — the comment swallowed the next line", got)
			}
		})
	}
}

// F0.9: a leading BOM is stripped rather than failing the decode.
func TestLeadingBOMStripped(t *testing.T) {
	cfg := parse(t, "\ufeff{\"a\": 1}")
	if got := cfg.GetInt64("a"); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
}

// The strict EOF check must not materialize what it is rejecting: this package
// reads files owned by other programs, so a large trailing payload is
// attacker-shaped input, not a curiosity.
func TestTrailingGarbageRejectedWithoutDecodingIt(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"a":1} `)
	b.WriteByte('[')
	for i := 0; i < 200000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
	}
	b.WriteByte(']')
	src := b.String()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := jsonc.Parse([]byte(src), "test.jsonc"); err == nil {
		t.Fatal("trailing array accepted, want error")
	}
	runtime.ReadMemStats(&after)

	// Two comment/comma stripping passes copy the input, and the decoder
	// buffers it, so ~3x the input is the floor for any input of this size.
	// Decoding the trailing payload as well took ~9x when this was written.
	// The bound sits between the two.
	allocated := after.TotalAlloc - before.TotalAlloc
	if limit := 5 * uint64(len(src)); allocated > limit {
		t.Errorf("rejecting %d bytes of trailing data allocated %d bytes (limit %d); "+
			"the check is materializing what it discards", len(src), allocated, limit)
	}
}

// F0.2: a ${...} in foreign data is text, not a reference.
func TestSubstitutionSyntaxStaysLiteral(t *testing.T) {
	cfg := parse(t, `{"a": "${foo.bar}"}`)
	resolved, err := cfg.Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := resolved.GetString("a"); got != "${foo.bar}" {
		t.Errorf("a = %q, want the literal ${foo.bar}", got)
	}
}

func TestUseAsSubstitutionSourceUnderHOCON(t *testing.T) {
	base, err := jsonc.Parse([]byte(`{
	  // owned by the frontend build
	  "compilerOptions": { "outDir": "dist" },
	}`), "tsconfig.json")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg, err := hocon.ParseStringWithOptions(
		`artifacts = "./"${compilerOptions.outDir}"/bundle.js"`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	merged, err := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := merged.GetString("artifacts"); got != "./dist/bundle.js" {
		t.Errorf("artifacts = %q, want ./dist/bundle.js", got)
	}
}

// F3.5 — an unpaired surrogate escape is an error, mirroring F2.8 for
// .properties.
//
// encoding/json substitutes U+FFFD for one without a word, so the config held a
// character the document never contained and nothing failed: the
// plausible-but-wrong output this spec ranks worst. rs.hocon already refused
// (serde_json does); py.hocon kept the lone surrogate and failed later, at
// encode time, far from the parse that admitted it. ts.hocon accepts it and
// stays that way — JavaScript strings are UTF-16 like Java's, the S1.2.6-class
// divergence F2.8 already records.
func TestUnpairedSurrogateRejected(t *testing.T) {
	for name, src := range map[string]string{
		"lone high in a value": `{"a":"\ud800"}`,
		"lone low in a value":  `{"a":"\udc00"}`,
		"lone high in a key":   `{"\ud800":1}`,
		"lone after a pair":    `{"a":"\ud83d\ude00\ud800"}`,
		"high then non-low":    `{"a":"\ud800\u0041"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := jsonc.Parse([]byte(src), "test.jsonc")
			if err == nil {
				t.Fatalf("Parse(%s) succeeded, want an F3.5 error", src)
			}
			if !strings.Contains(err.Error(), "F3.5") {
				t.Errorf("error %q does not cite the spec item F3.5", err)
			}
		})
	}
}

// The other half: what must keep working. A valid pair is one astral
// codepoint, and the scan must not mistake text that merely looks like an
// escape for one.
func TestSurrogateCheckLeavesValidDocumentsAlone(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"valid pair":      {`{"a":"\ud83d\ude00"}`, "\U0001F600"},
		"astral literal":  {"{\"a\":\"\U0001F600\"}", "\U0001F600"},
		"ordinary escape": {`{"a":"x\u0041y"}`, "xAy"},
		// `\` is an escaped backslash, so the `u` after it is text, not an
		// escape — a scan that just looked for `\u` would misread this.
		"escaped backslash":      {`{"a":"\\u0041"}`, `\u0041`},
		"surrogate-looking text": {`{"a":"\\ud800 is text"}`, `\ud800 is text`},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := jsonc.Parse([]byte(tc.src), "test.jsonc")
			if err != nil {
				t.Fatalf("Parse(%s): %v", tc.src, err)
			}
			if got := cfg.GetString("a"); got != tc.want {
				t.Errorf("a = %q, want %q", got, tc.want)
			}
		})
	}
}
