module github.com/o3co/go.hocon/adapters

go 1.23.0

require (
	github.com/o3co/go.hocon v1.10.0
	github.com/pelletier/go-toml/v2 v2.4.3
)

require github.com/goccy/go-yaml v1.19.2

// Build against the core in this repo so that a change spanning the parser and
// an adapter can be made, tested and reviewed as one commit.  Consumers ignore
// a dependency's replace directive and get the required version above, so a
// core change these packages use must bump that require in the same PR, to the
// version that will be tagged — otherwise this checkout builds and every
// consumer's build does not.  CI enforces it: the `adapters without replace
// (published core)` job in test.yml drops this line and builds with
// GOFLAGS=-mod=mod against the published core.
replace github.com/o3co/go.hocon => ../
