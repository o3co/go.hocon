// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package hocon parses and evaluates HOCON (Human-Optimized Config Object Notation)
// as defined by the [Lightbend HOCON specification].
//
// # Parsing
//
// Parse HOCON from a string or file:
//
//	cfg, err := hocon.ParseString(`
//	  server {
//	    host = "localhost"
//	    port = 8080
//	    timeout = "30s"
//	  }
//	`)
//
//	cfg, err = hocon.ParseFile("application.conf")
//
// # Accessing values
//
// Three getter families share the same path and coercion rules and differ only
// in how a missing key, null or type mismatch comes back. Prefer the
// error-returning Get*E family in application code:
//
//	host, err := cfg.GetStringE("server.host")       // (string, error)
//	timeout, err := cfg.GetDurationE("server.timeout") // (time.Duration, error)
//
// The panicking family suits config already validated, and the Option family
// suits optional keys with a fallback:
//
//	port := cfg.GetInt("server.port")                       // panics with *ConfigError
//	user := cfg.GetStringOption("server.user").OrElse("app") // never panics
//
// # Unmarshaling
//
// Unmarshal into a struct using hocon struct tags, or into map[string]any:
//
//	type ServerConfig struct {
//	    Host    string         `hocon:"host"`
//	    Port    int            `hocon:"port"`
//	    Timeout time.Duration  `hocon:"timeout,omitempty"`
//	    MaxBody hocon.ByteSize `hocon:"max-body,omitempty"`
//	}
//
//	var s ServerConfig
//	err := cfg.Unmarshal(&s)
//
// A time.Duration field reads like GetDuration, a ByteSize field like GetBytes
// and a Period field like GetPeriod, so a unit value means the same thing
// through either path.
//
// # Substitutions
//
// HOCON substitutions (${path} and ${?path}) and self-referential assignments
// are fully supported:
//
//	base-url  = "https://api.example.com"
//	users-url = ${base-url}"/users"
//
//	path = ["/usr/bin"]
//	path = ${path} ["/usr/local/bin"]
//
// # Error types
//
// Structured error types carry source location information:
//
//	var pe *hocon.ParseError   // lexer/parser errors — Line, Col, FilePath
//	var re *hocon.ResolveError // substitution/include errors — Path
//	var ce *hocon.ConfigError  // Get*E errors and Get* panic payloads — Path
//
// [Lightbend HOCON specification]: https://github.com/lightbend/config/blob/main/HOCON.md
package hocon
