// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package hocon_test — Layer-2 E12 YAML scenario runner.
//
// Each scenario YAML in testdata/hocon/deferred-resolution/ is interpreted
// by this runner using the public hocon API.  Outcome is compared to the
// expected/* artefact (Lightbend ground truth) and to in-YAML assertions.
package hocon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/internal/yamlscenario"
)

const (
	drYAMLDir     = "testdata/hocon/deferred-resolution"
	drExpectedDir = "testdata/expected/deferred-resolution"
)

// dr17 is the E11 package-include scenario; the YAML runner can't register
// packages, so we skip it here.  dr17 has dedicated coverage in
// TestDr17_E11PackageIncludeDeferred (Task 12).
//
// dr19 (idempotency double-resolve) has a programmatic counterpart in
// TestResolve_OnAlreadyResolvedIsIdempotent — the YAML covers the single
// resolve.
//
// dr12 (origin preservation) checks Lightbend-format error position strings
// that don't translate to go.hocon's format; go.hocon's per-impl test
// covers the same intent (dr06 / dr08 / dr18 error tests).
var drYAMLSkip = map[string]string{
	"dr17": "E11 package-include — covered by TestDr17_E11PackageIncludeDeferred (programmatic)",
	"dr12": "origin format differs from Lightbend — go.hocon position info covered by dr06 / dr08 / dr18 error tests",
}

// errorContainsSkip lists scenarios where go.hocon's error message format
// is intentionally different from Lightbend's, so the YAML errorContains
// substring doesn't apply.  Scenarios listed here still run; only the
// errorContains substring check is downgraded to t.Logf.
//
// dr12 is already in drYAMLSkip; this map is for scenarios that otherwise
// pass but whose Lightbend-format errorContains hint doesn't match go.hocon's.
var errorContainsSkip = map[string]string{
	// (currently empty — dr08 verified to match; expand if cross-impl
	//  divergence surfaces in new fixtures)
}

func TestDeferredResolutionFixtures(t *testing.T) {
	entries, err := os.ReadDir(drYAMLDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v (did you run 'make testdata'?)", drYAMLDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		id := scenarioID(e.Name())
		if reason, skip := drYAMLSkip[id]; skip {
			t.Run(id, func(t *testing.T) { t.Skip(reason) })
			continue
		}
		t.Run(id, func(t *testing.T) { runScenario(t, id, filepath.Join(drYAMLDir, e.Name())) })
	}
}

func scenarioID(filename string) string {
	// "dr01-basic-fallback.yaml" → "dr01"
	// "dr11a-resolve-with-source-keys-absent.yaml" → "dr11a"
	base := strings.TrimSuffix(filename, ".yaml")
	if dash := strings.Index(base, "-"); dash > 0 {
		return base[:dash]
	}
	return base
}

func runScenario(t *testing.T, id, yamlPath string) {
	t.Helper()

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read %s: %v", yamlPath, err)
	}
	var sc yamlscenario.Scenario
	if err := yaml.Unmarshal(data, &sc); err != nil {
		t.Fatalf("yaml.Unmarshal %s: %v", yamlPath, err)
	}

	// Build sources -> Config artefacts.
	artefacts := make(map[string]*hocon.Config, len(sc.Sources))
	sourceErr := make(map[string]error, len(sc.Sources))
	for name, src := range sc.Sources {
		cfg, err := buildSource(src)
		if err != nil {
			sourceErr[name] = err
			continue
		}
		artefacts[name] = cfg
	}

	// Walk build steps; record per-step error (if any) for errorAt assertions.
	var stepErrors []error
	finalName := "result"
	for i, step := range sc.Build {
		stepErr := executeStep(i, step, artefacts, sourceErr)
		stepErrors = append(stepErrors, stepErr)
		if step.As != "" {
			finalName = step.As
		}
	}

	switch sc.Expect.Outcome {
	case "success":
		validateSuccess(t, id, sc, artefacts, finalName, stepErrors)
	case "error":
		validateError(t, id, sc, stepErrors)
	default:
		t.Fatalf("scenario %s: unknown expect.outcome %q", id, sc.Expect.Outcome)
	}
}

func buildSource(src yamlscenario.Source) (*hocon.Config, error) {
	if src.ParseString != "" {
		// Per fixture-conventions / scenario YAML README: parseString sources
		// default to ResolveSubstitutions=false (deferred mode).  Caller can
		// explicitly opt back into fused mode via parseOptions.resolveSubstitutions: true.
		opts := hocon.DefaultParseOptions().WithResolveSubstitutions(false)
		if src.ParseOptions != nil {
			if src.ParseOptions.ResolveSubstitutions != nil {
				opts = opts.WithResolveSubstitutions(*src.ParseOptions.ResolveSubstitutions)
			}
			if src.ParseOptions.OriginDescription != "" {
				opts = opts.WithOriginDescription(src.ParseOptions.OriginDescription)
			}
		}
		return hocon.ParseStringWithOptions(src.ParseString, opts)
	}
	if src.FromMap != nil {
		return hocon.FromMap(src.FromMap, src.OriginDescription)
	}
	return hocon.Empty(src.OriginDescription), nil
}

func executeStep(i int, step yamlscenario.Step, artefacts map[string]*hocon.Config, sourceErr map[string]error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if perr, ok := r.(error); ok {
				err = perr
			} else {
				err = fmt.Errorf("step %d panic: %v", i, r)
			}
		}
	}()
	switch step.Op {
	case "take":
		if e, hadErr := sourceErr[step.Source]; hadErr {
			return e
		}
		cfg, ok := artefacts[step.Source]
		if !ok {
			return fmt.Errorf("take: source %q not found", step.Source)
		}
		artefacts[step.As] = cfg
		return nil
	case "extract":
		base, ok := artefacts[step.This]
		if !ok {
			return fmt.Errorf("extract: this=%q not found", step.This)
		}
		sub := base.GetConfig(step.Path)
		artefacts[step.As] = sub
		return nil
	case "withFallback":
		base, ok := artefacts[step.This]
		if !ok {
			return fmt.Errorf("withFallback: this=%q not found", step.This)
		}
		fb, ok := artefacts[step.Other]
		if !ok {
			if e, hadErr := sourceErr[step.Other]; hadErr {
				return e
			}
			return fmt.Errorf("withFallback: other=%q not found", step.Other)
		}
		artefacts[step.As] = base.WithFallback(fb)
		return nil
	case "resolve":
		base, ok := artefacts[step.This]
		if !ok {
			return fmt.Errorf("resolve: this=%q not found", step.This)
		}
		opts := hocon.DefaultResolveOptions()
		if step.AllowUnresolved != nil {
			opts = opts.WithAllowUnresolved(*step.AllowUnresolved)
		}
		if step.UseSystemEnvironment != nil {
			opts = opts.WithUseSystemEnvironment(*step.UseSystemEnvironment)
		}
		out, err := base.Resolve(opts)
		if err != nil {
			return err
		}
		artefacts[step.As] = out
		return nil
	case "resolveWith":
		base, ok := artefacts[step.This]
		if !ok {
			return fmt.Errorf("resolveWith: this=%q not found", step.This)
		}
		src, ok := artefacts[step.Source]
		if !ok {
			if e, hadErr := sourceErr[step.Source]; hadErr {
				return e
			}
			return fmt.Errorf("resolveWith: source=%q not found", step.Source)
		}
		opts := hocon.DefaultResolveOptions()
		if step.AllowUnresolved != nil {
			opts = opts.WithAllowUnresolved(*step.AllowUnresolved)
		}
		if step.UseSystemEnvironment != nil {
			opts = opts.WithUseSystemEnvironment(*step.UseSystemEnvironment)
		}
		out, err := base.ResolveWith(src, opts)
		if err != nil {
			return err
		}
		artefacts[step.As] = out
		return nil
	default:
		return fmt.Errorf("unknown op %q", step.Op)
	}
}

func validateSuccess(t *testing.T, id string, sc yamlscenario.Scenario, artefacts map[string]*hocon.Config, finalName string, stepErrors []error) {
	t.Helper()
	for i, e := range stepErrors {
		if e != nil {
			t.Fatalf("scenario %s: unexpected error at step %d: %v", id, i, e)
		}
	}
	cfg, ok := artefacts[finalName]
	if !ok {
		t.Fatalf("scenario %s: final artefact %q not found", id, finalName)
	}
	if sc.Expect.IsResolved != nil && cfg.IsResolved() != *sc.Expect.IsResolved {
		t.Errorf("scenario %s: isResolved = %v, want %v", id, cfg.IsResolved(), *sc.Expect.IsResolved)
	}
	if sc.Expect.JSON != "" && cfg.IsResolved() {
		expectedJSON, err := loadExpectedJSON(id)
		if err == nil {
			compareJSON(t, id, expectedJSON, cfg)
		} else {
			compareJSONString(t, id, sc.Expect.JSON, cfg)
		}
	}
	for _, g := range sc.Expect.Getter {
		runGetter(t, id, cfg, g)
	}
}

func validateError(t *testing.T, id string, sc yamlscenario.Scenario, stepErrors []error) {
	t.Helper()
	idx := -1
	var firstErr error
	for i, e := range stepErrors {
		if e != nil {
			idx = i
			firstErr = e
			break
		}
	}
	if firstErr == nil {
		t.Fatalf("scenario %s: expected error (category=%s) but all steps succeeded", id, sc.Expect.ErrorCategory)
	}
	if sc.Expect.ErrorAt != nil && idx != *sc.Expect.ErrorAt {
		t.Errorf("scenario %s: errorAt = %d, want %d (err=%v)", id, idx, *sc.Expect.ErrorAt, firstErr)
	}
	if !categoryMatches(sc.Expect.ErrorCategory, firstErr) {
		t.Errorf("scenario %s: error category = %T %v, want %s", id, firstErr, firstErr, sc.Expect.ErrorCategory)
	}
	if sc.Expect.ErrorContains != "" && !strings.Contains(firstErr.Error(), sc.Expect.ErrorContains) {
		if _, skip := errorContainsSkip[id]; skip {
			// Known impl-format divergence — record but don't fail.
			t.Logf("scenario %s: errorContains %q not in %q (impl-specific format, expected)", id, sc.Expect.ErrorContains, firstErr.Error())
		} else {
			t.Errorf("scenario %s: error message %q does not contain expected substring %q", id, firstErr.Error(), sc.Expect.ErrorContains)
		}
	}
}

func categoryMatches(category string, err error) bool {
	switch category {
	case "ParseError":
		var pe *hocon.ParseError
		return errors.As(err, &pe)
	case "ResolveError":
		var re *hocon.ResolveError
		return errors.As(err, &re)
	case "NotResolved":
		return errors.Is(err, hocon.ErrNotResolved)
	case "TypeError":
		// go.hocon currently surfaces type errors as parse or resolve errors.
		var pe *hocon.ParseError
		var re *hocon.ResolveError
		return errors.As(err, &pe) || errors.As(err, &re)
	case "CycleError":
		// go.hocon surfaces cycles as ResolveError with "circular" / "cycle" / "self-referential" message.
		var re *hocon.ResolveError
		if !errors.As(err, &re) {
			return false
		}
		msg := strings.ToLower(re.Message)
		return strings.Contains(msg, "circular") || strings.Contains(msg, "cycle") ||
			strings.Contains(msg, "self-referential")
	}
	return false
}

func loadExpectedJSON(id string) ([]byte, error) {
	entries, err := os.ReadDir(drExpectedDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, id+"-") || !strings.HasSuffix(name, "-expected.json") {
			continue
		}
		return os.ReadFile(filepath.Join(drExpectedDir, name))
	}
	return nil, fmt.Errorf("no expected json for %s", id)
}

func compareJSON(t *testing.T, id string, expectedRaw []byte, cfg *hocon.Config) {
	t.Helper()
	actualStr, err := hocon.RenderJSON_ForTest(cfg)
	if err != nil {
		t.Fatalf("scenario %s: renderJSON: %v", id, err)
	}
	if !drJSONEqual(expectedRaw, []byte(actualStr)) {
		t.Errorf("scenario %s: JSON mismatch\n want: %s\n got:  %s", id, string(expectedRaw), actualStr)
	}
}

func compareJSONString(t *testing.T, id, expected string, cfg *hocon.Config) {
	t.Helper()
	actual, err := hocon.RenderJSON_ForTest(cfg)
	if err != nil {
		t.Fatalf("scenario %s: renderJSON: %v", id, err)
	}
	if !drJSONEqual([]byte(expected), []byte(actual)) {
		t.Errorf("scenario %s: in-YAML JSON mismatch\n want: %s\n got:  %s", id, expected, actual)
	}
}

// drJSONEqual compares two JSON byte slices for semantic equality (ignoring
// whitespace, key ordering, and number representation differences like 42 vs
// 42.0). Named with "dr" prefix to avoid conflict with lightbend_test.go's
// jsonEqual(a, b any) helper.
func drJSONEqual(a, b []byte) bool {
	var ja, jb any
	if err := json.Unmarshal(a, &ja); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &jb); err != nil {
		return false
	}
	return drJSONDeepEqual(ja, jb)
}

func drJSONDeepEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !drJSONDeepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !drJSONDeepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}

func runGetter(t *testing.T, id string, cfg *hocon.Config, g yamlscenario.GetterAssert) {
	t.Helper()
	if g.ExpectError == "NotResolved" {
		defer func() {
			rec := recover()
			ce, ok := rec.(*hocon.ConfigError)
			if !ok || !errors.Is(ce, hocon.ErrNotResolved) {
				t.Errorf("scenario %s getter %q: expected NotResolved panic, got %T %v", id, g.Path, rec, rec)
			}
		}()
		_ = cfg.GetString(g.Path)
		return
	}
	switch {
	case g.ExpectString != nil:
		if got := cfg.GetString(g.Path); got != *g.ExpectString {
			t.Errorf("scenario %s getter %q: got %q want %q", id, g.Path, got, *g.ExpectString)
		}
	case g.ExpectInt != nil:
		if got := cfg.GetInt64(g.Path); got != *g.ExpectInt {
			t.Errorf("scenario %s getter %q: got %d want %d", id, g.Path, got, *g.ExpectInt)
		}
	case g.ExpectBool != nil:
		if got := cfg.GetBool(g.Path); got != *g.ExpectBool {
			t.Errorf("scenario %s getter %q: got %v want %v", id, g.Path, got, *g.ExpectBool)
		}
	case g.ExpectArray != nil:
		actual := cfg.GetIntSlice(g.Path)
		if len(actual) != len(g.ExpectArray) {
			t.Errorf("scenario %s getter %q: len(actual)=%d want %d", id, g.Path, len(actual), len(g.ExpectArray))
			return
		}
		for i, ev := range g.ExpectArray {
			expectedInt, ok := numericToInt(ev)
			if !ok {
				continue
			}
			if int64(actual[i]) != expectedInt {
				t.Errorf("scenario %s getter %q[%d]: got %d want %d", id, g.Path, i, actual[i], expectedInt)
			}
		}
	}
}

func numericToInt(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	}
	return 0, false
}
