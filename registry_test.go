// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"errors"
	"testing"

	"github.com/o3co/go.hocon"
)

func TestRegisterPackageBasic(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	content := []byte("key = value")
	err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterPackageIdempotentByteEqual(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	content := []byte("key = value")
	if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", content); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	// Re-register byte-identical content — must be idempotent (nil error).
	if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", content); err != nil {
		t.Fatalf("idempotent re-registration: expected nil, got: %v", err)
	}
}

func TestRegisterPackageCollision(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	contentA := []byte("version = 1")
	contentB := []byte("version = 2")

	if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", contentA); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", contentB)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	var colErr *hocon.PackageCollisionError
	if !errors.As(err, &colErr) {
		t.Fatalf("expected *PackageCollisionError, got %T: %v", err, err)
	}
	if colErr.Identifier != "github.com/example/lib" {
		t.Errorf("Identifier: want %q, got %q", "github.com/example/lib", colErr.Identifier)
	}
	if colErr.File != "reference.conf" {
		t.Errorf("File: want %q, got %q", "reference.conf", colErr.File)
	}
}

func TestRegisterPackageEmptyIdentifier(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	err := hocon.RegisterPackage("", "reference.conf", []byte("x = 1"))
	if err == nil {
		t.Fatal("expected RegistrationError for empty identifier, got nil")
	}
	var regErr *hocon.RegistrationError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected *RegistrationError, got %T: %v", err, err)
	}
}

func TestRegisterPackageInvalidFile(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	// Empty file arg
	err := hocon.RegisterPackage("foo", "", []byte("x = 1"))
	if err == nil {
		t.Fatal("expected RegistrationError for empty file, got nil")
	}
	var regErr *hocon.RegistrationError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected *RegistrationError, got %T: %v", err, err)
	}
}

func TestRegisterPackageInvalidFileAbsolute(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	err := hocon.RegisterPackage("foo", "/etc/passwd", []byte("x = 1"))
	if err == nil {
		t.Fatal("expected RegistrationError for absolute path, got nil")
	}
	var regErr *hocon.RegistrationError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected *RegistrationError, got %T: %v", err, err)
	}
}

func TestRegisterPackageInvalidFileTraversal(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	err := hocon.RegisterPackage("foo", "../escape.conf", []byte("x = 1"))
	if err == nil {
		t.Fatal("expected RegistrationError for .. traversal, got nil")
	}
	var regErr *hocon.RegistrationError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected *RegistrationError, got %T: %v", err, err)
	}
}

func TestResetPackageRegistry(t *testing.T) {
	hocon.ResetPackageRegistry()
	t.Cleanup(hocon.ResetPackageRegistry)

	content := []byte("key = value")
	if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", content); err != nil {
		t.Fatalf("registration: %v", err)
	}
	hocon.ResetPackageRegistry()
	// After reset, re-registering with different content should succeed (no collision).
	contentB := []byte("key = other")
	if err := hocon.RegisterPackage("github.com/example/lib", "reference.conf", contentB); err != nil {
		t.Fatalf("post-reset registration: %v", err)
	}
}
