// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package yaml ingests YAML documents as HOCON config.
//
//	base, _ := yaml.ParseFile("docker-compose.yml")
//	cfg, _ := hocon.ParseFileWithOptions("app.conf",
//		hocon.DefaultParseOptions().WithResolveSubstitutions(false))
//	merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
//
// This is a HOCON library, not a YAML implementation, and the API keeps that
// boundary. What this package owns is the decoded-tree -> HOCON step, exposed
// directly as FromValue: root must be a mapping, ${...} stays literal, NaN and
// infinity are refused, a multi-document stream is refused, binary becomes its
// base64 text. How YAML *text* becomes a tree — whether 010 is 8 or 10,
// whether no is a boolean — is the YAML library's answer, not a contract here.
//
// Parse and ParseFile are a convenience front on goccy/go-yaml. A caller who
// needs a different library, version or schema decodes the text themselves and
// hands the tree to FromValue; that is the supported way to swap parsers, and
// it keeps the choice — and its consequences — in the caller's hands.
//
// Keys follow spec F5.3 on both paths. A non-string scalar key takes its
// string form and a collection key is refused; two siblings whose string forms
// coincide — 1.0 and "1", or the int 1 and the string "1" in an injected
// map[any]any — are an error rather than a silent loss of one value. Parse
// checks that on the document itself (see keys.go), because a decoder hands
// over a map with the loser already gone.
//
// This package is part of the github.com/o3co/go.hocon/adapters module, which
// is versioned separately from the parser: see the module README for the
// go get line and the core-version requirement.
//
// See docs/specs/format-ingestion-mapping.md items F5.x in the hocon scope.
package yaml

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	goyaml "github.com/goccy/go-yaml"
	"github.com/o3co/go.hocon"
	"github.com/o3co/go.hocon/adapters/internal/keypath"
	"github.com/o3co/go.hocon/adapters/internal/tree"
)

// Parse reads YAML data with this package's default library (goccy/go-yaml).
// originDescription names the source in error messages; "" leaves it to
// hocon's default.
//
// Scalar resolution — whether `010` is 8 or 10, whether a timestamp is a
// string — is the library's answer, not a contract of this package. A caller
// who wants a different library, version or schema decodes the text themselves
// and hands the result to FromValue; this function is the convenience path.
func Parse(data []byte, originDescription string) (*hocon.Config, error) {
	data = stripBOM(data)
	doc, err := decode(data, originDescription)
	if err != nil {
		return nil, err
	}
	// After decode, so that a syntax error still comes from the decoder, which
	// words it better; before FromValue, because by then the collision has
	// already been resolved in the map (spec F5.3).
	if err := checkKeyCollisions(data, originDescription); err != nil {
		return nil, err
	}
	return FromValue(doc, originDescription)
}

// stripBOM drops a leading UTF-8 byte-order mark (spec F0.9). Windows editors
// write one, and left in place it becomes part of the first key: `a: 1` would
// yield a key with U+FEFF glued to the front, so a lookup of "a" misses and
// the value is unreachable — plausible-but-wrong output.
func stripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte("\ufeff"))
}

// FromValue builds a Config from an already-decoded YAML value tree, produced
// by whatever YAML library and settings the caller chose. This is the
// tree-level boundary this package actually owns (spec F5): Parse is just a
// default decoder in front of it.
//
// Leaf normalization accepts the shapes common across Go YAML libraries, not
// only the default one: map[any]any (yaml.v2 style) has its scalar keys
// stringified per F5.3, time.Time (go.yaml.in timestamps) becomes its RFC 3339
// string like a TOML date (F4.2's reasoning), and []byte becomes base64 (F5.5).
//
// Stringifying can make two sibling keys collide (the int 1 and the string
// "1"); that is an error naming both, not a race decided by map iteration
// order (F5.3).
func FromValue(doc any, originDescription string) (*hocon.Config, error) {
	// An empty document is the empty object, as an empty HOCON document is
	// (S3.1), rather than a root-type failure (spec F5.9).
	if doc == nil {
		return hocon.FromMap(map[string]any{}, originDescription)
	}
	var n normalizer
	normalized := n.walk(doc, nil)
	if err := n.err(); err != nil {
		return nil, fmt.Errorf("yaml: %s: %w", describe(originDescription), err)
	}
	nested, err := tree.Object(normalized, scalar)
	if err != nil {
		return nil, fmt.Errorf("yaml: %s: %w", describe(originDescription), err)
	}
	return hocon.FromMap(nested, originDescription)
}

// normalizer rewrites a decoded tree into the string-keyed shape
// hocon.FromMap accepts, applying F5.3 to the keys: a scalar key takes its
// string form, a collection key is refused, and two siblings whose string
// forms coincide are refused rather than one of them silently vanishing.
//
// Problems are collected rather than returned at the first sighting, because
// Go randomizes map iteration: aborting on whichever collision the runtime
// happened to reach first would make the *message* vary run to run even though
// the outcome (an error) does not. err sorts what it found and reports the
// same one every time.
type normalizer struct{ issues []issue }

type issue struct {
	path string // rendered location, e.g. db.ports."1"
	msg  string
}

func (n *normalizer) add(path []string, seg, msg string) {
	full := path
	if seg != "" {
		full = sub(path, seg)
	}
	where := keypath.Render(full)
	if where == "" {
		where = "the document root"
	}
	n.issues = append(n.issues, issue{path: where, msg: msg})
}

func (n *normalizer) err() error {
	if len(n.issues) == 0 {
		return nil
	}
	sort.Slice(n.issues, func(i, j int) bool {
		if n.issues[i].path != n.issues[j].path {
			return n.issues[i].path < n.issues[j].path
		}
		return n.issues[i].msg < n.issues[j].msg
	})
	first := n.issues[0]
	if len(n.issues) == 1 {
		return fmt.Errorf("at %s: %s", first.path, first.msg)
	}
	return fmt.Errorf("at %s: %s (and %d more key problem(s) in this document)",
		first.path, first.msg, len(n.issues)-1)
}

// walk normalizes v, which sits at path, recording any key problems it meets.
//
// Three map shapes reach here: the map[any]any of the yaml.v2 era, plain
// map[string]any (what goccy produces, and what most libraries produce), and
// sequences holding either. A map has already collapsed any colliding keys, so
// what this catches is a collision the *caller* built; the document itself is
// checked before it is decoded, in keys.go.
func (n *normalizer) walk(v any, path []string) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(x))
		seen := make(map[string]any, len(x))
		// Sorted so that, among several colliding pairs in one mapping, the
		// pair reported is not the one Go's map iteration happened to hit
		// first.
		for _, k := range sortedKeys(x) {
			ks, ok := n.key(k, path)
			if !ok {
				continue
			}
			if prev, dup := seen[ks]; dup {
				n.collision(path, ks, prev, k)
				continue
			}
			seen[ks] = k
			out[ks] = n.walk(x[k], sub(path, ks))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			// Distinct strings by construction, so no collision is possible.
			out[k] = n.walk(e, sub(path, k))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = n.walk(e, sub(path, strconv.Itoa(i)))
		}
		return out
	}
	return v
}

// sub returns path extended by seg, without sharing a backing array with its
// siblings.
func sub(path []string, seg string) []string {
	out := make([]string, len(path)+1)
	copy(out, path)
	out[len(path)] = seg
	return out
}

// key returns the object key for k, recording a problem and reporting false if
// k has no string form.
func (n *normalizer) key(k any, path []string) (string, bool) {
	ks, err := keyString(k)
	if err != nil {
		n.add(path, "", err.Error())
		return "", false
	}
	return ks, true
}

func (n *normalizer) collision(path []string, ks string, a, b any) {
	fa, fb := keyForm(a), keyForm(b)
	if fb < fa {
		fa, fb = fb, fa
	}
	n.add(path, ks, fmt.Sprintf(
		"sibling mapping keys %s and %s both give this key; which value wins "+
			"would depend on map iteration order (spec F5.3)", fa, fb))
}

// sortedKeys orders a map[any]any's keys by their string form, then by their
// rendered source form, so a traversal of it is reproducible.
func sortedKeys(m map[any]any) []any {
	keys := make([]any, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ki, _ := keyString(keys[i])
		kj, _ := keyString(keys[j])
		if ki != kj {
			return ki < kj
		}
		return keyForm(keys[i]) < keyForm(keys[j])
	})
	return keys
}

// keyString gives a key its object-key text (F5.3).
//
// goccy hands Parse its keys already stringified, including "null" for ~, so
// the nil case below is only reachable from an injected tree. Refusing it
// there loses nothing: an error is visible, whereas a nil key silently
// becoming "null" could merge with a real "null" key.
func keyString(k any) (string, error) {
	switch x := k.(type) {
	case string:
		return x, nil
	case bool, int, int64, uint64, float64:
		return fmt.Sprintf("%v", x), nil
	}
	return "", fmt.Errorf("mapping key of type %T is not usable as an object key (spec F5.3)", k)
}

// keyForm renders a source key for the F5.3 collision error: strings are
// quoted, other scalars show their Go type, so 1 (int) and "1" stay apart in
// the message the way they failed to in the mapping.
func keyForm(k any) string {
	if s, ok := k.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("%v (%T)", k, k)
}

// ParseFile reads path and parses it, using path as the origin description.
func ParseFile(path string) (*hocon.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-chosen config path
	if err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return Parse(data, path)
}

// decode reads exactly one document.
//
// F5.7: goyaml.Unmarshal returns the first document of a stream and discards
// the rest without a word, so the second decode below exists to turn that
// silent data loss into an error.
func decode(data []byte, origin string) (any, error) {
	dec := goyaml.NewDecoder(bytes.NewReader(data))

	var doc any
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil // empty input
		}
		return nil, fmt.Errorf("yaml: %s: %w", describe(origin), err)
	}

	var extra any
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return doc, nil
	case err == nil:
		return nil, fmt.Errorf(
			"yaml: %s: multi-document streams are not supported (spec F5.7); "+
				"a config is one document, and decoding only the first would drop the rest silently",
			describe(origin))
	default:
		return nil, fmt.Errorf("yaml: %s: %w", describe(origin), err)
	}
}

func describe(origin string) string {
	if origin == "" {
		return "(in-memory)"
	}
	return origin
}

// scalar maps a decoded YAML leaf onto a value hocon.FromMap accepts.
//
// Positive integers arrive as uint64 and negative ones as int64, so the
// overflow check lives on the uint64 branch.
func scalar(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string, int64:
		return x, nil
	case uint64:
		if x > math.MaxInt64 {
			return nil, fmt.Errorf("integer %d does not fit in int64 (spec F0.5)", x)
		}
		return int64(x), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("%v is not representable in HOCON (spec F0.6)", x)
		}
		return x, nil
	case []byte:
		// !!binary — HOCON has no binary type, so keep the base64 text the
		// source itself carried (spec F5.5).
		return base64.StdEncoding.EncodeToString(x), nil
	case time.Time:
		// Some libraries resolve timestamps to time.Time. HOCON has no
		// datetime, so its RFC 3339 text is the honest form — the same
		// reasoning as F4.2 for TOML dates.
		return x.Format(time.RFC3339Nano), nil
	case int:
		return int64(x), nil
	}
	return nil, fmt.Errorf("unsupported value of type %T", v)
}
