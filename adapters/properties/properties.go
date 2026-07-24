// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package properties ingests java.util.Properties files as HOCON config.
//
// The result is always a fully resolved *hocon.Config holding only strings, so
// it is useful as a fallback layer under a HOCON document:
//
//	base, _ := properties.ParseFile("service.properties")
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// Deferring resolution matters: the plain hocon.ParseFile resolves as it parses,
// so a ${...} pointing into the Properties file would fail before the fallback
// is ever attached.
//
// Substitutions are never parsed out of the input: a value of ${a.b} stays the
// literal text ${a.b}, because the file belongs to another program (spec F0.2).
//
// See docs/specs/format-ingestion-mapping.md items F2.x in the hocon scope.
package properties

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/pathmap"
)

// Parse reads Properties-syntax data.  originDescription names the source in
// error messages; "" leaves it to hocon's default.
//
// Encoding is UTF-8 with \uXXXX escapes honoured, matching java.util.Properties
// load(Reader) rather than the ISO-8859-1 byte-stream form (spec F2.3).
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("properties: %s: input is not valid UTF-8", describe(originDescription))
	}

	var entries []pathmap.Entry
	for _, line := range logicalLines(string(data)) {
		rawKey, rawVal := splitKeyValue(line)
		key, err := unescape(rawKey)
		if err != nil {
			return nil, fmt.Errorf("properties: %s: in key: %w", describe(originDescription), err)
		}
		value, err := unescape(rawVal)
		if err != nil {
			return nil, fmt.Errorf("properties: %s: in value for key %q: %w", describe(originDescription), key, err)
		}
		path, err := splitPath(key)
		if err != nil {
			return nil, fmt.Errorf("properties: %s: %w", describe(originDescription), err)
		}
		entries = append(entries, pathmap.Entry{Path: path, Value: value, Source: key})
	}

	nested, err := pathmap.Build(entries)
	if err != nil {
		return nil, fmt.Errorf("properties: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
}

// ParseFile reads path and parses it, using path as the origin description.
func ParseFile(path string) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("properties: %w", err)
	}
	return Parse(data, path)
}

func describe(origin string) string {
	if origin == "" {
		return "(in-memory)"
	}
	return origin
}

// splitPath splits an already-unescaped key on '.' into path segments.
//
// Lightbend routes the key through its full path-expression parser, which also
// accepts quoted segments such as foo."bar.baz".  That form is rejected here
// rather than silently mis-split, pending a fixture-backed decision (F2.7).
func splitPath(key string) ([]string, error) {
	if strings.Contains(key, `"`) {
		return nil, fmt.Errorf("key %q: quoted path segments are not supported yet (spec F2.7)", key)
	}
	return strings.Split(key, "."), nil
}

// logicalLines drops blank and comment lines and joins backslash continuations,
// mirroring java.util.Properties' line reader.  Comment status is decided per
// natural line before joining, so a continuation line starting with '#' is
// value text, not a comment.
func logicalLines(s string) []string {
	natural := splitNaturalLines(s)
	var out []string
	for i := 0; i < len(natural); i++ {
		line := strings.TrimLeft(natural[i], " \t\f")
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		for endsWithContinuation(line) {
			line = line[:len(line)-1]
			if i+1 >= len(natural) {
				break
			}
			i++
			line += strings.TrimLeft(natural[i], " \t\f")
		}
		out = append(out, line)
	}
	return out
}

func splitNaturalLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

// endsWithContinuation reports whether the line ends in an odd number of
// backslashes, which is what makes the final one an escape rather than escaped.
func endsWithContinuation(line string) bool {
	n := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// splitKeyValue splits at the first unescaped '=', ':' or whitespace run,
// then skips whitespace around that separator.
func splitKeyValue(line string) (key, value string) {
	r := []rune(line)
	var k []rune
	i := 0
	for ; i < len(r); i++ {
		c := r[i]
		if c == '\\' && i+1 < len(r) {
			k = append(k, c, r[i+1])
			i++
			continue
		}
		if c == '=' || c == ':' || isPropsSpace(c) {
			break
		}
		k = append(k, c)
	}
	for i < len(r) && isPropsSpace(r[i]) {
		i++
	}
	if i < len(r) && (r[i] == '=' || r[i] == ':') {
		i++
		for i < len(r) && isPropsSpace(r[i]) {
			i++
		}
	}
	return string(k), string(r[i:])
}

func isPropsSpace(c rune) bool { return c == ' ' || c == '\t' || c == '\f' }

// unescape applies java.util.Properties escape rules.  An unknown escape drops
// the backslash, and a trailing lone backslash is dropped, both as Java does.
func unescape(s string) (string, error) {
	r := []rune(s)
	var out strings.Builder
	for i := 0; i < len(r); i++ {
		if r[i] != '\\' {
			out.WriteRune(r[i])
			continue
		}
		i++
		if i >= len(r) {
			break
		}
		switch r[i] {
		case 't':
			out.WriteRune('\t')
		case 'n':
			out.WriteRune('\n')
		case 'r':
			out.WriteRune('\r')
		case 'f':
			out.WriteRune('\f')
		case 'u':
			cp, consumed, err := unicodeEscape(r, i)
			if err != nil {
				return "", err
			}
			out.WriteRune(cp)
			i += consumed
		default:
			out.WriteRune(r[i])
		}
	}
	return out.String(), nil
}

// unicodeEscape decodes the \uXXXX at r[i] == 'u', combining a surrogate pair
// when one follows.  It returns how many runes past 'u' were consumed.
//
// Java strings are UTF-16 and can hold an unpaired surrogate; Go strings cannot,
// and writing one would silently produce U+FFFD, so it is an error here.
func unicodeEscape(r []rune, i int) (cp rune, consumed int, err error) {
	hi, err := hex4(r, i+1)
	if err != nil {
		return 0, 0, err
	}
	if hi < 0xD800 || hi > 0xDFFF {
		return hi, 4, nil
	}
	if hi > 0xDBFF {
		return 0, 0, fmt.Errorf(`\u%04X is an unpaired low surrogate`, hi)
	}
	if i+6 < len(r) && r[i+5] == '\\' && r[i+6] == 'u' {
		if lo, err := hex4(r, i+7); err == nil && lo >= 0xDC00 && lo <= 0xDFFF {
			return ((hi - 0xD800) << 10) + (lo - 0xDC00) + 0x10000, 10, nil
		}
	}
	return 0, 0, fmt.Errorf(`\u%04X is an unpaired high surrogate`, hi)
}

func hex4(r []rune, start int) (rune, error) {
	if start+4 > len(r) {
		return 0, fmt.Errorf(`truncated \u escape`)
	}
	digits := string(r[start : start+4])
	v, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return 0, fmt.Errorf(`invalid \u escape %q`, digits)
	}
	return rune(v), nil
}
