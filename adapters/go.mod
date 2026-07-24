module github.com/o3co/go.hocon/adapters

go 1.23.0

require (
	github.com/o3co/go.hocon v1.9.0
	github.com/pelletier/go-toml/v2 v2.4.3
)

require github.com/goccy/go-yaml v1.19.2

// Build against the core in this repo so that a change spanning the parser and
// an adapter can be made, tested and reviewed as one commit.  Consumers ignore
// a dependency's replace directive and get the required version above, so the
// release check is that the adapters still build with GOFLAGS=-mod=mod against
// the published core — not against this checkout.
replace github.com/o3co/go.hocon => ../
