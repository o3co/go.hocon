// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package parser

import "github.com/o3co/go.hocon/internal/lexer"

// Node is the interface implemented by all AST nodes.
type Node interface {
	node()
	nodePos() (line, col int)
}

type pos struct{ line, col int }

func (p pos) nodePos() (int, int) { return p.line, p.col }

// ObjectNode represents a HOCON object { key: value, ... }.
type ObjectNode struct {
	pos
	Fields []FieldNode
}

func (n *ObjectNode) node() {}

// FieldNode is a single key-value pair inside an ObjectNode.
// Key is a slice of path segments (dot notation is pre-split).
// Append=true means += was used.
type FieldNode struct {
	pos
	Key    []string
	Value  Node
	Append bool
}

// ArrayNode represents a HOCON array [ elem, elem, ... ].
type ArrayNode struct {
	pos
	Elements []Node
}

func (n *ArrayNode) node() {}

// ScalarNode holds a primitive value as its raw string plus a type tag.
// ValueType is one of: "string", "number", "boolean", "null".
type ScalarNode struct {
	pos
	Raw       string
	ValueType string
	// Separator is true when this scalar was synthesized by the parser as the
	// whitespace run between two concatenated value tokens (not user-authored).
	// The resolver's isSeparator uses this flag to decide whether the node
	// contributes to string concatenation (S10.5) or is stripped for
	// object/array concatenation (S10.14). Carrying a flag (rather than
	// detecting a single-space Raw) lets the parser preserve the literal
	// whitespace run per S10.5 (go.hocon#132) without losing separator identity.
	Separator bool
}

func (n *ScalarNode) node() {}

// ConcatNode represents a whitespace-concatenated sequence of nodes
// (string concat or array concat — determined at resolve time).
type ConcatNode struct {
	pos
	Nodes []Node
}

func (n *ConcatNode) node() {}

// Line returns the source line of this node.
func (n *ConcatNode) Line() int { return n.line }

// Col returns the source column of this node.
func (n *ConcatNode) Col() int { return n.col }

// SubstNode represents a substitution ${path} or ${?path}.
type SubstNode struct {
	pos
	Path     string
	Optional bool
	Segments *lexer.SubstPayload // Segments is always non-nil — populated by parser.go from the lexer's TokenSubstitution payload.
}

func (n *SubstNode) node() {}

// Line returns the source line of this node.
func (n *SubstNode) Line() int { return n.line }

// Col returns the source column of this node.
func (n *SubstNode) Col() int { return n.col }

// IncludeNode represents an include directive.
// Required=true means the resource must exist (include required(...) form);
// Required=false means missing files are silently ignored per HOCON spec.
// For package includes (IsPackage=true), lookup failure is always a hard
// error regardless of Required (E11 decision 4).
type IncludeNode struct {
	pos
	Path     string // for file/bare includes
	Required bool
	IsFile   bool
	// E11 package include fields — populated when IsPackage=true.
	IsPackage bool   // true when qualifier is package(...)
	PkgID     string // package identifier (only when IsPackage)
	PkgFile   string // package file path (only when IsPackage)
}

func (n *IncludeNode) node() {}
