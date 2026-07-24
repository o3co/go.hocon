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
// Backed by goccy/go-yaml, which follows the YAML 1.2 core schema for
// booleans, so the "Norway problem" does not arise here: no, yes, on and off
// stay strings and only true/false are booleans.
//
// Two things are refused rather than passed through. A multi-document stream
// is an error, because decoding one would silently drop the rest. NaN and
// infinity are errors, because HOCON's number model has no way to hold them.
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

	goyaml "github.com/goccy/go-yaml"
	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/tree"
)

// Parse reads YAML data. originDescription names the source in error
// messages; "" leaves it to hocon's default.
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	doc, err := decode(data, originDescription)
	if err != nil {
		return nil, err
	}
	// An empty document is the empty object, as an empty HOCON document is
	// (S3.1), rather than a root-type failure (spec F5.9).
	if doc == nil {
		return hocon.FromMap(map[string]any{}, originDescription)
	}
	nested, err := tree.Object(doc, scalar)
	if err != nil {
		return nil, fmt.Errorf("yaml: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
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
	}
	return nil, fmt.Errorf("unsupported value of type %T", v)
}
