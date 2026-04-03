// Copyright 2026 1o1 Co. Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package properties

import "testing"

func TestParse(t *testing.T) {
	result := Parse("host=localhost\nport=8080")
	if result["host"] != "localhost" {
		t.Errorf("host=%q", result["host"])
	}
	if result["port"] != "8080" {
		t.Errorf("port=%q", result["port"])
	}
}

func TestParseColon(t *testing.T) {
	result := Parse("host:localhost")
	if result["host"] != "localhost" {
		t.Errorf("host=%q", result["host"])
	}
}

func TestParseComments(t *testing.T) {
	result := Parse("# comment\n! also\nkey=val")
	if len(result) != 1 || result["key"] != "val" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestParseTrim(t *testing.T) {
	result := Parse("  key  =  value  ")
	if result["key"] != "value" {
		t.Errorf("key=%q", result["key"])
	}
}

func TestParseEmpty(t *testing.T) {
	result := Parse("\n\nkey=val\n\n")
	if result["key"] != "val" {
		t.Errorf("key=%q", result["key"])
	}
}
