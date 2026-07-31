module github.com/o3co/go.hocon/adapters

go 1.23.0

require (
	github.com/o3co/go.hocon v1.12.0
	github.com/pelletier/go-toml/v2 v2.4.3
)

require github.com/goccy/go-yaml v1.19.2

// Build against the core in this repo so that a change spanning the parser and
// an adapter can be made, tested and reviewed as one commit.  Consumers ignore
// a dependency's replace directive and get the required version above, so this
// checkout can build while every consumer's build does not — which is exactly
// what shipped in v1.10.0.
//
// The release sequence that keeps the two honest (also in CONTRIBUTING.md):
//
//  1. Bump the require above, in the same PR as the core change, to the
//     version that will be tagged.  CI's `adapters without replace (published
//     core)` job then finds that version missing from the proxy, warns and
//     skips — it cannot verify a version that does not exist yet.
//  2. Tag the core (vX.Y.Z) first.
//  3. Tag the adapters (adapters/vX.Y.Z).  The release workflow builds the
//     published module as a consumer would, and now the core it requires is
//     there to build against.
//
// On any PR that does not move the require ahead of the tags, that same job
// runs the build for real.
replace github.com/o3co/go.hocon => ../
