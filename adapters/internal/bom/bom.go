// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package bom strips the UTF-8 byte-order mark that Windows editors add.
//
// Spec F0.9: a BOM left in place becomes part of the first key — `a: 1` yields
// a key with U+FEFF glued to the front, so a lookup of "a" misses and the
// value is silently unreachable. That is plausible-but-wrong output, which
// this project treats as worse than an outright error. Every adapter strips it
// at its entry point, as the core HOCON parser already does.
package bom

import "bytes"

var mark = []byte("\ufeff")

// Strip removes a leading UTF-8 BOM, if there is one.
func Strip(data []byte) []byte { return bytes.TrimPrefix(data, mark) }
