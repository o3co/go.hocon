// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package yaml ingests YAML documents as HOCON config.
//
//	base, _ := yaml.ParseFile("docker-compose.yml")
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// This is a HOCON library, not a YAML implementation, and the API keeps that
// boundary. What this package owns is the decoded-tree -> HOCON step, exposed
// directly as FromValue: root must be a mapping, ${...} stays literal, NaN and
// infinity are refused, a multi-document stream is refused, binary becomes its
// base64 text. How YAML *text* becomes a tree — whether 010 is 8 or 10,
// whether no is a boolean — is the YAML library's answer, not a contract here.
//
// Parse and ParseFile are a convenience front on goccy/go-yaml. A caller who
// needs a different library, version or schema decodes the text themselves and
// hands the tree to FromValue; that is the supported way to swap parsers, and
// it keeps the choice — and its consequences — in the caller's hands.
//
// An injected tree gets the same key rules as a parsed one. A non-string
// scalar key takes its string form and a collection key is refused, so two
// siblings whose string forms coincide — the int 1 and the string "1" in one
// map[any]any — are an error naming both (spec F5.3). Go randomizes map
// iteration, so letting the last one win would mean a different config on a
// different run.
//
// This package is part of the github.com/o3co/go.hocon/adapters module, which
// is versioned separately from the parser: see the module README for the
// go get line and the core-version requirement.
//
// See docs/specs/format-ingestion-mapping.md items F5.x in the hocon scope.
package yaml

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	goyaml "github.com/goccy/go-yaml"
	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/tree"
)

// Parse reads YAML data with this package's default library (goccy/go-yaml).
// originDescription names the source in error messages; "" leaves it to
// hocon's default.
//
// Scalar resolution — whether `010` is 8 or 10, whether a timestamp is a
// string — is the library's answer, not a contract of this package. A caller
// who wants a different library, version or schema decodes the text themselves
// and hands the result to FromValue; this function is the convenience path.
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	doc, err := decode(data, originDescription)
	if err != nil {
		return nil, err
	}
	return FromValue(doc, originDescription)
}

// FromValue builds a Config from an already-decoded YAML value tree, produced
// by whatever YAML library and settings the caller chose. This is the
// tree-level boundary this package actually owns (spec F5): Parse is just a
// default decoder in front of it.
//
// Leaf normalization accepts the shapes common across Go YAML libraries, not
// only the default one: map[any]any (yaml.v2 style) has its scalar keys
// stringified per F5.3, time.Time (go.yaml.in timestamps) becomes its RFC 3339
// string like a TOML date (F4.2's reasoning), and []byte becomes base64 (F5.5).
//
// Stringifying can make two sibling keys collide (the int 1 and the string
// "1"); that is an error naming both, not a race decided by map iteration
// order (F5.3).
func FromValue(doc any, originDescription string) (*hocon.Config, error) {
	// An empty document is the empty object, as an empty HOCON document is
	// (S3.1), rather than a root-type failure (spec F5.9).
	if doc == nil {
		return hocon.FromMap(map[string]any{}, originDescription)
	}
	normalized, err := normalizeKeys(doc)
	if err != nil {
		return nil, fmt.Errorf("yaml: %s: %w", describe(originDescription), err)
	}
	nested, err := tree.Object(normalized, scalar)
	if err != nil {
		return nil, fmt.Errorf("yaml: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
}

// normalizeKeys rewrites map[any]any (the yaml.v2-era shape) into
// map[string]any, stringifying scalar keys (F5.3). A collection key is an
// error, and so are two sibling keys whose string forms coincide (1 and "1"):
// letting the last writer win would be nondeterministic under Go's randomized
// map iteration. Maps that are already string-keyed cannot collide — their
// keys are distinct strings by construction — and pass through with their
// values normalized recursively.
func normalizeKeys(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			ev, err := normalizeKeys(e)
			if err != nil {
				return nil, err
			}
			out[k] = ev
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(x))
		seen := make(map[string]any, len(x))
		for k, e := range x {
			ks, err := keyString(k)
			if err != nil {
				return nil, err
			}
			if prev, dup := seen[ks]; dup {
				a, b := keyForm(prev), keyForm(k)
				if b < a {
					a, b = b, a
				}
				return nil, fmt.Errorf(
					"sibling mapping keys %s and %s share the object key %q; "+
						"which value wins would depend on map iteration order (spec F5.3)",
					a, b, ks)
			}
			seen[ks] = k
			ev, err := normalizeKeys(e)
			if err != nil {
				return nil, err
			}
			out[ks] = ev
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			ev, err := normalizeKeys(e)
			if err != nil {
				return nil, err
			}
			out[i] = ev
		}
		return out, nil
	}
	return v, nil
}

func keyString(k any) (string, error) {
	switch x := k.(type) {
	case string:
		return x, nil
	case bool, int, int64, uint64, float64:
		return fmt.Sprintf("%v", x), nil
	}
	return "", fmt.Errorf("mapping key of type %T is not usable as an object key (spec F5.3)", k)
}

// keyForm renders a source key for the F5.3 collision error: strings are
// quoted, other scalars show their Go type, so 1 (int) and "1" stay apart in
// the message the way they failed to in the mapping.
func keyForm(k any) string {
	if s, ok := k.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("%v (%T)", k, k)
}

// ParseFile reads path and parses it, using path as the origin description.
func ParseFile(path string) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return Parse(data, path)
}

// decode reads exactly one document.
//
// F5.7: goyaml.Unmarshal returns the first document of a stream and discards
// the rest without a word, so the second decode below exists to turn that
// silent data loss into an error.
func decode(data []byte, origin string) (any, error) {
	dec := goyaml.NewDecoder(bytes.NewReader(data))

	var doc any
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil // empty input
		}
		return nil, fmt.Errorf("yaml: %s: %w", describe(origin), err)
	}

	var extra any
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return doc, nil
	case err == nil:
		return nil, fmt.Errorf(
			"yaml: %s: multi-document streams are not supported (spec F5.7); "+
				"a config is one document, and decoding only the first would drop the rest silently",
			describe(origin))
	default:
		return nil, fmt.Errorf("yaml: %s: %w", describe(origin), err)
	}
}

func describe(origin string) string {
	if origin == "" {
		return "(in-memory)"
	}
	return origin
}

// scalar maps a decoded YAML leaf onto a value hocon.FromMap accepts.
//
// Positive integers arrive as uint64 and negative ones as int64, so the
// overflow check lives on the uint64 branch.
func scalar(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string, int64:
		return x, nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("integer %d does not fit in int64 (spec F0.5)", x)
		}
		return int64(x), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("%v is not representable in HOCON (spec F0.6)", x)
		}
		return x, nil
	case []byte:
		// !!binary — HOCON has no binary type, so keep the base64 text the
		// source itself carried (spec F5.5).
		return base64.StdEncoding.EncodeToString(x), nil
	case time.Time:
		// Some libraries resolve timestamps to time.Time. HOCON has no
		// datetime, so its RFC 3339 text is the honest form — the same
		// reasoning as F4.2 for TOML dates.
		return x.Format(time.RFC3339Nano), nil
	case int:
		return int64(x), nil
	}
	return nil, fmt.Errorf("unsupported value of type %T", v)
}
