// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package yamlscenario defines the Go-side types for the cross-impl E12
// scenario YAML schema (testdata/hocon/deferred-resolution/*.yaml).
//
// Schema reference:
//
//	testdata/hocon/deferred-resolution/README.md
package yamlscenario

// Scenario is the top-level YAML document.
type Scenario struct {
	Description   string            `yaml:"description"`
	Xref          []string          `yaml:"xref,omitempty"`
	LightbendSkip bool              `yaml:"lightbendSkip,omitempty"`
	Sources       map[string]Source `yaml:"sources"`
	Build         []Step            `yaml:"build"`
	Expect        Expect            `yaml:"expect"`
}

type Source struct {
	ParseString       string         `yaml:"parseString,omitempty"`
	ParseOptions      *ParseOpts     `yaml:"parseOptions,omitempty"`
	FromMap           map[string]any `yaml:"fromMap,omitempty"`
	OriginDescription string         `yaml:"originDescription,omitempty"`
}

type ParseOpts struct {
	ResolveSubstitutions *bool  `yaml:"resolveSubstitutions,omitempty"`
	OriginDescription    string `yaml:"originDescription,omitempty"`
}

type Step struct {
	Op                   string `yaml:"op"`
	Source               string `yaml:"source,omitempty"` // for `take`, `resolveWith`
	This                 string `yaml:"this,omitempty"`
	Other                string `yaml:"other,omitempty"`  // for `withFallback`
	Path                 string `yaml:"path,omitempty"`   // for `extract`
	AllowUnresolved      *bool  `yaml:"allowUnresolved,omitempty"`
	UseSystemEnvironment *bool  `yaml:"useSystemEnvironment,omitempty"`
	As                   string `yaml:"as"`
}

type Expect struct {
	Outcome       string         `yaml:"outcome"`
	JSON          string         `yaml:"json,omitempty"`
	IsResolved    *bool          `yaml:"isResolved,omitempty"`
	Getter        []GetterAssert `yaml:"getter,omitempty"`
	ErrorAt       *int           `yaml:"errorAt,omitempty"`
	ErrorCategory string         `yaml:"errorCategory,omitempty"`
	ErrorContains string         `yaml:"errorContains,omitempty"`
}

type GetterAssert struct {
	Path         string         `yaml:"path"`
	ExpectString *string        `yaml:"expectString,omitempty"`
	ExpectInt    *int64         `yaml:"expectInt,omitempty"`
	ExpectBool   *bool          `yaml:"expectBool,omitempty"`
	ExpectObject map[string]any `yaml:"expectObject,omitempty"`
	ExpectArray  []any          `yaml:"expectArray,omitempty"`
	ExpectError  string         `yaml:"expectError,omitempty"` // e.g. "NotResolved"
}
