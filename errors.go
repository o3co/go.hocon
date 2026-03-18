// Copyright 2026 o3co Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import "fmt"

// ParseError is returned when lexing or parsing fails.
type ParseError struct {
	Message  string
	Line     int
	Col      int
	FilePath string // non-empty when inside an include file
}

func (e *ParseError) Error() string {
	if e.FilePath != "" {
		return fmt.Sprintf("parse error in %s at line %d, col %d: %s", e.FilePath, e.Line, e.Col, e.Message)
	}
	if e.Line > 0 {
		return fmt.Sprintf("parse error at line %d, col %d: %s", e.Line, e.Col, e.Message)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

// ResolveError is returned when resolution fails (substitution, include, circular ref).
type ResolveError struct {
	Message  string
	Path     string // HOCON substitution path e.g. "server.host"
	Line     int    // source line where the substitution appears (0 if unavailable)
	Col      int
	FilePath string // file path when resolving an include
}

func (e *ResolveError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("resolve error at path %q: %s", e.Path, e.Message)
	}
	return fmt.Sprintf("resolve error: %s", e.Message)
}

// ConfigError is used in panics from GetXxx methods.
type ConfigError struct {
	Message string
	Path    string // HOCON access path e.g. "server.host"
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error at path %q: %s", e.Path, e.Message)
}

// panicConfig panics with a ConfigError.
func panicConfig(path, msg string) {
	panic(&ConfigError{Path: path, Message: msg})
}
