// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package properties provides a parser for Java .properties files.
// It handles key=value and key:value syntax, comments (#, !), and whitespace trimming.
package properties

import "strings"

// Parse parses a .properties file into a flat map of key-value string pairs.
// Keys and values are trimmed of surrounding whitespace.
// Lines starting with '#' or '!' are treated as comments and skipped.
// Both '=' and ':' are accepted as key-value separators.
func Parse(input string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		sepIdx := strings.IndexAny(trimmed, "=:")
		if sepIdx == -1 {
			continue
		}
		key := strings.TrimSpace(trimmed[:sepIdx])
		value := strings.TrimSpace(trimmed[sepIdx+1:])
		if key != "" {
			result[key] = value
		}
	}
	return result
}
