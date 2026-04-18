// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package resolver

import (
	"strings"
	"unicode"
)

// needsQuoting returns true if a segment must be quoted in a key string.
// Matches Lightbend's hasFunkyChars: any char that is not letter/digit/hyphen/underscore.
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' {
			return true
		}
	}
	return false
}

// segmentsToKey produces a canonical string from path segments.
// Segments that are empty or contain chars outside letter/digit/hyphen/underscore are quoted.
func segmentsToKey(segments []string) string {
	parts := make([]string, len(segments))
	for i, s := range segments {
		if needsQuoting(s) {
			escaped := strings.ReplaceAll(s, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			parts[i] = `"` + escaped + `"`
		} else {
			parts[i] = s
		}
	}
	return strings.Join(parts, ".")
}
