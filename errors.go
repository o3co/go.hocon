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
	"fmt"
)

// ParseError is returned when lexing or parsing fails.
type ParseError struct {
	Message           string
	Line              int
	Col               int
	FilePath          string // non-empty when inside an include file
	OriginDescription string // E12: user-supplied label when no FilePath available
}

func (e *ParseError) Error() string {
	src := e.FilePath
	if src == "" {
		src = e.OriginDescription
	}
	if src != "" {
		return fmt.Sprintf("parse error in %s at line %d, col %d: %s", src, e.Line, e.Col, e.Message)
	}
	if e.Line > 0 {
		return fmt.Sprintf("parse error at line %d, col %d: %s", e.Line, e.Col, e.Message)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

// ResolveError is returned when resolution fails (substitution, include, circular ref).
type ResolveError struct {
	Message           string
	Path              string // HOCON substitution path e.g. "server.host"
	Line              int    // source line where the substitution appears (0 if unavailable)
	Col               int
	FilePath          string // file path when resolving an include
	OriginDescription string // E12: user-supplied label when no FilePath available
}

func (e *ResolveError) Error() string {
	src := e.FilePath
	if src == "" {
		src = e.OriginDescription
	}
	if e.Path != "" {
		if src != "" {
			return fmt.Sprintf("resolve error in %s at path %q: %s", src, e.Path, e.Message)
		}
		return fmt.Sprintf("resolve error at path %q: %s", e.Path, e.Message)
	}
	if src != "" {
		return fmt.Sprintf("resolve error in %s: %s", src, e.Message)
	}
	return fmt.Sprintf("resolve error: %s", e.Message)
}

// ConfigError is used in panics from GetXxx methods.
type ConfigError struct {
	Message string
	Path    string // HOCON access path e.g. "server.host"
	cause   error  // underlying sentinel (e.g. ErrNotResolved) for errors.Is
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error at path %q: %s", e.Path, e.Message)
}

// Unwrap returns the underlying cause for errors.Is / errors.As.
func (e *ConfigError) Unwrap() error {
	return e.cause
}

// panicConfig panics with a ConfigError.
func panicConfig(path, msg string) {
	panic(&ConfigError{Path: path, Message: msg})
}

// ErrNotResolved is the sentinel returned (wrapped) when a getter is called on
// a Config path whose value (or any transitive parent) contains an unresolved
// substitution placeholder.  E12 § "Getters on unresolved Config" pins this
// as a MUST.  Wrap via fmt.Errorf("...: %w", ErrNotResolved, ...) so callers
// can use errors.Is(err, hocon.ErrNotResolved).
var ErrNotResolved = errors.New("config: value is not resolved (call Resolve() first)")
