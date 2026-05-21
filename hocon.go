// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"errors"
	"os"

	"github.com/o3co/go.hocon/internal/parser"
	"github.com/o3co/go.hocon/internal/resolver"
)

// ParseString parses a HOCON string and returns a fully resolved Config.
// Equivalent to ParseStringWithOptions(input, DefaultParseOptions()).
func ParseString(input string) (*Config, error) {
	return parseWithOptions(input, "", DefaultParseOptions())
}

// ParseFile parses a HOCON file and returns a fully resolved Config.
// Equivalent to ParseFileWithOptions(path, DefaultParseOptions()).
func ParseFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{Message: err.Error(), FilePath: path}
	}
	return parseWithOptions(string(data), path, DefaultParseOptions())
}

// ParseStringWithOptions parses a HOCON string and returns a Config that is
// either fully resolved (opts.ResolveSubstitutions()=true, default) or possibly
// unresolved (opts.ResolveSubstitutions()=false).  See E12 for the deferred-
// resolution lifecycle.
func ParseStringWithOptions(input string, opts ParseOptions) (*Config, error) {
	return parseWithOptions(input, "", opts)
}

// ParseFileWithOptions parses a HOCON file and returns a Config per opts.
func ParseFileWithOptions(path string, opts ParseOptions) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{Message: err.Error(), FilePath: path}
	}
	return parseWithOptions(string(data), path, opts)
}

func parseWithOptions(input, filePath string, opts ParseOptions) (*Config, error) {
	ast, err := parser.Parse(input)
	if err != nil {
		pe := &ParseError{FilePath: filePath}
		if filePath == "" {
			pe.OriginDescription = opts.OriginDescription()
		}
		var parserErr *parser.Error
		if errors.As(err, &parserErr) {
			pe.Message = parserErr.Message
			pe.Line = parserErr.Line
			pe.Col = parserErr.Col
		} else {
			pe.Message = err.Error()
		}
		return nil, pe
	}
	baseDir := ""
	if filePath != "" {
		baseDir = dirOf(filePath)
	}
	resolveOpts := resolver.Options{
		BaseDir:              baseDir,
		PackageLookup:        globalRegistry.lookup,
		UseSystemEnvironment: true, // fused path: env always available; ResolveOptions gates phase 2 separately when called.
	}
	if opts.ResolveSubstitutions() {
		// Existing fused path: phase 1 + phase 2 via resolver.Resolve.
		res, err := resolver.Resolve(ast, resolveOpts)
		if err != nil {
			wrapped := wrapResolveError(err)
			if re, ok := wrapped.(*ResolveError); ok && re.FilePath == "" {
				re.OriginDescription = opts.OriginDescription()
			}
			return nil, wrapped
		}
		c := newConfig(res.Root)
		c.parseBaseDir = baseDir
		c.originDescription = opts.OriginDescription()
		return c, nil
	}
	// Deferred path: phase 1 only.  Includes are fully expanded; substitutions
	// remain as placeholders.
	tree, err := resolver.BuildTree(ast, resolveOpts)
	if err != nil {
		wrapped := wrapResolveError(err)
		if re, ok := wrapped.(*ResolveError); ok && re.FilePath == "" {
			re.OriginDescription = opts.OriginDescription()
		}
		return nil, wrapped
	}
	return newUnresolvedConfig(tree, baseDir, opts.OriginDescription()), nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

func wrapResolveError(err error) error {
	if re, ok := err.(*resolver.ResolveError); ok {
		return &ResolveError{
			Message:  re.Message,
			Path:     re.Path,
			Line:     re.Line,
			Col:      re.Col,
			FilePath: re.FilePath,
		}
	}
	return err
}
