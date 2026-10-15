// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package hocon_test

import (
	"strings"
	"testing"

	hocon "github.com/o3co/go.hocon"
)

// TestIncludePackage_MissMessageStatesHintOnce: an include of a package nobody
// registered names the package once and gives the blank-import hint once.
// The registry owns the hint (it alone knows the lookup missed); the
// resolver only adds the package(...) context.
func TestIncludePackage_MissMessageStatesHintOnce(t *testing.T) {
	_, err := hocon.ParseString(`include package("github.com/acme/missing", "reference.conf")`)
	if err == nil {
		t.Fatal("ParseString: expected error for an unregistered package")
	}
	msg := err.Error()
	for sub, want := range map[string]int{
		`package("github.com/acme/missing", "reference.conf")`:                      1,
		`not found in registry`:                                                     1,
		`ensure the providing package is imported with _ "github.com/acme/missing"`: 1,
	} {
		if got := strings.Count(msg, sub); got != want {
			t.Errorf("%q appears %d times in %q; want %d", sub, got, msg, want)
		}
	}
}
