// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package toml ingests TOML documents as HOCON config.
//
//	base, _ := toml.ParseFile("pyproject.toml")
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// TOML's types line up with HOCON's apart from dates: HOCON has no datetime,
// so all four TOML date-time types become their RFC 3339 string forms, which
// is the honest representation rather than a lossy number (spec F4.2).
//
// See docs/specs/format-ingestion-mapping.md items F4.x in the hocon scope.
package toml

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/tree"
	gotoml "github.com/pelletier/go-toml/v2"
)

// Parse reads TOML data. originDescription names the source in error
// messages; "" leaves it to hocon's default.
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	var doc any
	if err := gotoml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("toml: %s: %w", describe(originDescription), err)
	}
	nested, err := tree.Object(doc, scalar)
	if err != nil {
		return nil, fmt.Errorf("toml: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
}

// ParseFile reads path and parses it, using path as the origin description.
func ParseFile(path string) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("toml: %w", err)
	}
	return Parse(data, path)
}

func describe(origin string) string {
	if origin == "" {
		return "(in-memory)"
	}
	return origin
}

// scalar maps a decoded TOML leaf onto a value hocon.FromMap accepts.
//
// TOML has no null, so nil never appears in a decoded document.
func scalar(v any) (any, error) {
	switch x := v.(type) {
	case bool, string, int64:
		return x, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("%v is not representable in HOCON (spec F0.6)", x)
		}
		return x, nil
	case time.Time:
		return x.Format(time.RFC3339Nano), nil
	case gotoml.LocalDate:
		return x.String(), nil
	case gotoml.LocalTime:
		return x.String(), nil
	case gotoml.LocalDateTime:
		return x.String(), nil
	}
	return nil, fmt.Errorf("unsupported value of type %T", v)
}
