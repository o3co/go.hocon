// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package depth holds the ceiling on how deep a *name* may map.
//
// One environment variable name, or one dotted Properties key, produces one
// arbitrarily deep chain of objects. So unlike a nested document — which needs
// a deep document to get deep — the input needed here is a single long string,
// no structure required, and that is reachable for anything bulk-mounting a
// container environment.
//
// Go is the implementation least troubled by the consequence: goroutine stacks
// grow on demand, so a deep chain costs memory and time rather than crashing,
// where py.hocon raised RecursionError at 497 segments and rs.hocon aborted the
// process. The limit is here anyway, at the same number the three siblings use,
// because the alternative is that the same environment mounts in one
// implementation and errors in another — which is the divergence this project
// exists to keep out, whether or not it also crashes.
//
// Document nesting is deliberately *not* capped here. See the sibling
// implementations' notes: ts.hocon and py.hocon catch their runtime's own
// recursion error, rs.hocon has to cap because a Rust stack overflow aborts,
// and Go needs neither — 50 000 levels parse without incident.
package depth

// MaxPathSegments is the most segments one name may map to (spec F1.2 for env,
// S23.x for Properties). Shared with ts.hocon, py.hocon and rs.hocon.
const MaxPathSegments = 64

// TooDeep reports whether a mapped path is over MaxPathSegments.
func TooDeep(segments int) bool { return segments > MaxPathSegments }
