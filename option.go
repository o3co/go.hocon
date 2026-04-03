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
