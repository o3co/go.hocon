package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon"
)

func TestSome(t *testing.T) {
	o := hocon.Some(42)
	if !o.IsSome() {
		t.Fatal("expected IsSome true")
	}
	if o.IsNone() {
		t.Fatal("expected IsNone false")
	}
	v, ok := o.Get()
	if !ok || v != 42 {
		t.Fatalf("Get() = %v, %v; want 42, true", v, ok)
	}
}

func TestNone(t *testing.T) {
	o := hocon.None[int]()
	if o.IsSome() {
		t.Fatal("expected IsSome false")
	}
	if !o.IsNone() {
		t.Fatal("expected IsNone true")
	}
	v, ok := o.Get()
	if ok || v != 0 {
		t.Fatalf("Get() = %v, %v; want 0, false", v, ok)
	}
}

func TestOrElse(t *testing.T) {
	if hocon.Some(1).OrElse(99) != 1 {
		t.Fatal("Some.OrElse should return value")
	}
	if hocon.None[int]().OrElse(99) != 99 {
		t.Fatal("None.OrElse should return default")
	}
}

func TestDefaultParseOptions(t *testing.T) {
	opts := hocon.DefaultParseOptions()
	if !opts.ResolveSubstitutions() {
		t.Fatal("DefaultParseOptions: ResolveSubstitutions must default to true")
	}
	if opts.OriginDescription() != "" {
		t.Fatalf("DefaultParseOptions: OriginDescription must default empty, got %q", opts.OriginDescription())
	}
}

func TestParseOptions_WithResolveSubstitutions(t *testing.T) {
	o1 := hocon.DefaultParseOptions()
	o2 := o1.WithResolveSubstitutions(false)
	if o1.ResolveSubstitutions() != true {
		t.Fatal("WithResolveSubstitutions must not mutate receiver")
	}
	if o2.ResolveSubstitutions() != false {
		t.Fatal("WithResolveSubstitutions(false) must produce ResolveSubstitutions=false")
	}
}

func TestParseOptions_WithOriginDescription(t *testing.T) {
	o := hocon.DefaultParseOptions().WithOriginDescription("inline-config")
	if o.OriginDescription() != "inline-config" {
		t.Fatalf("expected origin 'inline-config', got %q", o.OriginDescription())
	}
}

func TestParseOptions_Chainable(t *testing.T) {
	o := hocon.DefaultParseOptions().
		WithResolveSubstitutions(false).
		WithOriginDescription("inline")
	if o.ResolveSubstitutions() != false {
		t.Fatal("chained WithResolveSubstitutions")
	}
	if o.OriginDescription() != "inline" {
		t.Fatal("chained WithOriginDescription")
	}
}

func TestDefaultResolveOptions(t *testing.T) {
	opts := hocon.DefaultResolveOptions()
	if !opts.UseSystemEnvironment() {
		t.Fatal("DefaultResolveOptions: UseSystemEnvironment must default true")
	}
	if opts.AllowUnresolved() {
		t.Fatal("DefaultResolveOptions: AllowUnresolved must default false")
	}
}

func TestResolveOptions_With(t *testing.T) {
	o := hocon.DefaultResolveOptions().
		WithUseSystemEnvironment(false).
		WithAllowUnresolved(true)
	if o.UseSystemEnvironment() != false {
		t.Fatal("WithUseSystemEnvironment")
	}
	if o.AllowUnresolved() != true {
		t.Fatal("WithAllowUnresolved")
	}
}

func TestResolveOptions_NonMutating(t *testing.T) {
	o1 := hocon.DefaultResolveOptions()
	o2 := o1.WithAllowUnresolved(true)
	if o1.AllowUnresolved() != false {
		t.Fatal("WithAllowUnresolved must not mutate receiver")
	}
	if o2.AllowUnresolved() != true {
		t.Fatal("WithAllowUnresolved must produce true")
	}
}

func TestParseOptions_ZeroValue_TreatedAsDefaults(t *testing.T) {
	// ParseOptions{} zero-value at the API boundary is silently substituted
	// with DefaultParseOptions() (Lightbend defaults). Verify by parsing a
	// config that contains a substitution — under ResolveSubstitutions=false
	// (zero-value if we hadn't substituted) the result would be unresolved;
	// under the substituted default (true) the result is resolved.
	c, err := hocon.ParseStringWithOptions(`a = 1`, hocon.ParseOptions{})
	if err != nil {
		t.Fatalf("ParseStringWithOptions: %v", err)
	}
	if !c.IsResolved() {
		t.Fatal("ParseOptions{} zero-value must be treated as DefaultParseOptions() (ResolveSubstitutions=true)")
	}
	if c.GetInt("a") != 1 {
		t.Fatalf("a=%d, want 1", c.GetInt("a"))
	}
}

func TestResolveOptions_ZeroValue_TreatedAsDefaults(t *testing.T) {
	// ResolveOptions{} zero-value would mean UseSystemEnvironment=false
	// AllowUnresolved=false. Verify substitution: env-fallback works
	// because UseSystemEnvironment becomes true (substituted default).
	t.Setenv("TEST_E12_ZERO_VALUE", "from-env")
	c, _ := hocon.ParseStringWithOptions(
		`a = ${TEST_E12_ZERO_VALUE}`,
		hocon.DefaultParseOptions().WithResolveSubstitutions(false),
	)
	r, err := c.Resolve(hocon.ResolveOptions{}) // zero-value
	if err != nil {
		t.Fatalf("Resolve(zero-value ResolveOptions): %v (substitute must enable env)", err)
	}
	if r.GetString("a") != "from-env" {
		t.Fatalf("a=%q, want 'from-env' (env should be enabled under substituted defaults)", r.GetString("a"))
	}
}
