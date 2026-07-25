// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package env mounts environment variables, or a .env file, as HOCON config.
//
// This is the bulk-mount case: a whole prefixed namespace becomes a config
// subtree, so ${...} references can reach it.  Reading one variable at a time
// needs nothing from this package — HOCON's own ${?VAR} already does that.
//
//	base, _ := env.Load(env.Options{Prefix: "APP_"})   // APP_DB__HOST -> db.host
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// Deferring resolution matters: the plain hocon.ParseFile resolves as it parses,
// so a ${...} pointing at a mounted variable would fail before the fallback is
// ever attached.
//
// Values are always strings and ${...} inside them stays literal, since the
// environment belongs to whoever launched the process (spec F0.2, F1.4).
//
// See docs/specs/format-ingestion-mapping.md items F1.x in the hocon scope.
package env

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/bom"
	"github.com/o3co/go.hocon/adapters/internal/keypath"
	"github.com/o3co/go.hocon/adapters/internal/pathmap"
)

// separator is the double underscore that marks a path boundary; a single
// underscore stays part of the segment, so APP_DB__MAX_CONN is db.max_conn.
// It is fixed rather than configurable so that every language's adapter nests
// identically (spec F1.2).
const separator = "__"

// Options controls how variable names become config paths.
type Options struct {
	// Prefix selects which variables to mount and is stripped from the path.
	// Load requires it: mounting the entire environment would pull in PATH,
	// HOME and any secrets that happen to be set (spec F1.1).  Parse and
	// ParseFile accept an empty Prefix, because a .env file is a closed set
	// the caller chose deliberately.
	Prefix string

	// Origin names the source in error messages.  Defaults to a description of
	// the environment, or to the file path.
	Origin string

	// Environ overrides the variable list for Load, in os.Environ form
	// ("NAME=value").  Nil means the real environment.
	Environ []string
}

// Load mounts the process environment.
func Load(opts Options) (*hocon.Config, error) {
	if opts.Prefix == "" {
		return nil, errors.New("env: Options.Prefix is required when mounting the environment (spec F1.1)")
	}
	environ := opts.Environ
	if environ == nil {
		environ = os.Environ()
	}
	// Sorted so that a collision is reported the same way on every run.
	sorted := append([]string(nil), environ...)
	sort.Strings(sorted)

	pairs := make([]pair, 0, len(sorted))
	for _, kv := range sorted {
		name, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue // not in NAME=value form; not a variable we can mount
		}
		pairs = append(pairs, pair{name: name, value: value})
	}
	return build(pairs, opts, "environment variables", true)
}

// Parse reads .env file content.
//
// The dialect is deliberately small (spec F1.7): NAME=value, optional "export "
// prefix, whole-line # comments, single quotes taken literally, double quotes
// with \n \r \t \\ \" escapes.  Multi-line values and trailing comments are not
// supported — an unquoted value containing " #" is an error rather than a guess
// about whether the user meant a comment.  No ${...} expansion is performed.
func Parse(data []byte, opts Options) (*hocon.Config, error) {
	origin := opts.Origin
	if origin == "" {
		origin = ".env"
	}
	data = bom.Strip(data) // spec F0.9
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("env: %s: input is not valid UTF-8", origin)
	}
	pairs, err := parseDotEnv(string(data), origin)
	if err != nil {
		return nil, err
	}
	return build(pairs, opts, origin, false)
}

// ParseFile reads path as a .env file, using path as the origin description.
func ParseFile(path string, opts Options) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("env: %w", err)
	}
	if opts.Origin == "" {
		opts.Origin = path
	}
	return Parse(data, opts)
}

type pair struct{ name, value string }

// build maps names to paths and hands the nesting to pathmap.
//
// fromProcessEnv marks the bulk-mount path, where two rules apply that a .env
// file does not need:
//
//   - F1.6: two names can map to one path (APP_A__B and APP_a__b both reach
//     a.b) and the environment has no meaningful order to break the tie with,
//     so it is an error.  A .env file has a definite line order, so the last
//     entry simply wins (spec F0.7).
//   - F1.9(b): an entry that is not valid UTF-8 is an error.  A .env file is
//     validated as a whole in Parse, since its bytes are one document.
func build(pairs []pair, opts Options, defaultOrigin string, fromProcessEnv bool) (*hocon.Config, error) {
	origin := opts.Origin
	if origin == "" {
		origin = defaultOrigin
	}

	seen := make(map[string]string, len(pairs))
	entries := make([]pathmap.Entry, 0, len(pairs))
	for _, p := range pairs {
		if !strings.HasPrefix(p.name, opts.Prefix) {
			continue
		}
		// F1.9(b): a bulk mount is an explicit request for a whole namespace,
		// so an entry inside it that cannot be decoded is an error.  Dropping
		// it would leave a subtree that looks complete while the operator's
		// setting is missing, and a stale default would win invisibly;
		// admitting the raw bytes is worse, since the key becomes unreachable
		// text.  Deliberately after the prefix filter: an undecodable variable
		// the caller never asked for must not break an unrelated mount.
		if fromProcessEnv {
			if !utf8.ValidString(p.name) {
				return nil, fmt.Errorf("env: %s: variable name %q is not valid UTF-8 (spec F1.9)",
					origin, p.name)
			}
			// The value is named but never echoed — environment values are
			// where credentials live.
			if !utf8.ValidString(p.value) {
				return nil, fmt.Errorf("env: %s: the value of %s is not valid UTF-8 (spec F1.9)",
					origin, p.name)
			}
		}
		path := toPath(strings.TrimPrefix(p.name, opts.Prefix))
		if fromProcessEnv {
			k := pathKey(path)
			if prev, dup := seen[k]; dup {
				// The path is rendered as a HOCON path expression, so a
				// segment holding a literal dot is quoted and cannot be
				// misread as two segments.  Same format in py.hocon and
				// rs.hocon; the xx.hocon fi11-collision fixture cites the
				// phrase "both map to".
				return nil, fmt.Errorf("env: %s: %s and %s both map to %s",
					origin, prev, p.name, keypath.Render(path))
			}
			seen[k] = p.name
		}
		entries = append(entries, pathmap.Entry{Path: path, Value: p.value, Source: p.name})
	}

	nested, err := pathmap.Build(entries)
	if err != nil {
		return nil, fmt.Errorf("env: %s: %w", origin, err)
	}
	return hocon.FromMap(nested, origin)
}

// pathKey indexes a path for collision detection.
//
// Joining the segments on a delimiter would need a byte that cannot occur in
// one, and no such byte exists: Options.Environ lets a caller pass any name,
// NUL included, so "A\x00B" (one segment) and "A__B" (two) hashed alike and
// produced a collision that was not there.  Length-prefixing every segment
// removes the assumption instead of moving it to a rarer byte.
func pathKey(path []string) string {
	var b strings.Builder
	for _, seg := range path {
		fmt.Fprintf(&b, "%d:%s", len(seg), seg)
	}
	return b.String()
}

// toPath splits a prefix-stripped name on "__" and lowercases each segment.
//
// The fold is ASCII-only (spec F1.3).  Go's strings.ToLower applies simple
// case mapping, so İ (U+0130) becomes "i" and would collide with I under F1.6,
// while Python, JS and Rust apply the full mapping and keep the two apart.
// Environment variable names are ASCII in every practical setting, so folding
// only A-Z costs nothing and makes the implementations agree.
func toPath(name string) []string {
	segs := strings.Split(name, separator)
	for i := range segs {
		segs[i] = lowerASCII(segs[i])
	}
	return segs
}

func lowerASCII(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c + ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

func parseDotEnv(s string, origin string) ([]pair, error) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var pairs []pair
	for i, raw := range strings.Split(s, "\n") {
		lineno := i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		name, rest, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("env: %s:%d: expected NAME=value", origin, lineno)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("env: %s:%d: empty variable name", origin, lineno)
		}
		value, err := dotEnvValue(strings.TrimLeft(rest, " \t"))
		if err != nil {
			return nil, fmt.Errorf("env: %s:%d: %s: %w", origin, lineno, name, err)
		}
		pairs = append(pairs, pair{name: name, value: value})
	}
	return pairs, nil
}

func dotEnvValue(v string) (string, error) {
	switch {
	case strings.HasPrefix(v, "'"):
		end := strings.Index(v[1:], "'")
		if end < 0 {
			return "", errors.New("unterminated ' quote (multi-line values are not supported)")
		}
		return v[1 : 1+end], trailingGarbage(v[2+end:])
	case strings.HasPrefix(v, `"`):
		return doubleQuoted(v)
	default:
		v = strings.TrimRight(v, " \t")
		if indexSpaceHash(v) >= 0 {
			return "", fmt.Errorf("ambiguous value %q: trailing comments are not supported, so quote the value if the # belongs to it", v)
		}
		return v, nil
	}
}

func doubleQuoted(v string) (string, error) {
	var out strings.Builder
	for i := 1; i < len(v); i++ {
		switch v[i] {
		case '"':
			return out.String(), trailingGarbage(v[i+1:])
		case '\\':
			i++
			if i >= len(v) {
				return "", errors.New(`dangling \ at end of line`)
			}
			switch v[i] {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '\\':
				out.WriteByte('\\')
			case '"':
				out.WriteByte('"')
			default:
				return "", fmt.Errorf(`unknown escape \%c (supported: \n \r \t \\ \")`, v[i])
			}
		default:
			out.WriteByte(v[i])
		}
	}
	return "", errors.New(`unterminated " quote (multi-line values are not supported)`)
}

func trailingGarbage(rest string) error {
	if strings.TrimSpace(rest) != "" {
		return fmt.Errorf("unexpected text %q after closing quote", strings.TrimSpace(rest))
	}
	return nil
}

// indexSpaceHash finds a '#' preceded by whitespace, the shape that other
// .env dialects treat as a trailing comment.  A '#' elsewhere (#fff, a#b) is
// ordinary value text.
func indexSpaceHash(v string) int {
	for i := 1; i < len(v); i++ {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			return i
		}
	}
	return -1
}
