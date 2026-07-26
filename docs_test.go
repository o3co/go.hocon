// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The README states facts that go stale on their own: the compliance rates, the
// minimum Go version, and "unreleased" markers left behind after the release
// that shipped the behavior. None of those are exercised by any other test, so
// they only ever drift in one direction. These tests recompute each from the
// artifact that actually decides it — docs/spec-compliance.md and go.mod — and
// run in the release workflow, so a stale README fails the cut instead of
// shipping.

// specItemTotal is the number of S-items in the shared spec checklist
// (xx.hocon/docs/spec-checklist.md). Both compliance rates use it as the
// denominator, so a per-impl doc that has drifted away from the checklist would
// silently change the rates; the count is asserted rather than derived.
const specItemTotal = 210

// complianceCounts is the per-status tally of the S-items in
// docs/spec-compliance.md.
type complianceCounts struct {
	pass, partial, fail, unverified, outOfScope int
}

func (c complianceCounts) total() int {
	return c.pass + c.partial + c.fail + c.unverified + c.outOfScope
}

// specTotalRate answers "how much of HOCON.md does this implementation
// handle?" — out-of-scope items are in the denominator on purpose.
func (c complianceCounts) specTotalRate() float64 {
	return (float64(c.pass) + float64(c.partial)*0.5) / float64(specItemTotal) * 100
}

// inScopeRate answers "of what the implementation chooses to support, how much
// is covered?" — out-of-scope items drop out of the denominator.
func (c complianceCounts) inScopeRate() float64 {
	return (float64(c.pass) + float64(c.partial)*0.5) / float64(specItemTotal-c.outOfScope) * 100
}

var (
	// An S-item heading: "- **S13a.10** Some rule — §Section (L123)". E-items
	// (extra-spec conventions) use the same block shape but are not part of the
	// checklist, so the heading pattern is what separates them.
	specItemHeadRe = regexp.MustCompile(`^\s*- \*\*(S[0-9A-Za-z._]+)\*\*`)
	otherHeadRe    = regexp.MustCompile(`^\s*- \*\*[0-9A-Za-z._]+\*\*`)
	statusRe       = regexp.MustCompile(`^\s*status:`)
)

// countCompliance tallies the status glyph of every S-item block in
// docs/spec-compliance.md. Only the first `status:` line after an S-item
// heading counts: a block may carry sub-bullets, and E-item blocks that follow
// the S-items must not be picked up.
func countCompliance(t *testing.T) complianceCounts {
	t.Helper()

	data, err := os.ReadFile("docs/spec-compliance.md")
	if err != nil {
		t.Fatalf("read docs/spec-compliance.md: %v", err)
	}

	var counts complianceCounts
	inSpecItem := false
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case specItemHeadRe.MatchString(line):
			inSpecItem = true
		case otherHeadRe.MatchString(line):
			inSpecItem = false
		case inSpecItem && statusRe.MatchString(line):
			inSpecItem = false
			switch {
			case strings.Contains(line, "✅"):
				counts.pass++
			case strings.Contains(line, "⚠️"):
				counts.partial++
			case strings.Contains(line, "❌"):
				counts.fail++
			case strings.Contains(line, "🤷"):
				counts.unverified++
			case strings.Contains(line, "➖"):
				counts.outOfScope++
			default:
				t.Errorf("status line carries no known glyph: %q", strings.TrimSpace(line))
			}
		}
	}
	return counts
}

func readReadme(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	return string(data)
}

// findOne returns the single capture group of re in text, failing the test when
// the pattern no longer matches — a rewrite that drops the claim must fail
// loudly rather than silently stop checking it. source names the file the text
// came from, since this reads go.mod as well as the README.
//
// More than one match is also a failure: it would mean the doc states the claim
// twice and only the first is pinned, so the pair can drift apart while this
// stays green.
func findOne(t *testing.T, source, text string, re *regexp.Regexp, what string) string {
	t.Helper()

	ms := re.FindAllStringSubmatch(text, -1)
	switch len(ms) {
	case 0:
		t.Fatalf("%s not found in %s (pattern %s); update the pattern if %s was restructured", what, source, re, source)
	case 1:
	default:
		found := make([]string, len(ms))
		for i, m := range ms {
			found[i] = m[1]
		}
		t.Fatalf("%s matched %d times in %s (pattern %s); the claim must appear once so there is one thing to pin: %v",
			what, len(ms), source, re, found)
	}
	return ms[0][1]
}

func TestDocs_SpecComplianceItemCount(t *testing.T) {
	counts := countCompliance(t)
	if got := counts.total(); got != specItemTotal {
		t.Errorf("docs/spec-compliance.md has %d S-items, want %d (the shared checklist count) — an item was added, dropped, or its status line is malformed", got, specItemTotal)
	}
	if counts.unverified != 0 {
		t.Errorf("docs/spec-compliance.md has %d unverified (🤷) items; every item must be pinned by a test", counts.unverified)
	}
}

func TestDocs_ReadmeComplianceRates(t *testing.T) {
	counts := countCompliance(t)
	readme := readReadme(t)

	for _, tc := range []struct {
		what string
		re   *regexp.Regexp
		want float64
	}{
		{
			what: "spec-total compliance rate",
			re:   regexp.MustCompile(`\| Spec total \(incl\. out-of-scope\) \| \*\*([0-9.]+)%\*\* \|`),
			want: counts.specTotalRate(),
		},
		{
			what: "in-scope compliance rate",
			re:   regexp.MustCompile(`\| In-scope only \| \*\*([0-9.]+)%\*\* \|`),
			want: counts.inScopeRate(),
		},
	} {
		got := findOne(t, "README.md", readme, tc.re, tc.what)
		if want := fmt.Sprintf("%.1f", tc.want); got != want {
			t.Errorf("README %s is %s%%, recomputed from docs/spec-compliance.md it is %s%% (✅%d ⚠️%d ❌%d ➖%d)",
				tc.what, got, want, counts.pass, counts.partial, counts.fail, counts.outOfScope)
		}
	}
}

func TestDocs_ReadmeGoVersionMatchesGoMod(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	declared := findOne(t, "go.mod", string(data), regexp.MustCompile(`(?m)^go (\d+\.\d+)`), "go directive")

	claimed := findOne(t, "README.md", readReadme(t), regexp.MustCompile(`Requires Go (\d+\.\d+)\+`), "minimum Go version")
	if claimed != declared {
		t.Errorf("README says Go %s+, go.mod requires go %s — a user on %s cannot build this module", claimed, declared, claimed)
	}
}

func TestDocs_ReadmeHasNoUnreleasedMarkers(t *testing.T) {
	for i, line := range strings.Split(readReadme(t), "\n") {
		if strings.Contains(line, "(Unreleased)") {
			t.Errorf("README.md:%d still marks shipped behavior as unreleased: %q", i+1, strings.TrimSpace(line))
		}
	}
}
