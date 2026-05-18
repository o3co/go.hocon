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
// Only file-based includes are supported in v1.0.
// Required=true means the file must exist (include required(...) form);
// Required=false means missing files are silently ignored per HOCON spec.
type IncludeNode struct {
	pos
	Path     string
	Required bool
	IsFile   bool
}

func (n *IncludeNode) node() {}
