// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import (
	"errors"
	"testing"

	"github.com/o3co/go.hocon/internal/parser"
)

func TestBuildTree_LeavesSubstitutionPlaceholders(t *testing.T) {
	ast, err := parser.Parse("a = ${b}\nb = 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	v, ok := tree.Get("a")
	if !ok {
		t.Fatalf("expected key a, got none")
	}
	if _, isSubst := v.(*substPlaceholder); !isSubst {
		t.Fatalf("expected substPlaceholder for a, got %T", v)
	}
}

func TestResolveTree_ResolvesPlaceholders(t *testing.T) {
	ast, err := parser.Parse("a = ${b}\nb = 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	res, err := ResolveTree(tree, Options{})
	if err != nil {
		t.Fatalf("ResolveTree: %v", err)
	}
	v, ok := res.Root.Get("a")
	if !ok {
		t.Fatalf("expected a in resolved tree")
	}
	sv, ok := v.(*ScalarVal)
	if !ok || sv.Raw != "1" {
		t.Fatalf("expected a=1, got %#v", v)
	}
}

func TestResolveTree_AllowUnresolvedKeepsPlaceholder(t *testing.T) {
	ast, err := parser.Parse(`a = ${missing}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	res, err := ResolveTree(tree, Options{AllowUnresolved: true, UseSystemEnvironment: false})
	if err != nil {
		t.Fatalf("ResolveTree(allowUnresolved): %v", err)
	}
	v, _ := res.Root.Get("a")
	if _, ok := v.(*substPlaceholder); !ok {
		t.Fatalf("expected placeholder under allowUnresolved, got %T", v)
	}
}

func TestResolveTree_UseSystemEnvironmentFalse_BlocksEnvListSubst(t *testing.T) {
	ast, err := parser.Parse(`a = ${LISTVAR[]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	// Strict mode: required env list with no env access → ResolveError.
	_, err = ResolveTree(tree, Options{UseSystemEnvironment: false})
	if err == nil {
		t.Fatal("expected ResolveError for required ${LISTVAR[]} with UseSystemEnvironment=false")
	}
	var re *ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("expected *ResolveError, got %T", err)
	}
}

func TestResolveTree_UseSystemEnvironmentFalse_LenientKeepsEnvListPlaceholder(t *testing.T) {
	ast, err := parser.Parse(`a = ${LISTVAR[]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	// Lenient mode: leave the env-list substitution as a placeholder.
	res, err := ResolveTree(tree, Options{UseSystemEnvironment: false, AllowUnresolved: true})
	if err != nil {
		t.Fatalf("ResolveTree: %v", err)
	}
	v, _ := res.Root.Get("a")
	if _, ok := v.(*substPlaceholder); !ok {
		t.Fatalf("expected placeholder under UseSystemEnvironment=false+AllowUnresolved, got %T", v)
	}
}

func TestResolveTree_UseSystemEnvironmentFalse_OptionalEnvListDropsField(t *testing.T) {
	ast, err := parser.Parse(`a = ${?LISTVAR[]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	// Optional env list under UseSystemEnvironment=false → field is dropped.
	res, err := ResolveTree(tree, Options{UseSystemEnvironment: false})
	if err != nil {
		t.Fatalf("ResolveTree: %v", err)
	}
	if _, ok := res.Root.Get("a"); ok {
		t.Fatal("expected optional env-list field to be dropped with UseSystemEnvironment=false")
	}
}

func TestContainsPlaceholders(t *testing.T) {
	ast, err := parser.Parse("a = ${b}\nb = 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree, err := BuildTree(ast, Options{})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if !ContainsPlaceholders(tree) {
		t.Fatal("expected ContainsPlaceholders to return true for unresolved tree")
	}
	res, err := ResolveTree(tree, Options{})
	if err != nil {
		t.Fatalf("ResolveTree: %v", err)
	}
	if ContainsPlaceholders(res.Root) {
		t.Fatal("expected ContainsPlaceholders to return false for resolved tree")
	}
}
