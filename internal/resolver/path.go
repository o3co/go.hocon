// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import "strings"

// parseSubstPath parses a substitution path string into segments,
// handling quoted identifiers that may contain dots.
func parseSubstPath(raw string) []string {
	var segments []string
	i := 0
	for i < len(raw) {
		// Skip whitespace
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i >= len(raw) {
			break
		}

		if raw[i] == '"' {
			// Quoted segment
			i++ // skip opening quote
			start := i
			for i < len(raw) && raw[i] != '"' {
				i++
			}
			segments = append(segments, raw[start:i])
			if i < len(raw) {
				i++ // skip closing quote
			}
			// Skip whitespace and dot separator
			for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
				i++
			}
			if i < len(raw) && raw[i] == '.' {
				i++
			}
		} else if raw[i] == '.' {
			// Dot at start or after dot means empty-string segment
			segments = append(segments, "")
			i++
		} else {
			// Unquoted segment - read until dot or end
			start := i
			for i < len(raw) && raw[i] != '.' {
				i++
			}
			segments = append(segments, strings.TrimSpace(raw[start:i]))
			if i < len(raw) && raw[i] == '.' {
				i++
			}
		}
	}
	return segments
}

// segmentsToKey produces a canonical string from path segments.
// Segments containing dots or that are empty strings are quoted.
func segmentsToKey(segments []string) string {
	parts := make([]string, len(segments))
	for i, s := range segments {
		if s == "" || strings.ContainsAny(s, ".\"") {
			parts[i] = `"` + s + `"`
		} else {
			parts[i] = s
		}
	}
	return strings.Join(parts, ".")
}
