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

func emitError(err error) {
	rec := map[string]map[string]string{
		"__error__": {
			"type":    fmt.Sprintf("%T", err),
			"message": err.Error(),
		},
	}
	b, _ := json.Marshal(rec)
	fmt.Println(string(b))
}
