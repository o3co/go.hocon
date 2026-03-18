package hocon_test

import (
	"testing"

	"github.com/o3co/go.hocon/hocon"
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
