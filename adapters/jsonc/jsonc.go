// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package jsonc ingests JSON with comments and trailing commas — the dialect
// VS Code and TypeScript use for their config files — as HOCON config.
//
//	base, _ := jsonc.ParseFile("tsconfig.json")
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// Plain JSON needs no adapter at all: HOCON is a JSON superset, so
// hocon.ParseFile already accepts it. This package exists for the two things
// HOCON does not accept, /* block comments */ and trailing commas. (HOCON does
// allow // and # comments of its own.)
//
// Comments and trailing commas are removed, then encoding/json does the
// parsing, so the accepted grammar is otherwise exactly Go's JSON.
//
// Two consequences of that removal are worth stating, because both are strict
// where a JSONC reader could be sloppy (spec F3.2):
//
//   - A comment is replaced by whitespace, never by nothing, so it still
//     separates the tokens around it: 1/*x*/2 is a syntax error, not 12.
//   - A document holds exactly one value. Whitespace and comments may follow
//     it, but anything else — including a stray closer such as {"a":1} } —
//     is an error rather than silently ignored text.
//
// This package is part of the github.com/o3co/go.hocon/adapters module, which
// is versioned separately from the parser: see the module README for the
// go get line and the core-version requirement.
//
// See docs/specs/format-ingestion-mapping.md items F3.x in the hocon scope.
package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/tree"
)

// Parse reads JSONC data. originDescription names the source in error
// messages; "" leaves it to hocon's default.
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	doc, err := decode(data, originDescription)
	if err != nil {
		return nil, err
	}
	nested, err := tree.Object(doc, scalar)
	if err != nil {
		return nil, fmt.Errorf("jsonc: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
}

// ParseFile reads path and parses it, using path as the origin description.
func ParseFile(path string) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("jsonc: %w", err)
	}
	return Parse(data, path)
}

func decode(data []byte, origin string) (any, error) {
	cleaned, err := StripComments(data)
	if err != nil {
		return nil, fmt.Errorf("jsonc: %s: %w", describe(origin), err)
	}
	cleaned = stripTrailingCommas(cleaned)

	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.UseNumber() // keep the source's integer/float distinction (spec F0.5)

	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("jsonc: %s: %w", describe(origin), err)
	}

	// F3.2: trailing content after the top-level value is an error via a
	// strict EOF check. Decoder.More is only a one-token peek and reports
	// false on a closing bracket, so stray closers ({"a":1} }) would pass;
	// a second Decode must hit io.EOF instead.
	var extra any
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return doc, nil
	case err == nil:
		return nil, fmt.Errorf("jsonc: %s: unexpected data after the top-level value", describe(origin))
	default:
		return nil, fmt.Errorf("jsonc: %s: unexpected data after the top-level value: %w", describe(origin), err)
	}
}

func describe(origin string) string {
	if origin == "" {
		return "(in-memory)"
	}
	return origin
}

// scalar maps a decoded JSON leaf onto a value hocon.FromMap accepts.
func scalar(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string:
		return x, nil
	case json.Number:
		return number(x)
	}
	return nil, fmt.Errorf("unsupported value of type %T", v)
}

// number keeps integers as int64 and only widens to float64 when the source
// actually wrote a fraction or exponent, so large integers surface as an error
// rather than silently losing precision (spec F0.5).
func number(n json.Number) (any, error) {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		i, err := n.Int64()
		if err != nil {
			return nil, fmt.Errorf("integer %s does not fit in int64 (spec F0.5)", s)
		}
		return i, nil
	}
	f, err := n.Float64()
	if err != nil {
		return nil, fmt.Errorf("number %s is out of range for float64 (spec F0.5)", s)
	}
	return f, nil
}

// StripComments replaces // line comments and /* block comments */ with
// whitespace, leaving string literals untouched.  A comment always becomes at
// least one space, never the empty string, so it stays token-separating:
// 1/*x*/2 remains two tokens and fails the JSON decode (spec F3.2).  Newlines
// inside removed spans are preserved so that encoding/json still reports
// useful offsets.
func StripComments(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == '"':
			end, err := endOfString(data, i)
			if err != nil {
				return nil, err
			}
			out = append(out, data[i:end]...)
			i = end
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			out = append(out, ' ')
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			end := bytes.Index(data[i+2:], []byte("*/"))
			if end < 0 {
				return nil, fmt.Errorf("unterminated /* comment")
			}
			out = append(out, ' ')
			for _, b := range data[i : i+2+end+2] {
				if b == '\n' {
					out = append(out, '\n')
				}
			}
			i += 2 + end + 2
		default:
			out = append(out, c)
			i++
		}
	}
	return out, nil
}

// endOfString returns the index just past the string literal starting at i.
func endOfString(data []byte, i int) (int, error) {
	for j := i + 1; j < len(data); j++ {
		switch data[j] {
		case '\\':
			j++ // skip the escaped byte
		case '"':
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated string literal")
}

// stripTrailingCommas drops a comma whose next meaningful byte closes the
// enclosing object or array.  Comments are already gone by this point.
func stripTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		c := data[i]
		if c == '"' {
			end, err := endOfString(data, i)
			if err != nil {
				// Unterminated strings are rejected by StripComments already;
				// leave the remainder for encoding/json to complain about.
				out = append(out, data[i:]...)
				return out
			}
			out = append(out, data[i:end]...)
			i = end
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(data) && isJSONSpace(data[j]) {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				i++ // drop the comma, keep the whitespace that follows
				continue
			}
		}
		out = append(out, c)
		i++
	}
	return out
}

func isJSONSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
