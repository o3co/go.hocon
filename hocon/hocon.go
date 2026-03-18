// Copyright 2026 o3co Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"os"

	"github.com/o3co/go.hocon/hocon/internal/parser"
	"github.com/o3co/go.hocon/hocon/internal/resolver"
)

// ParseString parses a HOCON string and returns a Config.
func ParseString(input string) (*Config, error) {
	return parseWith(input, "")
}

// ParseFile parses a HOCON file and returns a Config.
func ParseFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{Message: err.Error(), FilePath: path}
	}
	return parseWith(string(data), path)
}

func parseWith(input, filePath string) (*Config, error) {
	ast, err := parser.Parse(input)
	if err != nil {
		if pe, ok := err.(*ParseError); ok {
			pe.FilePath = filePath
			return nil, pe
		}
		return nil, &ParseError{Message: err.Error(), FilePath: filePath}
	}
	baseDir := ""
	if filePath != "" {
		baseDir = dirOf(filePath)
	}
	res, err := resolver.Resolve(ast, resolver.Options{BaseDir: baseDir})
	if err != nil {
		return nil, wrapResolveError(err)
	}
	return &Config{root: res.Root}, nil
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
