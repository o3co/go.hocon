// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package yaml

import (
	"fmt"
	"sort"
	"strconv"

	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/o3co/go.hocon/adapters/internal/keypath"
)

// checkKeyCollisions reports two sibling keys that resolve to the same object
// key — 1.0 and "1", ~ and "null", true and True (spec F5.3).
//
// It works on the document rather than on the decoded tree because by the time
// a YAML library has produced a Go map the collision is already resolved: both
// keys became the string "1", the map kept one entry, and a value is gone with
// no way left to notice. goccy's own duplicate-key detection compares the key
// *text*, so it catches `1:` against `"1":` and stops there.
//
// Merge keys are skipped. `<<: *defaults` legitimately brings in a key the
// mapping then overrides, which is YAML's own semantics and not a collision;
// the merged mapping is checked where it is written.
func checkKeyCollisions(data []byte, origin string) error {
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		// Not our error to report: the decoder runs first and phrases syntax
		// errors with its own (better) context.
		return nil
	}
	var c collisions
	for _, doc := range file.Docs {
		c.node(doc.Body, nil)
		// Only the first document is ever decoded; a stream is refused by
		// F5.7 in decode.
		break
	}
	return c.err(origin)
}

type collisions struct{ found []string }

func (c *collisions) err(origin string) error {
	if len(c.found) == 0 {
		return nil
	}
	msg := c.found[0]
	if n := len(c.found) - 1; n > 0 {
		msg = fmt.Sprintf("%s (and %d more colliding key(s) in this document)", msg, n)
	}
	return fmt.Errorf("yaml: %s: %s", describe(origin), msg)
}

// node walks the document structure. Only mappings and the things that can
// contain one matter here.
func (c *collisions) node(n ast.Node, path []string) {
	switch x := n.(type) {
	case *ast.DocumentNode:
		c.node(x.Body, path)
	case *ast.MappingNode:
		c.mapping(x.Values, path)
	case *ast.MappingValueNode:
		c.mapping([]*ast.MappingValueNode{x}, path)
	case *ast.SequenceNode:
		for i, e := range x.Values {
			c.node(e, sub(path, strconv.Itoa(i)))
		}
	case *ast.AnchorNode:
		// &name <value> — the anchored value is written here, so this is
		// where its keys get checked. Aliases to it are not revisited.
		c.node(x.Value, path)
	}
}

func (c *collisions) mapping(values []*ast.MappingValueNode, path []string) {
	seen := make(map[string]*ast.MappingValueNode, len(values))
	for _, v := range values {
		if v.Key.IsMergeKey() {
			continue
		}
		key, ok := objectKey(v.Key)
		if !ok {
			// An unusable key (a collection, an unresolvable alias). The
			// decoder rejects it with its own message.
			continue
		}
		if prev, dup := seen[key]; dup {
			c.report(path, key, prev, v)
			continue
		}
		seen[key] = v
		c.node(v.Value, sub(path, key))
	}
}

func (c *collisions) report(path []string, key string, a, b *ast.MappingValueNode) {
	where := keypath.Render(sub(path, key))
	forms := []string{
		fmt.Sprintf("%s (line %d)", a.Key.String(), a.Key.GetToken().Position.Line),
		fmt.Sprintf("%s (line %d)", b.Key.String(), b.Key.GetToken().Position.Line),
	}
	sort.Strings(forms)
	c.found = append(c.found, fmt.Sprintf(
		"mapping keys %s and %s both resolve to %s; quote the one you mean to "+
			"keep distinct, because one of the two values would otherwise be "+
			"lost (spec F5.3)",
		forms[0], forms[1], where))
}

// objectKey gives a key node the string the decoder will use for it.
//
// The rule mirrors goccy's own mapKeyNodeToString exactly: resolve the node,
// then null is "null", a string is itself, and everything else takes
// fmt.Sprint — including the kinds a scalar switch would not think to list.
// A tagged key resolves to time.Time (!!timestamp) or []byte (!!binary), and
// the decoder still makes a key of it, so an enumeration here would skip the
// node and miss the collision it causes: !!timestamp 2002-12-14 and the string
// "2002-12-14 00:00:00 +0000 UTC" are one key in the decoded map.
//
// Mirroring rather than enumerating is the whole point — a form this function
// disagreed with would either invent a collision or miss one.
//
// The false return is for a node that cannot be resolved at all (an alias with
// no anchor). Such a key is skipped rather than guessed at, and the decoder,
// which runs first, reports it with its own message.
func objectKey(k ast.MapKeyNode) (string, bool) {
	var v any
	if err := goyaml.NodeToValue(k, &v); err != nil {
		return "", false
	}
	switch x := v.(type) {
	case nil:
		return "null", true
	case string:
		return x, true
	default:
		return fmt.Sprint(x), true
	}
}
