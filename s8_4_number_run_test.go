// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// S8.4 + E8 step 4: a token that starts like a number is read as the greedy
// run [0-9.eE+-] (Lightbend Tokenizer.pullNumber). A run that is a number
// (Java's Double.parseDouble grammar when it has . e E) is a number; a run
// that is not a number and contains '+' is an error; any other run is
// unquoted text. Every expected value below was measured on Lightbend
// 1.4.6 (2026-10-16).

package hocon_test

import (
	"encoding/json"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

const runErr = "ERROR"

type runCase struct {
	doc  string
	want string // JSON of the decoded document, or runErr
}

func checkRunCases(t *testing.T, cases []runCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.doc, func(t *testing.T) {
			cfg, err := hocon.ParseString(tc.doc)
			if tc.want == runErr {
				if err == nil {
					var m map[string]any
					_ = cfg.Unmarshal(&m)
					b, _ := json.Marshal(m)
					t.Fatalf("ParseString(%q) = %s; want a parse error", tc.doc, b)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseString(%q): %v", tc.doc, err)
			}
			var m map[string]any
			if err := cfg.Unmarshal(&m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			b, _ := json.Marshal(m)
			if string(b) != tc.want {
				t.Errorf("ParseString(%q) = %s, want %s", tc.doc, b, tc.want)
			}
		})
	}
}

// TestS8_4_NumberRun_ValuePosition pins the value-position table.
func TestS8_4_NumberRun_ValuePosition(t *testing.T) {
	checkRunCases(t, []runCase{
		{"a = 1e+x", runErr},
		{"a = 1+2", runErr},
		{"a = 1.2.3+build", runErr},
		{"a = 1.0.0+abc", runErr},
		{"a = +1", runErr},
		{"a = 1e+5", `{"a":100000}`},
		{"a = 1x+2", runErr},
		{"a = abc+1", runErr},
		{"a = abc+", runErr},
		{"a = a+b", runErr},
		{"a = 1e+5ms", `{"a":"1e+5ms"}`},
		{"a = 1e+5 ms", `{"a":"1e+5 ms"}`},
		{"a = 1.5e+3s", `{"a":"1.5e+3s"}`},
		{"a = 1e+5x", `{"a":"1e+5x"}`},
		{"a = 1e-x", `{"a":"1e-x"}`},
		{"a = 1e-5e5", `{"a":"1e-5e5"}`},
		{"a = -1.5e+3", `{"a":-1500}`},
		{"a = x1e+5", runErr},
		{"a = .5e+3", runErr},
		// Not a number run, and it contains '+': an error, not 1e+5 + text.
		{"a = 1e+5.5", runErr},
		{"a = 1e+5.2", runErr},
		{"a = 1e+5e5", runErr},
		{"a = 1e+5-", runErr},
		// Java's grammar allows digits followed by '.' with no fraction.
		{"a = 1.e+5", `{"a":100000}`},
	})
}

// TestS8_4_NumberRun_DigitsDot pins the Lightbend number reading of runs
// with a trailing or bare '.', and the runs that stay text.
func TestS8_4_NumberRun_DigitsDot(t *testing.T) {
	checkRunCases(t, []runCase{
		{"a = 1.", `{"a":1}`},
		{"a = 1.e5", `{"a":100000}`},
		{"a = -1.", `{"a":-1}`},
		{"a = 1e5", `{"a":100000}`},
		{"a = 1.5.", `{"a":"1.5."}`},
		{"a = .5", `{"a":".5"}`},
		{"a = -.", `{"a":"-."}`},
		{"a = 1.e", `{"a":"1.e"}`},
		// '-' starts a number run (Lightbend firstNumberChars).
		{"a = -.5", `{"a":-0.5}`},
		{"a = -.5e+3", `{"a":-500}`},
		{"a = -", `{"a":"-"}`},
		{"a = -foo", `{"a":"-foo"}`},
		{"a = --1", `{"a":"--1"}`},
		{"-foo = 1", `{"-foo":1}`},
	})
}

// TestS8_4_NumberRun_KeyPosition pins the key-position table.
func TestS8_4_NumberRun_KeyPosition(t *testing.T) {
	checkRunCases(t, []runCase{
		{"1e+5 = 1", `{"1e+5":1}`},
		{"1e+5x = 1", `{"1e+5x":1}`},
		{"a+b = 1", runErr},
		{"a.1e+5 = 1", runErr},
		{"x = [1]\nx += 2", `{"x":[1,2]}`}, // the += operator is unaffected
	})
}

// TestS8_4_NumberRun_DigitsDotRendersAsJSON: a number lexeme that is not a
// JSON number is never emitted as malformed JSON.
func TestS8_4_NumberRun_DigitsDotRendersAsJSON(t *testing.T) {
	cfg, err := hocon.ParseString("a = 1.e5\nb = 1.")
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":"1.e5","b":"1."}`; got != want {
		t.Errorf("RenderJSONForTest() = %s, want %s", got, want)
	}
	if !json.Valid([]byte(got)) {
		t.Errorf("RenderJSONForTest() = %s is not valid JSON", got)
	}
}
