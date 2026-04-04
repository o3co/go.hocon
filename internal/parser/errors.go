// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package parser

import "fmt"

// Error is a parse error carrying structured line/col position.
// The root hocon package maps this to the public ParseError type.
type Error struct {
	Message string
	Line    int
	Col     int
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("parse error at line %d, col %d: %s", e.Line, e.Col, e.Message)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

func newError(line, col int, format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Line: line, Col: col}
}
