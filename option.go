// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon

// Option represents an optional value of type T.
// Use Some to wrap a present value, None for absent.
type Option[T any] struct {
	value   T
	present bool
}

// Some returns an Option containing v.
func Some[T any](v T) Option[T] {
	return Option[T]{value: v, present: true}
}

// None returns an empty Option of type T.
func None[T any]() Option[T] {
	return Option[T]{}
}

// IsSome reports whether the Option contains a value.
func (o Option[T]) IsSome() bool { return o.present }

// IsNone reports whether the Option is empty.
func (o Option[T]) IsNone() bool { return !o.present }

// Get returns the value and whether it is present.
func (o Option[T]) Get() (T, bool) { return o.value, o.present }

// OrElse returns the contained value if present, otherwise def.
func (o Option[T]) OrElse(def T) T {
	if o.present {
		return o.value
	}
	return def
}

// ParseOptions controls parse-phase behaviour.  Construct via DefaultParseOptions()
// and chain WithX() methods.  The zero-value literal ParseOptions{} is NOT a
// valid invocation — it would contradict Lightbend defaults (ResolveSubstitutions
// must default to true).  Per E12 § "Options encoding per language".
type ParseOptions struct {
	resolveSubstitutions bool
	originDescription    string
}

// DefaultParseOptions returns ParseOptions with Lightbend-equivalent defaults:
// ResolveSubstitutions=true, OriginDescription="".
func DefaultParseOptions() ParseOptions {
	return ParseOptions{
		resolveSubstitutions: true,
		originDescription:    "",
	}
}

// ResolveSubstitutions reports whether the parser must run resolve phase 2
// (substitution evaluation) before returning a Config.  Default: true.
func (o ParseOptions) ResolveSubstitutions() bool { return o.resolveSubstitutions }

// OriginDescription returns the user-visible source name used in error
// messages when no file path is available.  Default: "".
func (o ParseOptions) OriginDescription() string { return o.originDescription }

// WithResolveSubstitutions returns a copy with ResolveSubstitutions set to v.
func (o ParseOptions) WithResolveSubstitutions(v bool) ParseOptions {
	o.resolveSubstitutions = v
	return o
}

// WithOriginDescription returns a copy with OriginDescription set to s.
func (o ParseOptions) WithOriginDescription(s string) ParseOptions {
	o.originDescription = s
	return o
}

// ResolveOptions controls resolve-phase behaviour.  Construct via
// DefaultResolveOptions() and chain WithX() methods.
type ResolveOptions struct {
	useSystemEnvironment bool
	allowUnresolved      bool
}

// DefaultResolveOptions returns ResolveOptions with Lightbend-equivalent
// defaults: UseSystemEnvironment=true, AllowUnresolved=false.
func DefaultResolveOptions() ResolveOptions {
	return ResolveOptions{
		useSystemEnvironment: true,
		allowUnresolved:      false,
	}
}

// UseSystemEnvironment reports whether resolve consults process environment
// for substitution paths not satisfied within the config tree.  Default: true.
func (o ResolveOptions) UseSystemEnvironment() bool { return o.useSystemEnvironment }

// AllowUnresolved reports whether resolve leaves required-but-unsatisfied
// substitutions as placeholders (true) or returns ResolveError (false).
// Default: false.
func (o ResolveOptions) AllowUnresolved() bool { return o.allowUnresolved }

// WithUseSystemEnvironment returns a copy with UseSystemEnvironment set to v.
func (o ResolveOptions) WithUseSystemEnvironment(v bool) ResolveOptions {
	o.useSystemEnvironment = v
	return o
}

// WithAllowUnresolved returns a copy with AllowUnresolved set to v.
func (o ResolveOptions) WithAllowUnresolved(v bool) ResolveOptions {
	o.allowUnresolved = v
	return o
}
