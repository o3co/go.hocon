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
// The syntax layer is shared with the parser's own `include "x.properties"`
// handling, so the two cannot drift apart.
//
// See the F2.x items in the format-ingestion mapping spec:
// https://github.com/o3co/xx.hocon/blob/main/docs/format-ingestion-mapping.md
package properties

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/bom"
	"github.com/o3co/go.hocon/adapters/internal/depth"
	"github.com/o3co/go.hocon/adapters/internal/pathmap"
	syntax "github.com/o3co/go.hocon/internal/properties"
)

// Parse reads Properties-syntax data.  originDescription names the source in
// error messages; "" leaves it to hocon's default.
//
// Encoding is UTF-8 with \uXXXX escapes honoured, matching java.util.Properties
// load(Reader) rather than the ISO-8859-1 byte-stream form (spec F2.3).
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	pairs, err := syntax.Parse(string(bom.Strip(data)))
	if err != nil {
		return nil, fmt.Errorf("properties: %s: %w", describe(originDescription), err)
	}

	// Sorted so a document with more than one bad key reports the same one on
	// every run.
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]pathmap.Entry, 0, len(pairs))
	for _, key := range keys {
		path, err := splitPath(key)
		if err != nil {
			return nil, fmt.Errorf("properties: %s: %w", describe(originDescription), err)
		}
		entries = append(entries, pathmap.Entry{Path: path, Value: pairs[key], Source: key})
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
	// Counted before splitting, so an absurdly dotted key is refused without
	// first allocating the slice it would have produced.
	if segments := strings.Count(key, ".") + 1; depth.TooDeep(segments) {
		// One dotted key produces one arbitrarily deep chain — see the depth
		// package for why the limit is here even though Go survives it.
		return nil, fmt.Errorf("key %q maps to a path %d segments deep, over the limit of %d",
			key, segments, depth.MaxPathSegments)
	}
	return strings.Split(key, "."), nil
}
