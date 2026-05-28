package hocon_test

import (
	"testing"

	hocon "github.com/o3co/go.hocon"
)

func TestRenderJSONForTest_Canonical(t *testing.T) {
	cfg, err := hocon.ParseString("a = 1\nb = \"x\"\nc { d = true }\n")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	got, err := cfg.RenderJSONForTest()
	if err != nil {
		t.Fatalf("RenderJSONForTest: %v", err)
	}
	const want = `{"a":1,"b":"x","c":{"d":true}}`
	if got != want {
		t.Fatalf("RenderJSONForTest() = %q, want %q", got, want)
	}
}
