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

		switch raw[i] {
		case '"':
			// Quoted segment
			i++ // skip opening quote
			var seg strings.Builder
			for i < len(raw) && raw[i] != '"' {
				if raw[i] == '\\' && i+1 < len(raw) {
					seg.WriteByte(raw[i+1])
					i += 2
				} else {
					seg.WriteByte(raw[i])
					i++
				}
			}
			segments = append(segments, seg.String())
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
		case '.':
			// Dot at start or after dot means empty-string segment
			segments = append(segments, "")
			i++
		default:
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
		if s == "" || strings.ContainsAny(s, ".\"\\") {
			escaped := strings.ReplaceAll(s, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			parts[i] = `"` + escaped + `"`
		} else {
			parts[i] = s
		}
	}
	return strings.Join(parts, ".")
}
