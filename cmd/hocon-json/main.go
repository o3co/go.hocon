// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Command hocon-json is the go.hocon adapter for the cross-impl differential
// harness (xx.hocon/generate). It parses+resolves a HOCON file and prints the
// resolved tree as canonical JSON to stdout. On any parse/resolve error it
// prints a single-line {"__error__":{"type":..,"message":..}} record to stdout
// and exits 3, so the differential driver can compare error-vs-success
// behaviour uniformly across go/rs/ts and the Lightbend oracle.
//
// Usage: hocon-json <conf-file>
//
// Environment substitutions resolve against the process environment, so the
// driver controls hermeticity by clearing/setting the subprocess env.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	hocon "github.com/o3co/go.hocon"
)

const exitError = 3

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hocon-json <conf-file>")
		os.Exit(2)
	}
	cfg, err := hocon.ParseFile(os.Args[1])
	if err != nil {
		emitError(err)
		os.Exit(exitError)
	}
	out, err := cfg.RenderJSONForTest()
	if err != nil {
		emitError(err)
		os.Exit(exitError)
	}
	fmt.Println(out)
}

// errorRecord serializes to {"__error__":{"type":..,"message":..}} with a
// deterministic field order (a struct, unlike a map, preserves order and lets
// us check the marshal error).
type errorRecord struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"__error__"`
}

func emitError(err error) {
	var rec errorRecord
	rec.Error.Type = fmt.Sprintf("%T", err)
	rec.Error.Message = err.Error()
	b, mErr := json.Marshal(rec)
	if mErr != nil {
		// Last resort: still emit a single-line __error__ object (never a blank
		// line) so the harness can classify this as an error rather than broken.
		fmt.Printf("{\"__error__\":{\"type\":%q,\"message\":\"json marshal failed\"}}\n", rec.Error.Type)
		return
	}
	fmt.Println(string(b))
}
