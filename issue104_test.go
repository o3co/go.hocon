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

// Additional coverage from multi-agent review: pin the "comma surrounded by
// optional newlines" contract in array position too, and the multi-newline
// case. Array-position negative cases also live in parser_test's S5.4/S5.5
// spec suite; pinning here as well guards against future suite restructures.

func TestIssue104CommaOnOwnLineInArray(t *testing.T) {
	input := `a: [1
,
2
,
3]`
	cfg, err := hocon.ParseString(input)
	if err != nil {
		t.Fatalf("expected to parse, got error: %v", err)
	}
	got := cfg.GetIntSlice("a")
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("expected [1,2,3], got %v", got)
	}
}

func TestIssue104MultipleNewlinesBeforeComma(t *testing.T) {
	input := `a: 1


,
b: 2`
	cfg, err := hocon.ParseString(input)
	if err != nil {
		t.Fatalf("expected to parse, got error: %v", err)
	}
	if cfg.GetInt("a") != 1 || cfg.GetInt("b") != 2 {
		t.Errorf("expected a=1,b=2, got a=%d,b=%d", cfg.GetInt("a"), cfg.GetInt("b"))
	}
}

func TestIssue104LeadingCommaInArrayStillRejected(t *testing.T) {
	_, err := hocon.ParseString(`a: [,1,2]`)
	if err == nil {
		t.Errorf("expected leading comma in array to be rejected")
	}
}

func TestIssue104RepeatedCommaInArrayStillRejected(t *testing.T) {
	_, err := hocon.ParseString(`a: [1,,2]`)
	if err == nil {
		t.Errorf("expected repeated commas in array to be rejected")
	}
}
