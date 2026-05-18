// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import "testing"

// TestIsHoconWS exhaustively exercises every branch of the HOCON_WS predicate
// inlined in config.go (kept in sync with internal/lexer.isHoconWhitespace).
// Each branch (ASCII control, 0x1C-0x1F range, NBSP/BOM, U+1680, U+2000-200A
// range, 0x2028/2029/202F/205F, U+3000) is covered, plus a sample of
// non-whitespace runes (digit, letter, NEL U+0085 — Go unicode.IsSpace
// includes NEL but HOCON does not).
func TestIsHoconWS(t *testing.T) {
	yes := []rune{
		'\t', '\n', '\v', '\f', '\r', // ASCII control
		0x1C, 0x1D, 0x1E, 0x1F, // FS/GS/RS/US
		' ', 0xA0, 0xFEFF, // SP, NBSP, BOM
		0x1680,                         // OGHAM SPACE MARK
		0x2000, 0x2003, 0x2007, 0x200A, // U+2000-200A range samples
		0x2028, 0x2029, 0x202F, 0x205F, // line/para sep, narrow NBSP, medium math sp
		0x3000, // ideographic space
	}
	for _, r := range yes {
		if !isHoconWS(r) {
			t.Errorf("isHoconWS(U+%04X) = false, want true", r)
		}
	}
	no := []rune{
		'a', '0', '_',
		0x0085, // NEL — HOCON does NOT treat this as whitespace
		0x21, 0x7F, // ASCII boundary checks
		0x199F, 0x1681, // around U+1680
		0x1FFF, 0x200B, // around U+2000-200A range
		0x2027, 0x202A, 0x202E, 0x2030, 0x205E, 0x2060, // boundaries
		0x2FFF, 0x3001, // around U+3000
	}
	for _, r := range no {
		if isHoconWS(r) {
			t.Errorf("isHoconWS(U+%04X) = true, want false", r)
		}
	}
}
