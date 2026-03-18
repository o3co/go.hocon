// Copyright 2026 o3co Inc.
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

// Package hocon implements a parser for the HOCON (Human-Optimized Config
// Object Notation) configuration language, targeting full compliance with
// the Lightbend HOCON specification.
//
// Parse a config file:
//
//	cfg, err := hocon.ParseFile("application.conf")
//	host := cfg.GetString("server.host")
//	port := cfg.GetInt("server.port")
//
// Use Option for optional values:
//
//	timeout := cfg.GetDurationOption("server.timeout").OrElse(30 * time.Second)
//
// Unmarshal into a struct:
//
//	var s ServerConfig
//	err = cfg.Unmarshal(&s)
package hocon
