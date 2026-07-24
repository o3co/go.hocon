// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package adapters is the root of the go.hocon format adapters; the adapters
// themselves live in the subpackages.
//
// This is a module of its own so that go.hocon keeps zero dependencies —
// importing the parser never pulls in a TOML or YAML library — while still
// living in the parser's repository, so a change spanning both is one commit.
//
// Each subpackage reads a config format owned by some other program and
// returns a *hocon.Config you can put underneath your own document with
// WithFallback, so a ${...} can reach into it:
//
//	properties  java.util.Properties files
//	env         environment variables, and .env files
//	jsonc       JSON with comments and trailing commas
//	toml        TOML documents
//
// Plain JSON has no subpackage because it needs no adapter: HOCON is a JSON
// superset, so hocon.ParseFile accepts a .json file as it stands. The tests
// alongside this file exist to keep that claim honest.
//
// Nothing here renders HOCON text. A foreign document is decoded and turned
// straight into a value tree, which is why there is no emitter in this module
// and no escaping to get wrong.
package adapters
