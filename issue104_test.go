package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon"
)

func TestIssue104CommaAfterNewlineInArray(t *testing.T) {
	input := `steps: [
  {
    name: first
  }
  ,{
    name: second
  }
]`
	cfg, err := hocon.ParseString(input)
	if err != nil {
		t.Fatalf("expected to parse, got error: %v", err)
	}
	steps := cfg.GetConfigSlice("steps")
	if got := len(steps); got != 2 {
		t.Fatalf("expected 2 steps, got %d", got)
	}
	if got := steps[0].GetString("name"); got != "first" {
		t.Errorf("expected steps[0].name=first, got %q", got)
	}
	if got := steps[1].GetString("name"); got != "second" {
		t.Errorf("expected steps[1].name=second, got %q", got)
	}
}

func TestIssue104CommaOnOwnLineInObject(t *testing.T) {
	input := `settings {
  first: true
  ,
  second: true
}`
	cfg, err := hocon.ParseString(input)
	if err != nil {
		t.Fatalf("expected to parse, got error: %v", err)
	}
	if !cfg.GetBool("settings.first") {
		t.Errorf("expected settings.first=true")
	}
	if !cfg.GetBool("settings.second") {
		t.Errorf("expected settings.second=true")
	}
}

func TestIssue104LeadingCommaStillRejected(t *testing.T) {
	_, err := hocon.ParseString(`,a: 1`)
	if err == nil {
		t.Errorf("expected leading comma to be rejected")
	}
}

func TestIssue104RepeatedCommaStillRejected(t *testing.T) {
	_, err := hocon.ParseString(`a: 1,, b: 2`)
	if err == nil {
		t.Errorf("expected repeated commas to be rejected")
	}
}
