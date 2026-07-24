// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package properties parses Java .properties files.
//
// It backs two callers: `include "x.properties"`, which Lightbend serves by
// handing the file to java.util.Properties, and the properties adapter under
// adapters/. Both need the same syntax, so it lives here once.
package properties

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Parse parses .properties text into key/value pairs, following
// java.util.Properties: `=`, `:` or whitespace separators, `#` and `!`
// comments, backslash continuations, and the `\t \n \r \f \uXXXX` escapes.
//
// A repeated key keeps the last value. Trailing whitespace in a value is
// **kept** — Java skips whitespace before the value, never after it.
//
// Encoding is UTF-8, matching java.util.Properties.load(Reader) rather than
// the ISO-8859-1 byte-stream form.
func Parse(input string) (map[string]string, error) {
	if !utf8.ValidString(input) {
		return nil, fmt.Errorf("properties: input is not valid UTF-8")
	}
	result := make(map[string]string)
	for _, line := range logicalLines(input) {
		rawKey, rawValue := splitKeyValue(line)
		key, err := unescape(rawKey)
		if err != nil {
			return nil, fmt.Errorf("properties: in key: %w", err)
		}
		value, err := unescape(rawValue)
		if err != nil {
			return nil, fmt.Errorf("properties: in value for key %q: %w", key, err)
		}
		result[key] = value
	}
	return result, nil
}

// logicalLines drops blank and comment lines and joins backslash
// continuations. Comment status is decided per natural line before joining, so
// a continuation line that happens to start with '#' is value text.
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

// splitKeyValue splits at the first unescaped '=', ':' or whitespace run, then
// skips whitespace around that separator. Whatever remains is the value,
// trailing whitespace included.
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
		if c == '=' || c == ':' || isSpace(c) {
			break
		}
		k = append(k, c)
	}
	for i < len(r) && isSpace(r[i]) {
		i++
	}
	if i < len(r) && (r[i] == '=' || r[i] == ':') {
		i++
		for i < len(r) && isSpace(r[i]) {
			i++
		}
	}
	return string(k), string(r[i:])
}

func isSpace(c rune) bool { return c == ' ' || c == '\t' || c == '\f' }

// unescape applies the java.util.Properties escape rules. An unknown escape
// drops the backslash and a trailing lone backslash is dropped, both as Java
// does.
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
// when one follows. It returns how many runes past 'u' were consumed.
//
// Java strings are UTF-16 and can hold an unpaired surrogate; Go strings
// cannot, and writing one would silently produce U+FFFD, so it is an error.
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
