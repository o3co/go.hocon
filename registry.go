// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/o3co/go.hocon/internal/parser"
)

// pkgKey is the composite registry key for a package include.
// Using a struct key avoids string concatenation ambiguity.
type pkgKey struct {
	identifier string
	file       string
}

// pkgRegistry is the global package registry. Unexported; accessed via
// RegisterPackage, ResetPackageRegistry, and the lookup method.
type pkgRegistry struct {
	mu sync.RWMutex
	m  map[pkgKey][]byte
}

var globalRegistry = &pkgRegistry{m: make(map[pkgKey][]byte)}

// PackageCollisionError is returned by RegisterPackage when content already
// registered under (Identifier, File) differs from the new content being registered.
// This typically indicates two different import paths or major-version forks
// registered the same E11 identifier — resolve by ensuring only one registration wins.
type PackageCollisionError struct {
	Identifier string
	File       string
}

func (e *PackageCollisionError) Error() string {
	return fmt.Sprintf(
		"hocon: package registry collision for identifier %q file %q: "+
			"two different contents registered under the same key; "+
			"check for conflicting import paths or major-version forks "+
			"that register the same identifier",
		e.Identifier, e.File,
	)
}

// RegistrationError is returned by RegisterPackage when the identifier or file
// argument fails validation (empty identifier, invalid file path per E11 decision 6).
// This is distinct from PackageCollisionError, which concerns content conflicts.
type RegistrationError struct {
	Field  string // "identifier" or "file"
	Value  string
	Reason string
}

func (e *RegistrationError) Error() string {
	return fmt.Sprintf("hocon: invalid RegisterPackage argument %s=%q: %s", e.Field, e.Value, e.Reason)
}

// RegisterPackage registers HOCON content for use by include package(...) directives.
// The identifier is an opaque registry key (by convention a Go module path such as
// "github.com/o3co/auth"). The file argument is a forward-slash-separated relative
// path within that identifier's namespace. content is the raw HOCON source bytes
// (typically loaded via //go:embed).
//
// Re-registering byte-identical content under the same (identifier, file) key is
// idempotent and returns nil. Registering different content under the same key
// returns *PackageCollisionError. Registering an empty identifier or an invalid
// file path returns a *RegistrationError.
//
// In init() callers, the conventional pattern is:
//
//	if err := hocon.RegisterPackage(...); err != nil { panic(err) }
func RegisterPackage(identifier, file string, content []byte) error {
	if identifier == "" {
		return &RegistrationError{Field: "identifier", Value: identifier, Reason: "must be non-empty"}
	}
	if err := parser.ValidatePackageFile(file); err != nil {
		return &RegistrationError{Field: "file", Value: file, Reason: err.Error()}
	}

	key := pkgKey{identifier: identifier, file: file}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if existing, ok := globalRegistry.m[key]; ok {
		if bytes.Equal(existing, content) {
			return nil // idempotent re-registration of byte-equal content
		}
		return &PackageCollisionError{Identifier: identifier, File: file}
	}
	// Copy content to avoid aliasing with caller's slice.
	stored := make([]byte, len(content))
	copy(stored, content)
	globalRegistry.m[key] = stored
	return nil
}

// ResetPackageRegistry removes all registered packages from the global registry.
// It is intended for use in tests only; calling it in production code causes
// subsequent include package(...) directives to fail with lookup errors.
func ResetPackageRegistry() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.m = make(map[pkgKey][]byte)
}

// lookup looks up a (identifier, file) pair in the global registry.
// Returns (content, nil) on success, (nil, error) on miss.
func (r *pkgRegistry) lookup(identifier, file string) ([]byte, error) {
	key := pkgKey{identifier: identifier, file: file}
	r.mu.RLock()
	content, ok := r.m[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf(
			"package(%q, %q) not found in registry; "+
				"ensure the providing package is imported with _ %q in your application",
			identifier, file, identifier,
		)
	}
	return content, nil
}
